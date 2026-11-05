// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A route-backed verb runs its REST route's own handler in process as the
// caller, the write twin of read_view. One engine, so the doors cannot differ.

// routeVerbShape is what the generator read off a route-backed verb's route.
type routeVerbShape struct {
	Summary string
	IfMatch bool
	// IdempotencyKey is true when the route refuses a call with no key.
	IdempotencyKey bool
	Body           bool
	// UploadField is the form field of a route that takes one file as multipart.
	UploadField string
	// BodySchema and AnswerSchema are the route's JSON body and 2xx answer
	// schemas, $refs resolved.
	BodySchema   string
	AnswerSchema string
	Query        []viewQuery
}

// declaredRouteVerbs lists every verb the contract declares served_by: route.
func declaredRouteVerbs(policies map[string]agentPolicy) []agents.RouteVerb {
	var verbs []agents.RouteVerb
	for route, pol := range policies {
		if pol.Access != accessTool || pol.ServedBy != servedByRoute {
			continue
		}
		method, pattern, _ := strings.Cut(route, " ")
		shape := routeVerbShapes[route]
		verbs = append(verbs, agents.RouteVerb{
			Verb: pol.Tool, Router: pol.Router, Action: pol.Action, Op: pol.Op, Summary: shape.Summary,
			Method: method, Pattern: pattern, Scope: principal.Scope(pol.Scope), PathParams: pathParamsOf(pattern),
			Query: shape.Query, IfMatch: shape.IfMatch, IdempotencyKey: shape.IdempotencyKey, Body: shape.Body, UploadField: shape.UploadField, BodySchema: json.RawMessage(shape.BodySchema),
			AnswerSchema: json.RawMessage(shape.AnswerSchema), AskUserFirst: pol.AskUserFirst,
		})
	}
	slices.SortFunc(verbs, func(a, b agents.RouteVerb) int { return strings.Compare(a.Verb, b.Verb) })
	return verbs
}

// routeThrough runs a route-backed call through the contract router.
func routeThrough(door *viewDoor) agents.RouteRunner {
	return func(ctx context.Context, call agents.RouteCall) (json.RawMessage, error) {
		if door == nil || door.api == nil {
			return nil, fmt.Errorf("%s: route-backed verbs are served by the API process: %w", call.Verb.Verb, apperrors.ErrUnsupportedBySoR)
		}
		// These run as a user does in the app. Unattended work with no passport has no
		// user to act for, whatever grants its principal carries.
		if actor, ok := principal.Actor(ctx); ok && !actor.HumanOrItsAgent() {
			return nil, fmt.Errorf("%s: this acts for a user, and this work has none: %w", call.Verb.Verb, apperrors.ErrPermissionDenied)
		}
		in := inProcessCall{Method: call.Verb.Method, Pattern: call.Verb.Pattern, Path: call.Path, Query: call.Query, Version: call.Version, FileLimit: agents.MaxInlineFileBytes}
		// One key per call. This surface refuses a caller's idempotency_key on a
		// route verb, so a retry is a new call: the refusal says to read first.
		if call.Verb.IdempotencyKey {
			in.IdempotencyKey = ids.NewV7().String()
		}
		if call.Verb.Body {
			in.Body = call.Body
			if len(in.Body) == 0 {
				in.Body = json.RawMessage(`{}`)
			}
		}
		if call.Verb.UploadField != "" {
			upload, err := uploadFormOf(call.Verb.UploadField, call.File)
			if err != nil {
				return nil, fmt.Errorf("%s: writing the upload: %w", call.Verb.Verb, err)
			}
			in.Upload = upload
		}
		return routeAnswer(call.Verb, door.serveInProcess(ctx, in))
	}
}

// uploadFormOf is the multipart form a browser sends for one file. It carries the
// type the caller declared, because a route may judge a file by it.
func uploadFormOf(field string, file agents.RouteFile) (*uploadForm, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", mime.FormatMediaType("form-data", map[string]string{previewFieldName: field, "filename": file.Name}))
	header.Set("Content-Type", file.ContentType)
	part, err := form.CreatePart(header)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(file.Content); err != nil {
		return nil, err
	}
	if err := form.Close(); err != nil {
		return nil, err
	}
	return &uploadForm{contentType: form.FormDataContentType(), body: body.Bytes()}, nil
}

// routeAnswer is a route-backed call's answer. A read that overflows is refused,
// as read_view refuses one: nothing happened, and a retry is safe. A write has
// committed by then, so it is a success that says it was cut, as its schema
// declares. A refusal keeps the route's own detail, so a caller learns what to do.
func routeAnswer(call agents.RouteVerb, rec *cappedRecorder) (json.RawMessage, error) {
	verb := call.Verb
	res := rec.Result()
	if res.StatusCode < http.StatusBadRequest {
		return routeSuccess(call, rec, res)
	}
	if res.StatusCode >= http.StatusInternalServerError {
		return nil, fmt.Errorf("%s: the route failed with %d", verb, res.StatusCode)
	}
	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil || problem.Detail == "" {
		problem.Detail = http.StatusText(res.StatusCode)
	}
	if problem.Code == "version_skew" {
		return nil, fmt.Errorf("%s: %s: %w", verb, problem.Detail, apperrors.ErrVersionSkew)
	}
	if refusal, known := viewRefusals[res.StatusCode]; known {
		return nil, fmt.Errorf("%s: %s: %w", verb, problem.Detail, refusal)
	}
	return nil, httperr.Validation(verb, problem.Code, problem.Detail)
}

// routeSuccess is the answer of a route that did what it was asked.
func routeSuccess(call agents.RouteVerb, rec *cappedRecorder, res *http.Response) (json.RawMessage, error) {
	verb := call.Verb
	isFile := answersAFile(rec.declaredType)
	switch {
	case isFile && rec.overflowed:
		return nil, httperr.Validation(verb, "file_too_large",
			"this file is larger than the "+httperr.Megabytes(agents.MaxInlineFileBytes)+" one answer carries; ask the user to open it in the app")
	case isFile:
		return fileAnswer(res, rec.Body.Bytes())
	case rec.overflowed && call.Method == http.MethodGet:
		return nil, httperr.Validation(verb, "answer_too_large", fmt.Sprintf(
			"the answer is more than %d MB; narrow it with its filters, or ask the user to open it in the app", maxViewAnswerBytes>>20,
		))
	case rec.overflowed:
		return json.Marshal(map[string]any{previewFieldStatus: res.StatusCode, agents.RouteAnswerTruncated: true})
	case rec.Body.Len() == 0:
		return json.Marshal(map[string]int{previewFieldStatus: res.StatusCode})
	}
	return viewAnswer(res)
}

// answersAFile is true for a route answer that is not a JSON document.
func answersAFile(contentType string) bool {
	return contentType != "" && !strings.Contains(contentType, "json")
}

// fileAnswer carries a file's bytes in a JSON answer, as attach takes them.
func fileAnswer(res *http.Response, content []byte) (json.RawMessage, error) {
	var filename string
	if _, params, err := mime.ParseMediaType(res.Header.Get("Content-Disposition")); err == nil {
		filename = params["filename"]
	}
	return json.Marshal(struct {
		Filename      string `json:"filename,omitempty"`
		ContentType   string `json:"content_type"`
		SizeBytes     int    `json:"size_bytes"`
		ContentBase64 string `json:"content_base64"`
	}{filename, res.Header.Get("Content-Type"), len(content), base64.StdEncoding.EncodeToString(content)})
}

// RouteVerbs lists the route-backed verbs this build serves, for the tests
// that hold each one to its route.
func RouteVerbs() []agents.RouteVerb { return declaredRouteVerbs(agentPolicies) }

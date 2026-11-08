// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The in-process door sends what a browser sends. The bytes, the name and the
// declared type arrive where the route's own form reader looks for them.
func TestAnUploadReachesItsRouteAsTheBrowsersForm(t *testing.T) {
	content := []byte{0x89, 'P', 'N', 'G', 0x00, 0xff}
	form, err := uploadFormOf("file", agents.RouteFile{Name: "M\u00fcller logo.png", ContentType: "image/png", Content: content})
	if err != nil {
		t.Fatal(err)
	}
	media, params, err := mime.ParseMediaType(form.contentType)
	if err != nil || media != "multipart/form-data" {
		t.Fatalf("content type %q, err %v", form.contentType, err)
	}
	parsed, err := multipart.NewReader(bytes.NewReader(form.body), params["boundary"]).ReadForm(1 << 20)
	if err != nil {
		t.Fatal(err)
	}
	files := parsed.File["file"]
	if len(files) != 1 {
		t.Fatalf("form files = %v, want one under file", parsed.File)
	}
	if files[0].Filename != "M\u00fcller logo.png" || files[0].Header.Get("Content-Type") != "image/png" {
		t.Errorf("name %q, type %q", files[0].Filename, files[0].Header.Get("Content-Type"))
	}
	part, err := files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	var got bytes.Buffer
	if _, err := got.ReadFrom(part); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), content) {
		t.Errorf("bytes = %v, want %v", got.Bytes(), content)
	}
}

// A route-backed verb acts for a user. Work with no passport behind it is turned
// away at the door, whatever grants its principal carries.
func TestARouteVerbRefusesPassportlessWork(t *testing.T) {
	reached := false
	door := &viewDoor{api: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})}
	call := agents.RouteCall{Verb: agents.RouteVerb{Verb: "update_stage", Method: http.MethodPatch, Pattern: "/v1/stages/x", Scope: principal.ScopeWrite}}
	passportless := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:ext.job", Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	if _, err := routeThrough(door)(passportless, call); !errors.Is(err, apperrors.ErrPermissionDenied) || reached {
		t.Errorf("passportless work answered %v, reached the route %t", err, reached)
	}
	onPassport := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:on-passport", PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7(),
	})
	if _, err := routeThrough(door)(onPassport, call); err != nil || !reached {
		t.Errorf("an agent on a passport answered %v, reached the route %t", err, reached)
	}
}

// An in-process upload rides the bound the chassis gives its route, or a tool could
// send what a browser cannot. The verbs come from the contract, so a new one is asked.
func TestAnUploadVerbRidesItsRoutesCeiling(t *testing.T) {
	limits := UploadLimits{Attachment: 2 << 10, CSVImport: 3 << 10, LinkedInImport: 4 << 10, KnowledgeDocument: 5 << 10}
	ceilings := uploadCeilings(limits)
	asked := 0
	for route, shape := range routeVerbShapes {
		if shape.UploadField == "" {
			continue
		}
		asked++
		method, pattern, _ := strings.Cut(route, " ")
		ceiling, declared := ceilings[pattern]
		if !declared {
			t.Errorf("%s uploads a file and declares no ceiling", route)
			continue
		}
		var read int64
		var overflow *http.MaxBytesError
		door := &viewDoor{ceiling: bodyCeilingFor(ceilings, contractRoutes()), api: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var err error
			read, err = io.Copy(io.Discard, r.Body)
			if errors.As(err, &overflow) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})}
		verb := agents.RouteVerb{Verb: route, Method: method, Pattern: pattern, UploadField: shape.UploadField, PathParams: pathParamsOf(pattern)}
		call := agents.RouteCall{Verb: verb, Path: map[string]string{}, File: agents.RouteFile{Name: "a", ContentType: "text/plain", Content: make([]byte, ceiling+1)}}
		for _, param := range verb.PathParams {
			call.Path[param] = "01a1a9ff-1625-723c-906e-02eddcd5eb56"
		}
		if _, err := routeThrough(door)(context.Background(), call); err == nil || overflow == nil || overflow.Limit != ceiling {
			t.Errorf("%s past its ceiling answered %v, read %d, stopped by %+v", route, err, read, overflow)
		}
		call.File.Content = []byte("small")
		overflow = nil
		if _, err := routeThrough(door)(context.Background(), call); err != nil || overflow != nil {
			t.Errorf("%s under its ceiling answered %v", route, err)
		}
	}
	if asked == 0 {
		t.Fatal("the contract declares no upload verb, so this asked nothing")
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A model request names its records by short refs (s1, a2) and never by id.
// These suites script their fake models by the record ids the test seeded.
// This file reads the text a record shows from the database, finds that text
// in the request, and takes its ref. aitest.RefWhere reads a JSON payload by a
// field the test already knows; these prompts are fenced spans and lines.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// shownBy is how a request shows one kind of record. key is the fence
// attribute or line label its ref follows, and query reads its text by id ($1).
// render, when set, is the site's own rendering of that text into the prompt.
type shownBy struct {
	key    string
	query  string
	render func(string) string
}

// dispositionShown finds a first-time sender by its address, in the brackets
// addressLine writes it in, so one address is never found inside another.
var dispositionShown = shownBy{
	key:   "id",
	query: `SELECT '(' || email || ')' FROM capture_pending_counterparty WHERE id = $1`,
}

// requestShown finds an answered request by the first line the settlement
// prompt renders of it, which opens its conversation's span.
var requestShown = shownBy{
	key: "source_id", query: `SELECT body FROM activity WHERE id = $1`,
	render: func(body string) string {
		opening, _, _ := strings.Cut(strings.TrimSpace(settleMessageBody(threadMessage{
			Direction: string(crmcontracts.ActivityDirectionInbound), Body: body,
		})), "\n")
		return opening
	},
}

// recordUUID matches a record id in a scripted reply.
var recordUUID = regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)

// refOfRecord is the ref req names record id by, or "" when req does not show
// the record.
func refOfRecord(ctx context.Context, req model.Request, by shownBy, id string) (string, error) {
	text, err := shownText(ctx, by.query, id)
	if err != nil {
		return "", err
	}
	if by.render != nil {
		text = by.render(text)
	}
	if text == "" {
		return "", nil
	}
	return refHolding(req, by.key, text), nil
}

// recordsByRef maps each ref req shows to the candidate record it names.
func recordsByRef(ctx context.Context, req model.Request, by shownBy, candidates []string) (map[string]string, error) {
	out := map[string]string{}
	for _, id := range candidates {
		ref, err := refOfRecord(ctx, req, by, id)
		if err != nil {
			return nil, err
		}
		if ref != "" {
			out[ref] = id
		}
	}
	return out, nil
}

// refHolding is the ref of the fenced span holding text, or else of the line
// holding it; the ref follows key in either form. Of two spans holding it, the
// one it sits nearer the start of wins. A conversation span opens with its own
// request and may quote a later one.
func refHolding(req model.Request, key, text string) string {
	var prompt strings.Builder
	for _, m := range req.Messages {
		prompt.WriteString(m.Content + "\n")
	}
	if marker, ok := promptfence.MarkerIn(req.System); ok {
		nearest, at := "", -1
		for _, span := range strings.Split(prompt.String(), "<"+marker+" "+key+`="`)[1:] {
			ref, body, found := strings.Cut(span, `">`)
			body, _, _ = strings.Cut(body, "</"+marker+">")
			if i := strings.Index(body, text); found && i >= 0 && (at < 0 || i < at) {
				nearest, at = ref, i
			}
		}
		if nearest != "" {
			return nearest
		}
	}
	for line := range strings.SplitSeq(prompt.String(), "\n") {
		if _, after, found := strings.Cut(line, key+" "); found && strings.Contains(line, text) {
			ref, _, _ := strings.Cut(after, ",")
			return strings.TrimRight(ref, ")")
		}
	}
	return ""
}

// shownText reads one record's shown text over the owner connection. The
// suites run one at a time on a reset database, so the row is this test's.
func shownText(ctx context.Context, query, id string) (text string, err error) {
	conn, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		return "", fmt.Errorf("connecting to read what a record shows: %w", err)
	}
	defer func() { err = errors.Join(err, conn.Close(context.WithoutCancel(ctx))) }()
	err = conn.QueryRow(ctx, query, id).Scan(&text)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return text, err
}

// inRefs answers as inner does, with each record id in the reply rewritten to
// the ref the request shows that record by. An id the request does not show
// is left as written, and the site's parser refuses it as it would a model's.
type inRefs struct {
	inner completer
	shown []shownBy
}

func (r inRefs) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	resp, err := r.inner.Complete(ctx, req)
	if err != nil {
		return resp, err
	}
	var failed error
	resp.Text = recordUUID.ReplaceAllStringFunc(resp.Text, func(id string) string {
		for _, by := range r.shown {
			ref, err := refOfRecord(ctx, req, by, id)
			if err != nil {
				failed = err
			}
			if ref != "" {
				return ref
			}
		}
		return id
	})
	return resp, failed
}

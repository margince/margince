// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"unicode/utf8"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Subject is the record a model call is ABOUT, as the site that made the call
// names it: the company a brief describes, the person a relationship brief is
// written for, the meeting a plan prepares.
//
// The router cannot know this on its own. It sees a task and a request, and
// the request is prompt text — so a rail line drawn from the router alone can
// only say "a company", which tells a reader that software is busy rather than
// what it is busy with. The site that assembled the input already holds the
// record's id and the name the product calls it elsewhere, and hands both down
// through the context so the occurrence carries them to the rail.
type Subject struct {
	// Ref is the record, typed by the kernel's own entity vocabulary
	// ("organization", "person", "activity"), which is the vocabulary the
	// projection stores as subject_type.
	Ref ids.Ref
	// Label is what the product calls that record elsewhere, already cut to
	// the wire's bound. Empty means the site had no name to give, and the rail
	// draws its unnamed sentence rather than an empty pair of quotes.
	Label string
}

// railSubjectLabelBound is the contract's cap on subject_label, applied before
// the wire rather than at it: the projection stores what it is handed, and a
// source handing over more than the column admits would fail the write instead
// of the read. Spelled here as well as beside the projection's reader because
// the two live in modules that may not import each other; the contract in
// api/internal-events.yaml owns the number.
const railSubjectLabelBound = 120

type subjectKey struct{}

// WithSubject names the record the model calls under ctx are about.
//
// It is the site's declaration, made once where the input was assembled, and
// every call the router serves under that context — the first try, the
// structured retry, the escalation — carries it. A label longer than the wire
// admits is cut at a rune boundary, never mid-character.
func WithSubject(ctx context.Context, ref ids.Ref, label string) context.Context {
	return context.WithValue(ctx, subjectKey{}, Subject{Ref: ref, Label: boundedLabel(label)})
}

// SubjectOf is the record the calls under ctx are about, when a site said so.
func SubjectOf(ctx context.Context) (Subject, bool) {
	s, ok := ctx.Value(subjectKey{}).(Subject)
	return s, ok && !s.Ref.ID.IsZero()
}

// boundedLabel cuts label to the wire's bound without splitting a rune: a
// company name is not ASCII, and a byte cut through an umlaut is invalid UTF-8
// that a JSON encoder replaces with a question mark in front of a reader.
func boundedLabel(label string) string {
	if utf8.RuneCountInString(label) <= railSubjectLabelBound {
		return label
	}
	runes := []rune(label)
	return string(runes[:railSubjectLabelBound])
}

// stamp writes the subject onto a rail announcement, or leaves the fields
// absent when there is none.
//
// Both the start and the settle stamp it, and that is not redundancy: the
// projection overwrites every column from the newest event, so a settle that
// arrived without the name would strip it from the row the moment the work
// finished — the rail would say "what I know about Acme" while it was working
// and "what I know about this company" once it was ready.
func (s Subject) stamp(p *crmcontracts.InternalEventAiTaskStateChanged) {
	if s.Ref.ID.IsZero() {
		return
	}
	subjectType := s.Ref.Type
	subjectID := openapi_types.UUID(s.Ref.ID)
	p.SubjectType = &subjectType
	p.SubjectId = &subjectID
	if s.Label != "" {
		label := s.Label
		p.SubjectLabel = &label
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package rewrite

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// ErrNoModel is a deployment with no model bound: there is no rewrite to fall
// back to, so the rep keeps their own words.
var ErrNoModel = errors.New("rewrite: no model is bound to rewrite text")

// ErrTooLong is text past what one answer can carry back.
var ErrTooLong = fmt.Errorf("rewrite: the text is longer than %d characters", MaxSourceRunes)

// ErrNothingToRewrite is a request with neither a subject nor a body.
var ErrNothingToRewrite = errors.New("rewrite: there is no text to rewrite")

// ErrVerbNotOffered is a verb the envelope does not offer.
var ErrVerbNotOffered = errors.New("rewrite: this envelope does not offer that verb")

// Ask is one press of a rewrite verb over the words on screen.
type Ask struct {
	Envelope Envelope
	Verb     Verb
	Subject  string
	Body     string
	Reader   textlang.Lang
	// About is the record the words are written on; every ask names one.
	About ids.Ref
}

// Admit answers whose data a record carries, or refuses a record the caller
// may not read. It must refuse an unknown record the same way.
type Admit func(ctx context.Context, about ids.Ref) (*model.Subjects, error)

// Result is the rewritten words, the language they are written in, and the
// sender's voice an email rewrite was written in.
type Result struct {
	Reply
	Language textlang.Lang
	Voice    draftvoice.Context
}

// Service rewrites one ask and writes nothing.
type Service struct {
	lane  ai.Completer
	admit Admit
	voice draftvoice.Reader
}

// NewService binds the rewrite to the rewrite lane, nil for none, and to
// the read check the record an ask names must pass.
func NewService(lane ai.Completer, admit Admit) *Service {
	return &Service{lane: lane, admit: admit}
}

// WithVoice binds the signed-in user's own Voice DNA, which styles an email
// rewrite as it styles the composer's drafts.
func (s *Service) WithVoice(reader draftvoice.Reader) *Service {
	s.voice = reader
	return s
}

// Rewrite rewrites the rep's words under the verb's own instruction.
func (s *Service) Rewrite(ctx context.Context, ask Ask) (Result, error) {
	// The buttons under a rep's own words; an agent writes its own.
	if err := auth.RequireHuman(ctx); err != nil {
		return Result{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return Result{}, err
	}
	about, err := s.admitRecord(ctx, ask.About)
	if err != nil {
		return Result{}, err
	}
	if !ask.Envelope.Admits(ask.Verb) {
		return Result{}, ErrVerbNotOffered
	}
	in := InputFor(ask)
	if in.Subject == "" && in.Body == "" {
		return Result{}, ErrNothingToRewrite
	}
	if in.TooLong() {
		return Result{}, ErrTooLong
	}
	if s.lane == nil {
		return Result{}, ErrNoModel
	}
	var voice draftvoice.Context
	if in.Envelope == EnvelopeEmail {
		voice = draftvoice.Load(ctx, s.voice, nil)
	}
	res, err := ai.Ask(ai.WithSubject(ctx, ask.About, in.Subject), s.lane, Request(in, about, voice), func(raw string) error {
		reply, err := Parse(raw)
		if err != nil {
			return err
		}
		return Check(in, reply)
	})
	if err != nil {
		return Result{}, err
	}
	// Judged again: a lane that cannot re-ask hands back the reply unchecked.
	reply, err := Parse(res.Text)
	if err == nil {
		err = Check(in, reply)
	}
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err)
	}
	return Result{Reply: reply, Language: in.Language, Voice: voice}, nil
}

// admitRecord refuses a record the caller may not read as one that does not
// exist. So the restriction answer below never tells the two apart.
func (s *Service) admitRecord(ctx context.Context, ref ids.Ref) (*model.Subjects, error) {
	if ref.ID.IsZero() || s.admit == nil {
		return nil, apperrors.ErrNotFound
	}
	about, err := s.admit(ctx, ref)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, apperrors.ErrNotFound
	}
	return about, err
}

// InputFor trims the rep's words and places their language: the text's own,
// else the reader's.
func InputFor(ask Ask) Input {
	in := Input{
		Envelope: ask.Envelope,
		Verb:     ask.Verb,
		Subject:  strings.TrimSpace(ask.Subject),
		Body:     strings.TrimSpace(ask.Body),
		Language: ask.Reader,
	}
	// An email's subject is the rep's to keep; only its body is rewritten.
	if in.Envelope == EnvelopeEmail {
		in.Subject = ""
	}
	if lang := textlang.OfMessage(in.Body, in.Subject); lang != textlang.Unknown {
		in.Language, in.Detected = lang, true
	}
	return in
}

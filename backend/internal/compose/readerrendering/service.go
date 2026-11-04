// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Activities is the content read a rendering is made from. It answers
// ErrNotFound for a row whose words the caller may not read.
type Activities interface {
	GetActivityContent(ctx context.Context, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error)
}

// Completer is the reader_rendering model lane.
type Completer = ai.Completer

// ErrNoModel is a deployment with no model bound: there is no rendering to
// fall back to.
var ErrNoModel = errors.New("readerrendering: no model is bound to render text")

// ErrTooLong is text past what one answer can carry back.
var ErrTooLong = fmt.Errorf("readerrendering: the text is longer than %d characters", MaxSourceRunes)

// Service renders one activity for one reader and writes nothing.
type Service struct {
	pool       *pgxpool.Pool
	activities Activities
	lane       Completer
	now        func() time.Time
}

// NewService binds the rendering to the content read, with no model lane
// until WithLane binds one.
func NewService(pool *pgxpool.Pool, activities Activities, now func() time.Time) *Service {
	return &Service{pool: pool, activities: activities, now: now}
}

// WithLane binds the reader_rendering lane.
func (s *Service) WithLane(lane Completer) *Service {
	s.lane = lane
	return s
}

// Render shows one activity in the reader's language. locale is the language
// the client asked for, nil for the reader's own.
func (s *Service) Render(ctx context.Context, id ids.ActivityID, locale *string) (crmcontracts.ActivityReaderRendering, error) {
	// A reading aid for the member at the screen; an agent reads the original.
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.ActivityReaderRendering{}, err
	}
	activity, err := s.activities.GetActivityContent(ctx, id, storekit.LiveOnly)
	if err != nil {
		return crmcontracts.ActivityReaderRendering{}, err
	}
	in, err := s.input(ctx, activity, locale)
	if err != nil {
		return crmcontracts.ActivityReaderRendering{}, err
	}
	// Before either answer: text past the limit is refused even when it is
	// already in the reader's language, so the two paths cannot disagree.
	if in.TooLong() {
		return crmcontracts.ActivityReaderRendering{}, ErrTooLong
	}
	out := crmcontracts.ActivityReaderRendering{
		ActivityId:     activity.Id,
		SourceLanguage: sourceLanguage(in.Source),
		TargetLanguage: crmcontracts.ActivityReaderRenderingTargetLanguage(in.Target),
		GeneratedAt:    s.now().UTC(),
	}
	if in.Source == in.Target {
		return unchanged(out, in), nil
	}
	if s.lane == nil {
		return crmcontracts.ActivityReaderRendering{}, ErrNoModel
	}
	res, err := ai.Ask(ai.WithSubject(ctx, id.Ref(), in.Subject), s.lane, Request(in), func(raw string) error {
		reply, err := Parse(raw)
		if err != nil {
			return err
		}
		return Check(in, reply)
	})
	if err != nil {
		return crmcontracts.ActivityReaderRendering{}, err
	}
	// Judged again: a lane that cannot re-ask hands back the reply unchecked.
	reply, err := Parse(res.Text)
	if err == nil {
		err = Check(in, reply)
	}
	if err != nil {
		return crmcontracts.ActivityReaderRendering{}, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err)
	}
	if err := s.stillReadable(ctx, activity); err != nil {
		return crmcontracts.ActivityReaderRendering{}, err
	}
	out.ModelUsed = &res.ServedModel
	if reply.AlreadyInTarget {
		return unchanged(out, in), nil
	}
	out.Subject, out.Body = present(reply.Subject), present(reply.Body)
	return out, nil
}

// input is the text a reader sees and the two languages, or ErrNotFound for a
// row with no words to render.
func (s *Service) input(ctx context.Context, activity crmcontracts.Activity, locale *string) (Input, error) {
	withheld := activity.ContentState != nil && *activity.ContentState == crmcontracts.ActivityContentStateWithheld
	if withheld || !rendersKind(activity.Kind) {
		return Input{}, apperrors.ErrNotFound
	}
	in := Input{Subject: strings.TrimSpace(deref(activity.Subject)), Body: ReadableBody(activity.Kind, deref(activity.Body))}
	if in.Subject == "" && in.Body == "" {
		return Input{}, apperrors.ErrNotFound
	}
	in.Target = s.target(ctx, locale)
	// The text as it stands wins over a stored label, which an older writer may
	// have read from other words. The label decides only where detection cannot.
	in.Source = textlang.OfMessage(in.Body, in.Subject)
	if in.Source == textlang.Unknown && activity.Language != nil && textlang.Known(string(*activity.Language)) {
		in.Source = textlang.Lang(*activity.Language)
	}
	return in, nil
}

// stillReadable reads the activity again after the model answered. A row the
// caller lost, archived or held meanwhile, or whose words changed, answers
// ErrNotFound. Else the rendering would show text the caller may no longer
// read, or words nobody wrote.
func (s *Service) stillReadable(ctx context.Context, before crmcontracts.Activity) error {
	after, err := s.activities.GetActivityContent(ctx, ids.From[ids.ActivityKind](ids.UUID(before.Id)), storekit.LiveOnly)
	if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	withheld := after.ContentState != nil && *after.ContentState == crmcontracts.ActivityContentStateWithheld
	if withheld || deref(after.Subject) != deref(before.Subject) || deref(after.Body) != deref(before.Body) ||
		!sameVersion(after.Version, before.Version) {
		return apperrors.ErrNotFound
	}
	return nil
}

func sameVersion(a, b *crmcontracts.RowVersion) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// ReadableBody is the text of an activity a rendering is made from. A mail
// loses its quoted history, which is somebody else's earlier message.
func ReadableBody(kind crmcontracts.ActivityKind, body string) string {
	body = strings.TrimSpace(body)
	if kind == crmcontracts.ActivityKindEmail {
		return textlang.CurrentMessage(body)
	}
	return body
}

// target is the language asked for, else the reader's own.
func (s *Service) target(ctx context.Context, locale *string) textlang.Lang {
	if locale != nil && textlang.Known(*locale) {
		return textlang.Lang(*locale)
	}
	return textlang.Lang(identity.ReaderLanguageForPrompt(ctx, s.pool))
}

// rendersKind names the kinds a member writes words into.
func rendersKind(kind crmcontracts.ActivityKind) bool {
	switch kind {
	case crmcontracts.ActivityKindNote, crmcontracts.ActivityKindEmail,
		crmcontracts.ActivityKindMeeting, crmcontracts.ActivityKindCall:
		return true
	default:
		return false
	}
}

// unchanged answers the original text, marked as already in the target.
func unchanged(out crmcontracts.ActivityReaderRendering, in Input) crmcontracts.ActivityReaderRendering {
	out.Unchanged = true
	out.Subject, out.Body = present(in.Subject), present(in.Body)
	return out
}

func sourceLanguage(lang textlang.Lang) *crmcontracts.ActivityReaderRenderingSourceLanguage {
	if lang == textlang.Unknown {
		return nil
	}
	out := crmcontracts.ActivityReaderRenderingSourceLanguage(lang)
	return &out
}

// present is nil for text the activity does not have, so the wire says null
// rather than an empty string somebody wrote.
func present(text string) *string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return &text
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

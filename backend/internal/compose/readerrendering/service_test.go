// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/promptlang"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// oneActivity answers the one activity a test hands it.
type oneActivity struct{ activity crmcontracts.Activity }

func (o oneActivity) GetActivityContent(context.Context, ids.ActivityID, storekit.ArchivedFilter) (crmcontracts.Activity, error) {
	return o.activity, nil
}

// recordingLane answers one canned reply and keeps what it was sent.
type recordingLane struct {
	reply string
	sent  []model.Request
}

func (l *recordingLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.sent = append(l.sent, req)
	return model.Response{Text: l.reply, ServedModel: "stub-model"}, nil
}

const germanBody = "Wir haben über die Verlängerung gesprochen. Sie möchten das Angebot bis Freitag sehen, " +
	"und der Einkauf ist nicht mehr gegen den Preis."

func note(kind crmcontracts.ActivityKind, body string, language *crmcontracts.ActivityLanguage) crmcontracts.Activity {
	subject := "Gespräch"
	return crmcontracts.Activity{
		Id: openapi_types.UUID(ids.NewV7()), Kind: kind, Subject: &subject, Body: &body, Language: language,
	}
}

func reader(locale string) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(), Locale: locale,
	})
}

func render(t *testing.T, ctx context.Context, activity crmcontracts.Activity, lane *recordingLane) (crmcontracts.ActivityReaderRendering, error) {
	t.Helper()
	now := clocktest.Now(t)
	svc := NewService(nil, oneActivity{activity}, func() time.Time { return now })
	if lane != nil {
		svc = svc.WithLane(lane)
	}
	return svc.Render(ctx, ids.From[ids.ActivityKind](ids.UUID(activity.Id)), nil)
}

const englishReply = `{"already_in_target": false, "subject": "Conversation",
  "body": "We talked about the renewal. They want to see the offer by Friday, and purchasing is no longer against the price."}`

func TestTheRenderingIsWrittenInTheReadersOwnLanguage(t *testing.T) {
	for _, locale := range []string{"en", "vi"} {
		lane := &recordingLane{reply: englishReply}
		if locale == "vi" {
			lane.reply = `{"already_in_target": false, "subject": "Cuộc trò chuyện", "body": "Chúng tôi đã nói về việc gia hạn hợp đồng."}`
		}
		got, err := render(t, reader(locale), note(crmcontracts.ActivityKindNote, germanBody, nil), lane)
		if err != nil {
			t.Fatalf("%s reader: %v", locale, err)
		}
		if string(got.TargetLanguage) != locale || len(lane.sent) != 1 {
			t.Fatalf("%s reader got target %s after %d calls", locale, got.TargetLanguage, len(lane.sent))
		}
		if !strings.Contains(lane.sent[0].System, promptlang.Rule(locale)) {
			t.Errorf("the %s reader's request does not carry the %s language rule", locale, locale)
		}
		if got.ModelUsed == nil || *got.ModelUsed != "stub-model" || got.Unchanged {
			t.Errorf("%s reader's rendering = %+v, want one written by the model", locale, got)
		}
	}
}

func TestTextAlreadyInTheReadersLanguageAsksNoModel(t *testing.T) {
	de := crmcontracts.ActivityLanguageDe
	lane := &recordingLane{reply: englishReply}
	got, err := render(t, reader("de"), note(crmcontracts.ActivityKindNote, germanBody, &de), lane)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Unchanged || got.Body == nil || *got.Body != germanBody || got.ModelUsed != nil {
		t.Errorf("rendering = %+v, want the original unchanged and no model named", got)
	}
	if len(lane.sent) != 0 {
		t.Errorf("a German note shown to a German reader made %d model calls", len(lane.sent))
	}
	if _, err := render(t, reader("de"), note(crmcontracts.ActivityKindNote, germanBody, &de), nil); err != nil {
		t.Errorf("with no model bound, text already in the reader's language still answers: %v", err)
	}
}

func TestAnUnknownLanguageTheModelJudgesUnchangedShowsTheOriginal(t *testing.T) {
	lane := &recordingLane{reply: `{"already_in_target": true, "subject": "", "body": ""}`}
	got, err := render(t, reader("de"), note(crmcontracts.ActivityKindNote, "Termin steht.", nil), lane)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Unchanged || *got.Body != "Termin steht." || got.SourceLanguage != nil {
		t.Errorf("rendering = %+v, want the original text marked unchanged", got)
	}
}

func TestARenderingIsRefusedBeforeAnyModelCall(t *testing.T) {
	withheld := note(crmcontracts.ActivityKindEmail, germanBody, nil)
	withheld.ContentState = new(crmcontracts.ActivityContentStateWithheld)
	cases := []struct {
		name     string
		ctx      context.Context
		activity crmcontracts.Activity
		lane     *recordingLane
		want     error
	}{
		{"an agent", principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, ID: "agent:test"}),
			note(crmcontracts.ActivityKindNote, germanBody, nil), &recordingLane{}, apperrors.ErrPermissionDenied},
		{"a withheld message", reader("en"), withheld, &recordingLane{}, apperrors.ErrNotFound},
		{"a task", reader("en"), note(crmcontracts.ActivityKindTask, germanBody, nil), &recordingLane{}, apperrors.ErrNotFound},
		{"no model", reader("en"), note(crmcontracts.ActivityKindNote, germanBody, nil), nil, ErrNoModel},
		{"text past the limit", reader("en"), note(crmcontracts.ActivityKindNote, strings.Repeat("ü ", MaxSourceRunes), nil),
			&recordingLane{}, ErrTooLong},
	}
	for _, c := range cases {
		_, err := render(t, c.ctx, c.activity, c.lane)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", c.name, err, c.want)
		}
		if c.lane != nil && len(c.lane.sent) != 0 {
			t.Errorf("%s reached the model", c.name)
		}
	}
}

func TestAMailIsRenderedWithoutItsQuotedHistory(t *testing.T) {
	body := germanBody + "\n\nAm 28.11.2026 schrieb Tom Weber <tom.weber@example.com>:\n> Unser Lager ist voll."
	lane := &recordingLane{reply: englishReply}
	if _, err := render(t, reader("en"), note(crmcontracts.ActivityKindEmail, body, nil), lane); err != nil {
		t.Fatal(err)
	}
	if prompt := lane.sent[0].Messages[0].Content; strings.Contains(prompt, "Lager") || !strings.Contains(prompt, "Verlängerung") {
		t.Errorf("the model was shown %q, want the current message without the quote", prompt)
	}
}

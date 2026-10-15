// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webhooks

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestABodyFormatChangeKeepsWhatItDoesNotName(t *testing.T) {
	tpl, form := `{"t": "{{ type }}"}`, contentTypeForm
	stored := Subscription{BodyTemplate: new(`{{ type }}`), BodyContentType: new(contentTypeText)}
	for _, tc := range []struct {
		name            string
		change          BodyFormatChange
		wantTemplate    *string
		wantContentType *string
	}{
		{"null template clears the content type too", BodyFormatChange{TemplateSet: true}, nil, nil},
		{"a content type alone re-checks the stored template", BodyFormatChange{ContentTypeSet: true, ContentType: &form}, stored.BodyTemplate, &form},
		{"a template alone keeps the stored content type", BodyFormatChange{TemplateSet: true, Template: new(`{{ data.x }}`)}, new(`{{ data.x }}`), stored.BodyContentType},
		{"both together", BodyFormatChange{TemplateSet: true, Template: &tpl, ContentTypeSet: true, ContentType: new(contentTypeJSON)}, &tpl, new(contentTypeJSON)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := storekit.NewPatch()
			if err := setBodyFormat(p, stored, &tc.change); err != nil {
				t.Fatal(err)
			}
			if got := resulting(p, fieldBodyTemplate, stored.BodyTemplate); !equalOptional(got, tc.wantTemplate) {
				t.Errorf("template = %q, want %q", deref(got), deref(tc.wantTemplate))
			}
			if got := resulting(p, fieldBodyContentType, stored.BodyContentType); !equalOptional(got, tc.wantContentType) {
				t.Errorf("content type = %q, want %q", deref(got), deref(tc.wantContentType))
			}
		})
	}
}

// resulting is the value a column holds after the patch: its new value when
// the patch sets it, the stored one when it does not.
func resulting(p *storekit.Patch, column string, stored *string) *string {
	v, set := p.After()[column]
	if !set {
		return stored
	}
	s, _ := v.(*string)
	return s
}

func TestABodyFormatThatCannotRenderIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored Subscription
		change BodyFormatChange
		field  string
	}{
		{"content type with no template", Subscription{}, BodyFormatChange{ContentTypeSet: true, ContentType: new(contentTypeText)}, fieldBodyContentType},
		{"stored text template switched to JSON", Subscription{BodyTemplate: new(`{{ type }}`)}, BodyFormatChange{ContentTypeSet: true, ContentType: new(contentTypeJSON)}, fieldBodyTemplate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := setBodyFormat(storekit.NewPatch(), tc.stored, &tc.change)
			if bad, ok := errors.AsType[*BadInputError](err); !ok || bad.Field != tc.field {
				t.Fatalf("err = %v, want a refusal on %s", err, tc.field)
			}
		})
	}
}

func TestEachSubscriptionGetsItsOwnBodyAndAFailureStaysItsOwn(t *testing.T) {
	envelope := []byte(`{"type": "deal.created", "data": {"name": "Acme", "blob": "` + strings.Repeat("y", 300) + `"}}`)
	named, oversized := `{"n": "{{ data.name }}"}`, strings.Repeat("{{ data.blob }}", 1000)
	rows := renderFor([]subCandidate{
		{id: ids.NewV7()},
		{id: ids.NewV7(), bodyTemplate: &named},
		{id: ids.NewV7(), bodyTemplate: &oversized, bodyContentType: new(contentTypeText)},
	}, envelope)
	if string(rows[0].payload) != string(envelope) || rows[0].renderError != "" || rows[0].contentType != nil {
		t.Errorf("a standard row must carry the envelope as is, got %q", rows[0].renderError)
	}
	if string(rows[1].payload) != `{"n": "Acme"}` {
		t.Errorf("templated row = %s", rows[1].payload)
	}
	if rows[2].payload != nil || !strings.Contains(rows[2].renderError, "256 KiB") {
		t.Errorf("an over-cap row must have no payload and say why, got %q", rows[2].renderError)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// MaxLabelRunes is the longest label a suggestion takes, the length a stored
// label translation may have (LabelTranslations in api/crm.yaml).
const MaxLabelRunes = 200

// labelEditors are the objects whose labels carry translations: a stage name
// and a custom field's name and options. Editing either is what a suggestion
// serves.
var labelEditors = []string{"pipeline", "custom_field"}

// SuggestLabel writes one admin-named label in target, through the same
// prompt and checks as an activity's rendering, with the label as its
// subject. It stores nothing; the admin saves what they accept.
func (s *Service) SuggestLabel(ctx context.Context, text string, target textlang.Lang) (crmcontracts.LabelTranslation, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.LabelTranslation{}, err
	}
	if !editsALabel(ctx) {
		return crmcontracts.LabelTranslation{}, apperrors.ErrPermissionDenied
	}
	if utf8.RuneCountInString(text) > MaxLabelRunes {
		return crmcontracts.LabelTranslation{}, ErrTooLong
	}
	in := Input{Subject: strings.TrimSpace(text), Target: target, Source: textlang.OfMessage("", text)}
	out := crmcontracts.LabelTranslation{Text: in.Subject, Language: crmcontracts.LabelTranslationLanguage(target)}
	if in.Source == in.Target {
		out.Unchanged = true
		return out, nil
	}
	if s.lane == nil {
		return crmcontracts.LabelTranslation{}, ErrNoModel
	}
	check := func(raw string) error {
		reply, err := Parse(raw)
		if err != nil {
			return err
		}
		return Check(in, reply)
	}
	res, err := ai.Ask(ctx, s.lane, Request(in), check)
	if err != nil {
		return crmcontracts.LabelTranslation{}, err
	}
	// Judged again: a lane that cannot re-ask hands back the reply unchecked.
	if err := check(res.Text); err != nil {
		return crmcontracts.LabelTranslation{}, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err)
	}
	reply, err := Parse(res.Text)
	if err != nil {
		return crmcontracts.LabelTranslation{}, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err)
	}
	out.ModelUsed = &res.ServedModel
	if reply.AlreadyInTarget {
		out.Unchanged = true
		return out, nil
	}
	out.Text = strings.TrimSpace(reply.Subject)
	return out, nil
}

func editsALabel(ctx context.Context) bool {
	for _, object := range labelEditors {
		if auth.Allows(ctx, object, principal.ActionUpdate) {
			return true
		}
	}
	return false
}

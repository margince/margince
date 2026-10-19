// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func (s *Store) controllerView(ctx context.Context, tx pgx.Tx, rendered Rendered, category commsauthz.Category) (Rendered, error) {
	lines, err := s.DisclosuresFor(ctx, tx, category)
	if err != nil {
		return Rendered{}, err
	}
	identity, err := settings.ApplyTx(ctx, tx, ControllerIdentity)
	if err != nil {
		return Rendered{}, err
	}
	disclosures := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Missing() {
			return Rendered{}, &ValidationError{Field: "controller_particulars", Reason: "complete the required company and privacy contact details before sending"}
		}
		disclosures = append(disclosures, strings.TrimSpace(line.Text))
	}
	return CompleteControllerMessage(rendered, identity.LegalName, disclosures), nil
}

func controllerAction(key string, words mailcopy.Copy) string {
	switch key {
	case TemplateRecordConfirmation:
		return words.ConfirmRecordAction
	case TemplateConsentConfirmation:
		return words.ConfirmConsentAction
	case TemplatePrivacyNotice:
		return words.NoticeAction
	}
	return ""
}

// CompleteControllerMessage attaches the same identity and disclosures in every presentation.
func CompleteControllerMessage(rendered Rendered, company string, disclosures []string) Rendered {
	if company != "" {
		rendered.Body = fmt.Sprintf(mailcopy.For(rendered.Language).ControllerFrom, mailcopy.OneLine(company)) + "\n\n" + rendered.Body
	}
	for _, line := range disclosures {
		rendered.Body += "\n\n" + line
	}
	return rendered
}

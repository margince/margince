// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// controllerView completes the installation's own mail to one recipient. The
// installation sends it, so the default legal entity is the sender.
func (s *Store) controllerView(ctx context.Context, tx pgx.Tx, rendered Rendered, category commsauthz.Category, recipient string) (Rendered, error) {
	lines, err := s.DisclosuresFor(ctx, tx, DisclosureQuery{Category: category, Recipients: []string{recipient}})
	if err != nil {
		return Rendered{}, err
	}
	identity, err := config.StoredApplyTx(ctx, tx, config.Bound(), ControllerIdentity)
	if err != nil {
		return Rendered{}, err
	}
	words := mailcopy.For(rendered.Language)
	disclosures := make([]string, 0, len(lines))
	for _, line := range lines {
		if line.Missing() {
			return Rendered{}, &ValidationError{Field: "controller_particulars", Reason: "complete the required company and privacy contact details before sending"}
		}
		disclosures = append(disclosures, labelledDisclosure(line, words))
	}
	return CompleteControllerMessage(rendered, identity.LegalName, disclosures), nil
}

// labelledDisclosure puts the label of its kind, in the mail's language, over
// one disclosed particular. A kind with no label is printed bare.
func labelledDisclosure(line DisclosureLine, words mailcopy.Copy) string {
	text := strings.TrimSpace(line.Text)
	var label string
	switch messagingrules.DisclosureKind(line.Kind) {
	case messagingrules.ControllerIdentity:
		label = words.DisclosureController
	case messagingrules.PrivacyContact:
		label = words.DisclosurePrivacyContact
	case messagingrules.ObjectionRoute:
		label = words.DisclosureObjection
	case messagingrules.AdvertiserContact:
		label = words.DisclosureAdvertiser
	}
	if label == "" {
		return text
	}
	return label + "\n" + text
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

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADraftIsHeldToTheContractsSubjectAndAddressCaps(t *testing.T) {
	anchor := MailDraftAnchor{Type: crmcontracts.MailDraftAnchorTypeActivity, ID: ids.NewV7()}
	for name, tc := range map[string]struct {
		content MailDraftContent
		field   string
	}{
		"a 998-character subject":        {MailDraftContent{Subject: strings.Repeat("é", 998)}, ""},
		"a 999-character subject":        {MailDraftContent{Subject: strings.Repeat("a", 999)}, "subject"},
		"a 320-character address":        {MailDraftContent{To: []string{strings.Repeat("é", 320)}}, ""},
		"a 321-character address in to":  {MailDraftContent{To: []string{strings.Repeat("a", 321)}}, "to"},
		"a 321-character address in cc":  {MailDraftContent{Cc: []string{"ok@example.com", strings.Repeat("a", 321)}}, "cc"},
		"a 321-character address in bcc": {MailDraftContent{Bcc: []string{strings.Repeat("a", 321)}}, "bcc"},
		"more than 100 addresses":        {MailDraftContent{To: make([]string, 101)}, "to"},
		"an empty composer":              {MailDraftContent{}, ""},
	} {
		err := validateDraft(anchor, tc.content)
		var refused *InvalidMailDraftError
		switch {
		case tc.field == "" && err != nil:
			t.Errorf("%s: refused with %v", name, err)
		case tc.field != "" && (!errors.As(err, &refused) || refused.Field != tc.field):
			t.Errorf("%s: %v, want a refusal naming %s", name, err, tc.field)
		}
	}
}

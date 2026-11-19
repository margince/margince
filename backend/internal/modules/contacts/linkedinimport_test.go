// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LinkedIn's company field is a member-edited headline, not a company name.
// These are real strings from a 5,064-row export: every one of them failed to
// reach an account that exists, because the tagline or the styling made the
// key something nobody stores a customer under.
func TestLinkedInCompanyHeadlinesReachTheAccount(t *testing.T) {
	for _, tc := range []struct{ headline, want string }{
		{".NFQ | Digital Creatives", "nfq"},
		{"tagtu | Result-Driven Business Travel", "tagtu"},
		{"basecom GmbH & Co. KG", "basecom"},
		{"CONRADFILM GmbH", "conradfilm"},
		{"MediaMarktSaturn", "mediamarktsaturn"},
		// A plain name survives untouched.
		{"Acme", "acme"},
	} {
		if got := NormalizeCompanyName(cleanLinkedInCompany(tc.headline)); got != tc.want {
			t.Errorf("%q → %q, want %q", tc.headline, got, tc.want)
		}
	}
}

// The company strings that reached no account on a real 5,064-row export, and
// the key each one needs. Every case here is a company that EXISTS in the CRM
// and was missed because a member wrote their employer the way contacts do.
func TestCompanyMatchKeysReachTheAccountsThatWereMissed(t *testing.T) {
	for _, tc := range []struct{ company, account string }{
		{"Wortfilter.de", "wortfilter"},
		{"The Sentry", "thesentry"},
		{"pinops consumer research", "pinops"},
		{"brickfox Multichannel eCommerce", "brickfox"},
		{"SIMIO GmbH & Co. KG | Geschäftsführender Gesellschafter", "simio"},
		{"100 DAYS software projects GmbH", "100 days software projects"},
		{"marcos software", "marcossoftware"},
	} {
		if !slices.Contains(companyMatchKeys(tc.company), tc.account) {
			t.Errorf("%q → %v, missing the key %q that reaches its account",
				tc.company, companyMatchKeys(tc.company), tc.account)
		}
	}
}

// The exact key is always first, so a company whose full name IS an account
// can never be resolved by a looser fallback to a different one.
func TestCompanyMatchKeysPutTheExactNameFirst(t *testing.T) {
	keys := companyMatchKeys("Acme Consulting GmbH")
	if len(keys) == 0 || keys[0] != "acme consulting" {
		t.Fatalf("keys = %v, want the exact name first", keys)
	}
}

// A string that names no company must produce nothing an account can match.
// These are real values from the same export: a member between jobs, a
// placeholder, a description. Matching them would attach a colleague's whole
// network to whichever account happened to share a word.
func TestCompanyMatchKeysRefuseWhatIsNotACompany(t *testing.T) {
	for _, notACompany := range []string{"", "   ", ".", "|", "..."} {
		if keys := companyMatchKeys(notACompany); len(keys) != 0 {
			t.Errorf("%q → %v, want no keys at all", notACompany, keys)
		}
	}
}

// A name's legal form is the tail NormalizeCompanyName strips, in one spelling.
func TestALegalFormIsReadInOneSpelling(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"Acme GmbH", "gmbh"},
		{"Acme G.m.b.H.", "gmbh"},
		{"SIMIO GmbH & Co. KG", "gmbh & co kg"},
		{"Acme Inc", "inc"},
		{"Acme", ""},
		{"Co", ""},
	} {
		if got := companyLegalForm(tc.name); got != tc.want {
			t.Errorf("companyLegalForm(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// Two legal forms name two companies. A form missing on one side does not.
func TestOnlyADifferentLegalFormNamesAnotherEntity(t *testing.T) {
	for _, tc := range []struct {
		employer, account string
		another           bool
	}{
		{"Acme GmbH", "Acme Inc", true},
		{"Acme GmbH", "Acme", false},
		{"Acme", "Acme Inc", false},
		{"Acme GmbH", "ACME G.m.b.H.", false},
	} {
		account := companyCandidate{legalForm: companyLegalForm(tc.account)}
		if got := account.namesAnotherEntity(companyLegalForm(tc.employer)); got != tc.another {
			t.Errorf("%q at %q: names another entity = %v, want %v", tc.employer, tc.account, got, tc.another)
		}
	}
}

// A LinkedIn network is one member's own. A caller holding the grant but naming
// no member is refused, and so is a refusal of a match recorded for nobody.
func TestALinkedInImportOrRefusalWithoutAMemberIsDenied(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{"contact": {Create: true}}, RowScope: principal.RowScopeAll,
		},
	})
	_, err := (&Store{}).ImportLinkedInConnections(ctx, strings.NewReader("First Name,Last Name\n"))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an import naming no member = %v, want ErrPermissionDenied", err)
	}
	err = RecordLinkedInMatchRefusedTx(context.Background(), nil, ids.NewV7(), ids.Nil, ids.NewV7())
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a refusal for no owner = %v, want ErrPermissionDenied", err)
	}
}

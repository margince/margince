// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/platform/messagingpolicy"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// A pack cannot import this module, so the paired fixtures its policy names
// run here, through the engine's own draft, subject and footer checks.

// extensionsRoot is where the packs sit, seen from this package.
const extensionsRoot = "../../../../extensions"

// compiledPolicies compiles every unit's policy folder from its sources, as
// gen-messaging-rules does, keyed by the jurisdiction each pack declares.
func compiledPolicies(t *testing.T) map[string]messagingpolicy.Policy {
	t.Helper()
	folders, err := filepath.Glob(filepath.Join(extensionsRoot, "*", "policy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(folders) == 0 {
		t.Fatalf("no unit under %s has a policy folder; the packs moved", extensionsRoot)
	}
	out := map[string]messagingpolicy.Policy{}
	for _, folder := range folders {
		unit, err := messagingpolicy.CompileUnit(os.DirFS(folder))
		if err != nil {
			t.Fatalf("%s: %v", folder, err)
		}
		for _, policy := range unit.Policies {
			country := string(policy.Rules.Jurisdiction)
			if _, twice := out[country]; twice {
				t.Fatalf("%s declares %s, which another pack already declares", folder, country)
			}
			out[country] = policy
		}
	}
	return out
}

// shippedPacks are the rules each shipped pack declares.
func shippedPacks(t *testing.T) map[string]messagingrules.Rules {
	t.Helper()
	out := map[string]messagingrules.Rules{}
	for country, policy := range compiledPolicies(t) {
		out[country] = policy.Rules
	}
	return out
}

func TestEveryPolicyFixturePairSplitsOnItsFinding(t *testing.T) {
	t.Parallel()
	for country, policy := range compiledPolicies(t) {
		if len(policy.Fixtures) == 0 {
			t.Errorf("%s names no fixture pair", country)
		}
		for _, pair := range policy.Fixtures {
			if got := fixtureFindings(t, country, pair.Triggers); !slices.Contains(got, pair.Finding) {
				t.Errorf("%s/%s: %s should raise %s and raised %v", country, pair.Layer, pair.Triggers.Path, pair.Finding, got)
			}
			if got := fixtureFindings(t, country, pair.Clears); slices.Contains(got, pair.Finding) {
				t.Errorf("%s/%s: %s should not raise %s and raised %v", country, pair.Layer, pair.Clears.Path, pair.Finding, got)
			}
		}
	}
}

// fixtureFindings answers what the engine finds in one fixture's send to a
// recipient under the pack of country.
func fixtureFindings(t *testing.T, country string, f messagingpolicy.Fixture) []string {
	t.Helper()
	particulars := ControllerParticulars{
		LegalName: f.Particulars.LegalName, PostalAddress: f.Particulars.PostalAddress,
		PrivacyContact: f.Particulars.PrivacyContact, ObjectionRoute: f.Particulars.ObjectionRoute,
		AdvertiserContact: f.Particulars.AdvertiserContact,
	}
	for _, tr := range f.Translations {
		particulars.Translations = append(particulars.Translations, ParticularsTranslation{
			Language: tr.Language, LegalName: tr.LegalName, PostalAddress: tr.PostalAddress,
			PrivacyContact: tr.PrivacyContact, ObjectionRoute: tr.ObjectionRoute, AdvertiserContact: tr.AdvertiserContact,
		})
	}
	return sendFindings(t, shippedSend{
		country: country, sender: f.Sender, subject: f.Subject, body: f.Body, advertising: f.Advertising,
		particulars: particulars, mailLanguage: f.MailLanguage, language: f.Language,
	})
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// fixedRetention answers the rules one test states, standing in for the
// retention settings compose reads them from.
type fixedRetention []RetentionRule

func (f fixedRetention) ActingRetention(context.Context) ([]RetentionRule, error) { return f, nil }

func noticeFor(t *testing.T, e *channelConsentEnv, ref ConfirmRef) PrivacyInformation {
	t.Helper()
	ref.Kind = LinkPrivacyNotice
	info, err := e.store.PrivacyInformationFor(e.ctx, ref)
	if err != nil {
		t.Fatalf("assembling the disclosure: %v", err)
	}
	return info
}

func TestThePrivacyPageNamesTheControllerTheInstallationStated(t *testing.T) {
	e := setupChannelConsent(t)
	stated := ControllerParticulars{
		LegalName: "Beispiel GmbH", PostalAddress: "Hauptstr. 1\n10115 Berlin",
		PrivacyContact: "privacy@beispiel.test", ObjectionRoute: "Reply STOP",
	}
	stateParticulars(t, e, stated)

	info := noticeFor(t, e, ConfirmRef{ContactID: e.contact})
	if info.Controller != stated {
		t.Errorf("the page names controller %+v, want the stated particulars %+v", info.Controller, stated)
	}
}

func TestEachPurposeIsDisclosedWithTheBasisOfItsClass(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_purpose (key, label, class) VALUES
		  ('talks', 'Talks', 'business_correspondence'),
		  ('service', 'Service', 'transactional'),
		  ('news', 'News', 'marketing')`); err != nil {
		t.Fatal(err)
	}

	info := noticeFor(t, e, ConfirmRef{ContactID: e.contact})
	if len(info.PurposeBases) != len(info.PurposeKeys) {
		t.Fatalf("%d bases for %d purposes", len(info.PurposeBases), len(info.PurposeKeys))
	}
	want := map[string]string{"talks": "legitimate_interests", "service": "contract", "news": "consent"}
	for i, key := range info.PurposeKeys {
		if basis, stated := want[key]; stated && info.PurposeBases[i] != basis {
			t.Errorf("purpose %q is disclosed on %q, want %q", key, info.PurposeBases[i], basis)
		}
	}
}

func TestTheDataCategoriesAreWhatTheRecordHolds(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)

	info := noticeFor(t, e, ConfirmRef{ContactID: e.contact})
	if want := []string{"name", "email"}; !slices.Equal(info.DataCategories, want) {
		t.Fatalf("categories %v, want %v: the fixture holds a name and an address and nothing else", info.DataCategories, want)
	}

	// The installation's own notice is not correspondence with the reader.
	linkActivity(t, e, "system_notice")
	if info := noticeFor(t, e, ConfirmRef{ContactID: e.contact}); slices.Contains(info.DataCategories, "correspondence") {
		t.Error("the notice mail itself was disclosed as correspondence")
	}
	linkActivity(t, e, "human")
	if info := noticeFor(t, e, ConfirmRef{ContactID: e.contact}); !slices.Contains(info.DataCategories, "correspondence") {
		t.Errorf("categories %v leave out the mail exchanged with the reader", info.DataCategories)
	}
}

func linkActivity(t *testing.T, e *channelConsentEnv, origin string) {
	t.Helper()
	activity := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, direction, source, occurred_at, captured_by, origin)
		VALUES ($1, 'email', 'outbound', 'gmail', now(), 'human:x', $2)`, activity, origin); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
		activity, e.contact); err != nil {
		t.Fatal(err)
	}
}

func TestTheRetentionShownIsTheActingRulesThatReachTheReader(t *testing.T) {
	e := setupChannelConsent(t)
	e.store = e.store.WithRetentionReader(fixedRetention{
		{Scope: "activity", RetainDays: 1095, Action: "archive"},
		{Scope: "deal/lost", RetainDays: 1825, Action: "archive"},
		{Scope: "lead/unconverted", RetainDays: 365, Action: "archive"},
		{Scope: "contact/no_consent_no_deal", RetainDays: 730, Action: "anonymize"},
	})

	contact := noticeFor(t, e, ConfirmRef{ContactID: e.contact})
	if got := scopesOf(contact.Retention); !slices.Equal(got, []string{"activity", "contact/no_consent_no_deal"}) {
		t.Errorf("a contact is told of %v, want only the rules that reach a contact", got)
	}

	lead := ids.New[ids.LeadKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO lead (id, full_name, email, phone, source, captured_by)
		VALUES ($1, 'Lena Lead', 'lena@lead.test', '+49 30 1', 'website', 'human:x')`, lead); err != nil {
		t.Fatal(err)
	}
	info := noticeFor(t, e, ConfirmRef{LeadID: lead})
	if got := scopesOf(info.Retention); !slices.Equal(got, []string{"activity", "lead/unconverted"}) {
		t.Errorf("a lead is told of %v, want only the rules that reach a lead", got)
	}
	if want := []string{"name", "email", "phone"}; !slices.Equal(info.DataCategories, want) {
		t.Errorf("a lead's categories are %v, want %v", info.DataCategories, want)
	}
}

func scopesOf(rules []RetentionRule) []string {
	out := []string{}
	for _, rule := range rules {
		out = append(out, rule.Scope)
	}
	return out
}

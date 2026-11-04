// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"slices"
	"testing"
)

// fixedHeld answers what one test states, standing in for the privacy module
// compose reads it from.
type fixedHeld struct {
	held NoticeHeld
	seen NoticeSubject
}

func (f *fixedHeld) NoticeHeld(_ context.Context, subject NoticeSubject) (NoticeHeld, error) {
	f.seen = subject
	return f.held, nil
}

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

func TestThePageNamesOnlyTheUsesOnRecordWithTheirRecordedBasis(t *testing.T) {
	e := setupChannelConsent(t)
	reader := &fixedHeld{held: NoticeHeld{
		Categories: []string{"name", "postal_address"},
		Uses: []NoticeUse{
			{PurposeKey: "newsletter", Basis: "consent"},
			{PurposeKey: "business_correspondence", Basis: "subject_initiated_correspondence"},
		},
	}}
	e.store = e.store.WithNoticeHeldReader(reader)

	info := noticeFor(t, e, ConfirmRef{ContactID: e.contact})
	if reader.seen.ContactID != e.contact {
		t.Fatalf("the reader was asked about %+v, want the link's own contact", reader.seen)
	}
	if !slices.Equal(info.PurposeKeys, []string{"newsletter", "business_correspondence"}) ||
		!slices.Equal(info.PurposeBases, []string{"consent", "subject_initiated_correspondence"}) {
		t.Errorf("the page discloses %v on %v, want only the two uses on record", info.PurposeKeys, info.PurposeBases)
	}
	if info.Purposes[0] != "Newsletter" || info.Purposes[1] != "business_correspondence" {
		t.Errorf("purposes are named %v, want the published label, or the key where none is published", info.Purposes)
	}
	if !slices.Equal(info.DataCategories, []string{"name", "postal_address"}) {
		t.Errorf("categories %v, want the reader's", info.DataCategories)
	}
}

func TestAnUnwiredReaderDisclosesNothingItCannotBack(t *testing.T) {
	e := setupChannelConsent(t)

	info := noticeFor(t, e, ConfirmRef{ContactID: e.contact})
	if len(info.Purposes)+len(info.DataCategories)+len(info.Retention) != 0 {
		t.Errorf("an unwired page claims purposes %v, categories %v, retention %v",
			info.Purposes, info.DataCategories, info.Retention)
	}
}

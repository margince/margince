// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The privacy notice: a message that tells somebody what is held about them and
// asks for nothing.
//
// Everything here is about the difference between that and the record
// confirmation beside it. The record confirmation discharges the same Art. 14
// duty, shows the contact's employer, phone, address and provenance trail, and
// asks whether they want to hear from us. This asks nothing, shows less, and
// reaches contacts the other one cannot.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// TestThePrivacyNoticeAsksForNoSubscription is the whole reason this template
// exists beside the record confirmation.
//
// Art. 14 requires telling somebody we hold their data. It requires no answer.
// A message that discharges that duty and also asks whether they want marketing
// puts a commercial question inside a legal obligation, which is the
// arrangement a supervisory authority reads as consent obtained under pressure.
func TestThePrivacyNoticeAsksForNoSubscription(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	stager, vault := &recordingStager{}, &recordingVault{}
	e.store = e.store.WithConfirmationLane(stager, vault, "https://crm.example.test/")

	if _, err := e.store.IssuePrivacyNotice(e.ctx, e.contact); err != nil {
		t.Fatalf("minting the privacy notice: %v", err)
	}
	if stager.calls != 1 {
		t.Fatalf("the notice staged %d mails, want 1", stager.calls)
	}

	sent := stager.seen
	if sent.Rendered.Key != TemplatePrivacyNotice {
		t.Errorf("the notice went out under template %q, want %q", sent.Rendered.Key, TemplatePrivacyNotice)
	}
	// The words themselves, because this is the assertion that survives
	// somebody "improving" the copy: a notice that starts asking a question
	// stops being a notice.
	//
	// The LINK PLACEHOLDER is stripped first. It is spelled
	// {{confirmation-link}} for every controller mail — the lane substitutes it
	// and the name is comms' business, not this message's — so leaving it in
	// would fail on the word "confirmation" appearing in a token no reader ever
	// sees.
	body := strings.ReplaceAll(
		sent.Rendered.Subject+"\n"+sent.Rendered.Body, linkPlaceholder, "")
	for _, asking := range []string{
		"whether you want", "want to hear from us", "subscribe", "confirm",
	} {
		if containsFold(body, asking) {
			t.Errorf("the privacy notice says %q:\n%s\n\nIt discharges a duty to TELL "+
				"somebody something and must ask them for nothing — a marketing question "+
				"inside a legal obligation is consent obtained under pressure", asking, body)
		}
	}
}

// containsFold is a case-insensitive substring test, spelled here because the
// assertion above is about words a human reads rather than tokens.
func containsFold(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		indexFold(haystack, needle) >= 0
}

func indexFold(haystack, needle string) int {
	lowerHay, lowerNeedle := []rune(haystack), []rune(needle)
	fold := func(rs []rune) string {
		out := make([]rune, len(rs))
		for i, r := range rs {
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			out[i] = r
		}
		return string(out)
	}
	h, n := fold(lowerHay), fold(lowerNeedle)
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}

// TestTheNoticeReachesAContactWhoAskedUsToStop is the hole this closes.
//
// A subject request stops nearly everything, and the disclosure duty survives
// it — Art. 18(2) and the notice duties are exactly what a restriction leaves
// room for. Only CategoryPrivacyNotice survives that stop in the engine, and
// before this template nothing carried it. So the contacts most likely to have
// asked us to stop were owed a disclosure the product could not deliver.
func TestTheNoticeReachesAContactWhoAskedUsToStop(t *testing.T) {
	e := setupChannelConsent(t)

	if !survivesARestriction(commsauthz.CategoryPrivacyNotice) {
		t.Fatal("the privacy-notice category no longer survives a restriction; this whole " +
			"route exists because it does")
	}
	if survivesARestriction(commsauthz.CategoryRecordConfirmation) {
		t.Fatal("the record confirmation now survives a restriction too, which would make " +
			"this template unnecessary — check whether that is intended")
	}
	// And the template the notice link actually carries is the surviving one.
	if got := controllerTemplates[TemplatePrivacyNotice].category; got != commsauthz.CategoryPrivacyNotice {
		t.Errorf("the notice template carries category %q, want %q: a notice under any other "+
			"category is refused for exactly the contacts it exists to reach",
			got, commsauthz.CategoryPrivacyNotice)
	}
	_ = e
}

// TestTheNoticeDischargesTheDuty. Sending it is what moves the case, the same
// way the record confirmation does — otherwise the route would be a name in
// allowed_routes that no writer honours, which is the defect the discharge path
// was built to end.
func TestTheNoticeDischargesTheDuty(t *testing.T) {
	e := setupChannelConsent(t)
	owed := seedNoticeCase(t, e, "purchased_or_imported",
		[]string{noticeRoutePrivacyNotice})
	delivery := seedDelivery(t, e)

	var moved int
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		moved, err = dischargeNoticeCases(e.ctx, tx, e.contact,
			noticeRoutePrivacyNotice, time.Now(), delivery)
		return err
	}); err != nil {
		t.Fatalf("discharging by notice: %v", err)
	}
	if moved != 1 {
		t.Fatalf("the notice discharged %d duties, want 1", moved)
	}
	if state, _ := noticeCaseState(t, e, owed); state != string(NoticeQueued) {
		t.Errorf("the case rests in %q after its notice was sent, want %q", state, NoticeQueued)
	}
}

// TestAnArt14DutyNamesTheNoticeRoute. The duty has to OFFER the route, or a
// case whose contact has asked us to stop still cannot be discharged.
func TestAnArt14DutyNamesTheNoticeRoute(t *testing.T) {
	duty, owed := DutyFor("purchased_or_imported")
	if !owed {
		t.Fatal("a bought contact owes no duty; the fixture this test reads is gone")
	}
	var named bool
	for _, route := range duty.Routes {
		if route == noticeRoutePrivacyNotice {
			named = true
		}
	}
	if !named {
		t.Errorf("an Art. 14 duty names routes %v and not the privacy notice: the notice is "+
			"the only route that reaches a contact who asked us to stop, so a duty without "+
			"it is undischargeable for exactly those contacts", duty.Routes)
	}
}

// TestTheNoticePageShowsTheSourceAndTheRightsAndNoFile.
//
// What Art. 14 requires: that we hold it, where it came from, what we use it
// for, the rights. What it does not require, and what an unbidden link should
// not disclose: the contact's name, employer, address, phone and provenance
// trail — all of which the record page carries.
func TestTheNoticePageShowsTheSourceAndTheRightsAndNoFile(t *testing.T) {
	e := setupChannelConsent(t)
	acquired := time.Now().Add(-90 * 24 * time.Hour).UTC().Truncate(time.Microsecond)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
		VALUES ($1, 'purchased_or_imported', $2, 'test')`, e.contact, acquired); err != nil {
		t.Fatalf("seeding the acquisition: %v", err)
	}

	info, err := e.store.PrivacyInformationFor(e.ctx, ConfirmRef{
		Kind: LinkPrivacyNotice, ContactID: e.contact,
	})
	if err != nil {
		t.Fatalf("assembling the disclosure: %v", err)
	}
	if info.AcquiredAs != "purchased_or_imported" {
		t.Errorf("the notice says the data came from %q, want purchased_or_imported: "+
			"Art. 14(2)(f) requires naming the source", info.AcquiredAs)
	}
	if info.AcquiredAt == nil || !info.AcquiredAt.Equal(acquired) {
		t.Errorf("the notice dates the acquisition %v, want %v", info.AcquiredAt, acquired)
	}
	if len(info.Rights) == 0 {
		t.Error("the notice names no rights: Art. 14(2)(c)-(e) requires naming them")
	}
	// The purposes this installation publishes, which the fixture seeds two of.
	if len(info.Purposes) == 0 {
		t.Error("the notice names no purposes: what the data is used for is the disclosure")
	}
}

// TestAnUndatedAcquisitionStillDisclosesItsSource. A contact created before the
// doors recorded a time has no occurred_at, and the disclosure is still owed.
func TestAnUndatedAcquisitionStillDisclosesItsSource(t *testing.T) {
	e := setupChannelConsent(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO contact_acquisition_evidence (contact_id, kind, captured_by)
		VALUES ($1, 'referral', 'test')`, e.contact); err != nil {
		t.Fatalf("seeding the acquisition: %v", err)
	}

	info, err := e.store.PrivacyInformationFor(e.ctx, ConfirmRef{
		Kind: LinkPrivacyNotice, ContactID: e.contact,
	})
	if err != nil {
		t.Fatalf("assembling the disclosure: %v", err)
	}
	if info.AcquiredAs != "referral" {
		t.Errorf("the notice says %q, want referral", info.AcquiredAs)
	}
	// captured_at stands in, so the page can still say when — it is the row's
	// own write, which is the best this installation knows.
	if info.AcquiredAt == nil {
		t.Error("an undated acquisition disclosed no time at all; captured_at stands in")
	}
}

// TestAContactWithNoAcquisitionRowSaysSoRatherThanNothing. A record predating
// the doors that write evidence has none, and "we do not know how you came to
// be here" is itself the honest disclosure.
func TestAContactWithNoAcquisitionRowSaysSoRatherThanNothing(t *testing.T) {
	e := setupChannelConsent(t)

	info, err := e.store.PrivacyInformationFor(e.ctx, ConfirmRef{
		Kind: LinkPrivacyNotice, ContactID: e.contact,
	})
	if err != nil {
		t.Fatalf("assembling the disclosure: %v", err)
	}
	if info.AcquiredAs != acquiredUnknownLegacy {
		t.Errorf("a contact with no acquisition evidence discloses %q, want %q: an empty "+
			"string renders as a gap, and the vocabulary has a word for not knowing",
			info.AcquiredAs, acquiredUnknownLegacy)
	}
}

// TestTheDisclosureIsAssembledOnlyForANoticeLink. The kind is checked rather
// than trusted: a reference of another kind reaching this assembler would serve
// the Art. 14 page to somebody whose mail described something else.
func TestTheDisclosureIsAssembledOnlyForANoticeLink(t *testing.T) {
	e := setupChannelConsent(t)

	if _, err := e.store.PrivacyInformationFor(e.ctx, ConfirmRef{
		Kind: LinkRecordConfirmation, ContactID: e.contact,
	}); err == nil {
		t.Error("a RECORD link assembled the privacy disclosure: the two pages answer " +
			"different mails, and each must be reached only by the link that promised it")
	}
	_ = ids.UUID{}
}

// TestTheNoticeLinkTakesNoAnswer is the strictest property here.
//
// The mail closes with "you do not need to reply or do anything", so the page
// it opens may accept nothing: not a correction, not an erasure request, not a
// marketing choice. A reader who wants any of those has the rights the notice
// itself names, through doors that ask them to identify themselves.
//
// This is the one link a contact who asked us to stop still receives. If it
// accepted a marketing answer it would become the re-engagement surface their
// stop exists to prevent, reached by a message they cannot refuse.
func TestTheNoticeLinkTakesNoAnswer(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   ConfirmSubmission
	}{
		{"a correction", ConfirmSubmission{Corrections: map[string]string{"full_name": "New"}}},
		{"an erasure request", ConfirmSubmission{RequestErasure: true}},
		{"a marketing answer", ConfirmSubmission{MarketingChoice: "yes"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := refuseWiderThanTheMail(LinkPrivacyNotice, tc.in)
			var verr *ValidationError
			if !errorsAs(err, &verr) {
				t.Fatalf("a notice link accepted %s (err %v): the mail asked for nothing, "+
					"so the page may take nothing", tc.name, err)
			}
		})
	}
	// AN EMPTY ONE TOO, and that arm is the one Codex found. There is no form
	// on the notice page, so nothing legitimate posts to it — and the token is
	// spent before the kind is known, so an empty POST would burn the link.
	// The reader would lose the disclosure they were sent, and the page would
	// answer not-found for a duty the installation still owes them.
	if err := refuseWiderThanTheMail(LinkPrivacyNotice, ConfirmSubmission{}); err == nil {
		t.Error("an empty submission on a notice link was accepted: it spends the token, " +
			"so a stray request costs the reader a disclosure that cannot be re-opened")
	}
}

// errorsAs is errors.As, spelled here so the table above reads as one line.
func errorsAs(err error, target **ValidationError) bool {
	return err != nil && errors.As(err, target)
}

// TestTheNoticeIsEvidencedByItsOwnLink is the defect the template census found.
//
// The engine evidences a controller mail by the live link it carries, and
// confirmKindFor mapped only the two older kinds — on the stated ground that a
// privacy notice carries no link. It does now. Without the mapping every notice
// this installation sent resolved to "no evidence" and was refused, so the
// feature would have shipped unable to send a single message.
func TestTheNoticeIsEvidencedByItsOwnLink(t *testing.T) {
	kind, ok := confirmKindFor(commsauthz.CategoryPrivacyNotice)
	if !ok {
		t.Fatal("the privacy-notice category has no confirm kind: the engine then finds no " +
			"evidence for a notice and refuses every one of them")
	}
	if kind != LinkPrivacyNotice {
		t.Errorf("the notice category is evidenced by %q links, want %q", kind, LinkPrivacyNotice)
	}
}

// TestTheEngineSupportsANoticeOnItsOwnLink is the defect Codex found, and the
// one that would have shipped a feature incapable of sending anything.
//
// The engine evidences a controller mail by resolving its category through
// authorizeValidators. That switch listed the two older confirmation
// categories, so a privacy notice fell to the default arm, resolved to "no
// evidence", and was refused at transmit. Mapping confirmKindFor was necessary
// and not sufficient: the dispatch above it never reached the mapping.
//
// Asserted through validate, which is the function that DISPATCHES. An earlier
// version of this test called validateConfirmation directly and passed with the
// dispatch reverted — it proved the mapping and skipped the switch that was the
// actual defect. Mutation testing is what showed it.
func TestTheEngineSupportsANoticeOnItsOwnLink(t *testing.T) {
	e := setupChannelConsent(t)
	seedSubjectAddress(t, e)
	e.store = e.store.WithConfirmationLane(
		&recordingStager{}, &recordingVault{}, "https://crm.example.test/")

	if _, err := e.store.IssuePrivacyNotice(e.ctx, e.contact); err != nil {
		t.Fatalf("minting the notice: %v", err)
	}

	var got resolution
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		gate := NewGate(e.store)
		rules, err := e.store.packRulesFor(e.ctx, tx)
		if err != nil {
			return err
		}
		got, err = gate.validate(e.ctx, tx, commsauthz.Request{},
			subjectRef{Kind: entityContact, ID: e.contact.String()},
			commsauthz.CategoryPrivacyNotice, rules)
		return err
	}); err != nil {
		t.Fatalf("resolving the notice: %v", err)
	}
	if !got.Supported {
		t.Fatalf("a notice with a live link resolves unsupported (%q): every notice this "+
			"installation sends would then be refused for having no evidence, which is a "+
			"feature that cannot send a single message", got.Reason)
	}
}

// TestAStoppedContactStillGetsTheirNotice is the claim the whole feature rests
// on, checked against the engine's own rule rather than restated.
//
// A subject request stops nearly everything. The disclosure duty survives it,
// and before this template nothing carried a category that did — so the contacts
// most likely to have asked us to stop were owed a disclosure the product could
// not deliver.
func TestAStoppedContactStillGetsTheirNotice(t *testing.T) {
	if !suppressionBinds("subject_request", commsauthz.CategoryRecordConfirmation) {
		t.Fatal("a subject request no longer stops the record confirmation, which would " +
			"make this template unnecessary — check whether that is intended")
	}
	if suppressionBinds("subject_request", commsauthz.CategoryPrivacyNotice) {
		t.Error("a subject request stops the privacy notice: the disclosure duty survives a " +
			"stop, so refusing it leaves the duty owed and undeliverable to exactly the " +
			"contacts who asked")
	}
	// A DEAD MAILBOX still refuses it, and must: a notice nobody can receive
	// discharges nothing, and pretending otherwise would mark the duty handled.
	if !suppressionBinds("hard_bounce", commsauthz.CategoryPrivacyNotice) {
		t.Error("a hard-bounced address accepts the privacy notice: the mailbox is gone, so " +
			"the message cannot arrive and the duty is not discharged by sending it")
	}
}

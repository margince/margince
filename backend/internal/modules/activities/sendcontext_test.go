// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a caller may claim about their own send, and what they may not.

import (
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Claiming nothing is the ordinary case and must stay free: a rep answering a
// message in the compose window names no category, and the engine resolves one
// from the origin. A decoder that refused an absent claim would refuse every
// reply this product sends.
func TestClaimingNoCategoryIsAccepted(t *testing.T) {
	got, err := sendContextFrom(nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("an absent claim was refused: %v", err)
	}
	if got.category != "" {
		t.Errorf("category = %q, want empty — the engine resolves it, not the decoder", got.category)
	}
}

// The five categories that serve the recipient are the ones a hard suppression
// does not stop. They belong to the installation's own controller mail, which
// rides a registered template. A send that could claim one could dress
// marketing as a security warning and reach somebody who has objected — so
// every one of them is refused here, at the door.
func TestAServeTheSubjectCategoryCannotBeClaimedByASend(t *testing.T) {
	reserved := []commsauthz.Category{
		commsauthz.CategorySecurityNotice,
		commsauthz.CategoryPrivacyNotice,
		commsauthz.CategoryOptoutConfirmation,
		commsauthz.CategoryConsentConfirmation,
		commsauthz.CategoryRecordConfirmation,
	}
	for _, c := range reserved {
		claimed := string(c)
		if _, err := sendContextFrom(&claimed, nil, nil, nil); err == nil {
			t.Errorf("a send claiming %q was accepted — that category is reserved for controller mail", c)
		}
	}
}

// Every reserved category is one this test knows about. Derived from the
// vocabulary rather than listed beside it: a category added to
// ServesTheSubject without being added above would otherwise be claimable by
// an ordinary send and no test would notice.
func TestEveryServeTheSubjectCategoryIsRefused(t *testing.T) {
	for _, c := range commsauthz.Categories() {
		if !c.ServesTheSubject() {
			continue
		}
		claimed := string(c)
		if _, err := sendContextFrom(&claimed, nil, nil, nil); err == nil {
			t.Errorf("%q serves the subject and a send may still claim it", c)
		}
	}
}

// The categories a rep's own send is ABOUT stay claimable. A decoder that
// refused these would leave a caller unable to say anything true about their
// message.
func TestAnOrdinaryCategoryIsAccepted(t *testing.T) {
	for _, c := range []commsauthz.Category{
		commsauthz.CategoryReplyToInbound,
		commsauthz.CategoryActiveDealFollowup,
		commsauthz.CategoryInvoiceOrPayment,
		commsauthz.CategoryMarketing,
	} {
		claimed := string(c)
		got, err := sendContextFrom(&claimed, nil, nil, nil)
		if err != nil {
			t.Errorf("claiming %q was refused: %v", c, err)
			continue
		}
		if got.category != c {
			t.Errorf("category = %q, want %q", got.category, c)
		}
	}
}

// A misspelled category is refused rather than dropped. Silently ignoring it
// would tell the caller their claim was accepted when nothing ever read it.
func TestAnUnknownCategoryIsRefusedRatherThanIgnored(t *testing.T) {
	claimed := "transactional"
	_, err := sendContextFrom(&claimed, nil, nil, nil)
	if err == nil {
		t.Fatal("an unknown category was accepted — a claim nothing reads is worse than a refusal")
	}
	var fault interface {
		FieldFault() (string, string, string)
	}
	if !asFieldFault(err, &fault) {
		t.Fatalf("refusal %v names no field; a caller cannot tell which input to change", err)
	}
	field, _, _ := fault.FieldFault()
	if field != contextField {
		t.Errorf("refusal names field %q, want %q", field, contextField)
	}
}

// The claimed value never comes back in the message. It is the caller's own
// text, and echoing it is how a refusal becomes a reflection surface.
func TestTheRefusalDoesNotEchoTheClaim(t *testing.T) {
	claimed := "<script>alert(1)</script>"
	_, err := sendContextFrom(&claimed, nil, nil, nil)
	if err == nil {
		t.Fatal("want a refusal")
	}
	if got := err.Error(); strings.Contains(got, "script") {
		t.Errorf("the refusal echoes the caller's value: %q", got)
	}
}

// Evidence arrives as ids and nothing else. The decoder's job is to flatten
// the optional block; whether a named record supports anything is the engine's
// question, asked against the record itself.
func TestEvidenceIsFlattenedById(t *testing.T) {
	deal := openapi_types.UUID(ids.NewV7())
	got, err := sendContextFrom(nil, nil, nil, &crmcontracts.CommunicationEvidence{DealId: &deal})
	if err != nil {
		t.Fatalf("evidence was refused: %v", err)
	}
	if got.evidence.DealID != ids.UUID(deal) {
		t.Errorf("deal id = %v, want %v", got.evidence.DealID, ids.UUID(deal))
	}
	if got.evidence.InvoiceID != (ids.UUID{}) {
		t.Error("an unnamed record came back non-zero; the engine reads zero as 'the caller named none'")
	}
}

// applyTo puts the claim on the input the send path actually carries. Without
// it the decode would be correct and reach nothing.
func TestTheClaimReachesTheSendInput(t *testing.T) {
	c := sendContext{category: commsauthz.CategoryMarketing, marketing: "newsletter", reason: "they asked at the fair"}
	got := c.applyTo(SendEmailInput{Subject: "Hello"})
	if got.Context != commsauthz.CategoryMarketing {
		t.Errorf("Context = %q, want marketing", got.Context)
	}
	if got.MarketingPurpose != "newsletter" || got.OperatorReason != "they asked at the fair" {
		t.Errorf("marketing purpose or operator reason did not reach the input: %+v", got)
	}
	if got.Subject != "Hello" {
		t.Error("applyTo overwrote a field it does not own")
	}
}

// asFieldFault is errors.As without importing errors for one call.
func asFieldFault(err error, target *interface {
	FieldFault() (string, string, string)
},
) bool {
	f, ok := err.(interface {
		FieldFault() (string, string, string)
	})
	if ok {
		*target = f
	}
	return ok
}

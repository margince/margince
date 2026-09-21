// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// See contacts/mapping_reservedsource_test.go: the activity store keys the
// same idempotent replay on (source_system, source_id), so the same
// boundary is enforced here and asserted the same way.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestActivityLogInputRefusesTheImporterNamespace(t *testing.T) {
	reserved := "mirror:legacy_crm"
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "email", SourceSystem: &reserved, SourceId: strPtr("emails:900"),
	})
	var refused *provenance.ReservedError
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want provenance.ReservedError — a client must not write the importer's namespace", err)
	}
}

// A connector's NAME is not the mail identity and stays writable: the guard is
// one reserved value, not a ban on naming a system.
func TestActivityLogInputAcceptsAnOrdinarySourceSystem(t *testing.T) {
	ordinary := "gmail"
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "email", SourceSystem: &ordinary, SourceId: strPtr("msg-1"),
	})
	if err != nil {
		t.Fatalf("an ordinary source system must stay writable: %v", err)
	}
	if in.SourceSystem == nil || *in.SourceSystem != ordinary {
		t.Errorf("SourceSystem = %v, want it carried through", in.SourceSystem)
	}
}

func strPtr(s string) *string { return &s }

// The mail identity is the store's replay key for every captured and sent
// message, so a caller who could write it would plant a row under a Message-ID
// and have the real capture of that message hand the planted row back as
// already existing.
func TestActivityLogInputRefusesTheMailIdentity(t *testing.T) {
	// Every kind, not just email: the unique index spans kinds, so a planted
	// note under a Message-ID suppresses the mail just as well.
	for _, kind := range []crmcontracts.CreateActivityRequestKind{"email", "note", "call"} {
		reserved := connector.EmailSourceSystem
		_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
			Kind: kind, SourceSystem: &reserved, SourceId: strPtr("planted@acme.test"),
		})
		var refused *ReservedMailIdentityError
		if !errors.As(err, &refused) {
			t.Fatalf("[%s] err = %v, want ReservedMailIdentityError — a client must not claim a mail identity", kind, err)
		}
		if field, code, _ := refused.FieldFault(); field != "source_system" || code != "reserved_source_system" {
			t.Errorf("[%s] refusal names (%q, %q), want (source_system, reserved_source_system)", kind, field, code)
		}
	}
}

// A STATED Message-ID is accepted, and that acceptance is correct.
//
// `rfc_message_id` carries no namespace guard. It is checked only for kind
// (mailimportmapping.go: an email may state one, other kinds may not) and then
// becomes the `mail` identity key verbatim. So a caller CAN claim the identity
// of a Message-ID they never received.
//
// What that costs, and why it is not the hole ReservedMailIdentityError closes:
//
//   - The planted row keeps the IMPORTER'S OWN natural key. source_system
//     'email' is separately refused (TestActivityLogInputRefusesTheMailIdentity),
//     so the row is filed under whatever the caller states — never under
//     ('email', <Message-ID>).
//   - A later capture of the real message therefore does NOT collide with it.
//     Both sides resolve this key — replayedActivity here, capture's own
//     ON CONFLICT (source_system, source_id) there — and the keys differ, so
//     the capture lands its own row. The message is not suppressed.
//   - The capture's own identity claim loses, which ClaimIdentity reports as a
//     non-error by contract — a lost claim is indistinguishable from having no
//     identity to claim, which is what stops it being an existence oracle.
//
// So the cost is a DENIED DEDUPLICATION, not denied mail: the planter can force
// one message to exist as two rows, and can attach their own row to a Message-ID
// they never held. That is a visible annoyance a human can delete.
//
// If a future change makes a stated Message-ID decide more than an identity
// claim — routing a create through the replay path, or letting it widen an
// audience — this is the test that must be read and revised rather than left
// silently passing.
func TestAStatedMessageIDIsAcceptedAndClaimsOnlyAnIdentity(t *testing.T) {
	planted := "never-received@counterparty.example"
	ordinary := "hubspot"
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "email", SourceSystem: &ordinary, SourceId: strPtr("engagement-4471"),
		RfcMessageId: &planted,
	})
	if err != nil {
		t.Fatalf("a stated Message-ID must stay writable — an importer states the "+
			"Message-ID of every message it carries: %v", err)
	}
	// It becomes the mail identity, which is the whole cross-door mechanism.
	kind, key := identityOf(in)
	if kind != IdentityKindMail || key != planted {
		t.Fatalf("identityOf = (%q, %q), want (%q, %q)", kind, key, IdentityKindMail, planted)
	}
	// And the row's OWN key stays the importer's. This is the line between a
	// nuisance and a suppression: were the natural key rewritten to
	// ('email', <Message-ID>), the planted row would collide with the real
	// capture and take the message away from the mailbox that holds it.
	if in.SourceSystem == nil || *in.SourceSystem != ordinary {
		t.Fatalf("SourceSystem = %q, want the importer's own %q", deref(in.SourceSystem), ordinary)
	}
	if in.SourceID == nil || *in.SourceID != "engagement-4471" {
		t.Errorf("SourceID = %q, want the importer's own record id", deref(in.SourceID))
	}
}

// The `source` guard matters as much as source_system's: activity is one
// of the classes the crash repair scans by provenance, so a client that
// could write the namespace there could have a planted row adopted.
func TestActivityLogInputRefusesAReservedSource(t *testing.T) {
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "note", Source: "mirror:legacy_crm:activity:a-1",
	})
	var refused *provenance.ReservedError
	if !errors.As(err, &refused) {
		t.Fatalf("err = %v, want provenance.ReservedError", err)
	}
	if refused.Field != "source" {
		t.Errorf("refusal names field %q, want source", refused.Field)
	}
}

func TestActivityLogInputAcceptsAnOrdinarySource(t *testing.T) {
	// The guard is a prefix rule, not a ban on the field: an ordinary
	// provenance string has to survive, or every capture connector that
	// stamps its own source would start failing at the wire.
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{Kind: "note", Source: "webform"})
	if err != nil {
		t.Fatalf("an ordinary source must stay writable: %v", err)
	}
	if in.Source != "webform" {
		t.Errorf("Source = %q, want it carried through", in.Source)
	}
}

func TestActivityLogInputRefusesInternalRequestProvenance(t *testing.T) {
	reserved := provenance.EmailRequestSource
	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{Kind: "task", SourceSystem: &reserved, SourceId: strPtr("request-1")})
	var refused *provenance.ReservedError
	if !errors.As(err, &refused) {
		t.Fatalf("internal request source was writable: %v", err)
	}
}

// A client that could spell a quiet-account reminder's identity could SUPPRESS
// that reminder, which is the reason these two names are reserved.
//
// The exploit, concretely: the scan skips an entity that already links an open
// task carrying the reminder's source_system, and replayedActivity resolves
// (source_system, source_id) without testing the row's source, captured_by or
// kind. So a row planted under a guessed key reads back as "already asked" and
// the account's reminder is never written — a silence that looks exactly like
// the system working.
//
// Every kind, not just task: the unique index spans kinds, so a planted note
// suppresses the reminder as well as a planted task would.
func TestActivityLogInputRefusesAQuietAccountReminderIdentity(t *testing.T) {
	for _, reserved := range []string{provenance.NoActivityReminderSource, provenance.CheckInCadenceSource} {
		for _, kind := range []crmcontracts.CreateActivityRequestKind{"task", "note"} {
			planted := reserved
			_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
				Kind: kind, SourceSystem: &planted,
				SourceId: strPtr("no_activity_reminder:company:11111111-1111-1111-1111-111111111111:anchor:2026-09-05T00:00:00Z"),
			})
			var refused *provenance.ReservedError
			if !errors.As(err, &refused) {
				t.Fatalf("[%s/%s] err = %v, want provenance.ReservedError — a client able to write this identity can suppress the reminder", reserved, kind, err)
			}
			if field, code, _ := refused.FieldFault(); field != "source_system" || code != "reserved_source_system" {
				t.Errorf("[%s/%s] refusal names (%q, %q), want (source_system, reserved_source_system)", reserved, kind, field, code)
			}
		}
	}
}

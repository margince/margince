// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two doors a unit writes an activity through, and the declared-transport
// rule that bounds both. The published record's mirror of the core's capture
// shapes is walked in extingress/drift_test.go.

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/pkg/extension"
)

// A unit has two doors it can write an activity through. The rule on which
// transport it may name is about the unit, not about a door, so this walks both
// doors against the same gate. The failure is asymmetric. Capture ingress dies on
// the CHECK as an unattributable 500. The core-write door carries channel_provider
// on the published request. A unit could name a core connector's transport and
// mint a row that is a valid send anchor for a conversation it does not own.
//
// The block below pins what the gate does. The source assertion after it pins
// that both doors call it, which is the half that regresses.
func TestAUnitMayNameOnlyItsOwnTransportThroughEitherDoor(t *testing.T) {
	composed := declaresTransport("mine", extension.Channel{Provider: "mine_chat", CredentialModel: extension.CredentialPerMember})

	if err := refuseUndeclaredTransport(composed, "mine", activities.KindMessage, "mine_chat"); err != nil {
		t.Errorf("a unit filing a message on the transport it declared was refused (%v); the declaration is what the permission is bounded by, so this is the case that must pass", err)
	}

	// A transport it did not declare, such as a core connector's, would become a
	// send anchor for a conversation the unit does not own.
	for _, provider := range []string{"telegram", "someone_elses"} {
		if err := refuseUndeclaredTransport(composed, "mine", activities.KindMessage, provider); err == nil {
			t.Errorf("a unit filing a message on %q was permitted; it declares only mine_chat", provider)
		} else if !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("the refusal is %v, want extension.ErrInvalid so the unit reads it as a bad record rather than a core fault", err)
		}
	}

	// A message that names nothing is refused too: it would be a conversation
	// with no transport, which nothing can reply on.
	if err := refuseUndeclaredTransport(composed, "mine", activities.KindMessage, ""); err == nil {
		t.Error("a message naming no transport was permitted; it lands as a conversation no reply can leave on")
	}

	// The other direction is the quieter defect. A record that is not a message
	// may name no transport at all, or it would be repliable, like a note that
	// answers back.
	for _, kind := range []string{"note", "email", "call", "meeting", "task"} {
		if err := refuseUndeclaredTransport(composed, "mine", kind, ""); err != nil {
			t.Errorf("a unit filing %q was refused (%v); only a message is bounded here", kind, err)
		}
		if err := refuseUndeclaredTransport(composed, "mine", kind, "mine_chat"); err == nil {
			t.Errorf("a %q naming a transport was permitted; the send path reads the provider column and would answer on it", kind)
		}
	}

	// The core-write door needs a live pool to walk, so this reads the source.
	for _, door := range []string{"extingress.go", "extcore.go"} {
		src, err := os.ReadFile(door)
		if err != nil {
			t.Fatalf("reading %s: %v", door, err)
		}
		if !strings.Contains(string(src), "refuseUndeclaredTransport(") {
			t.Errorf("%s writes an activity for a unit and does not call refuseUndeclaredTransport; that door can mint a message on a transport the unit does not supply", door)
		}
	}
}

// The counterparty binding is bounded by the same declaration. It is a separate
// gate because it writes a different row. The activity's provider decides where
// a reply is sent. The binding writes contact_channel_identity, where the core
// resolves who receives it. A unit that bound an account under a core
// connector's provider could attach its own account to somebody else's contact.
// It would then inherit the replies meant for them.
func TestAUnitMayBindAnAccountOnlyOnItsOwnTransport(t *testing.T) {
	composed := declaresTransport("mine", extension.Channel{Provider: "mine_chat", CredentialModel: extension.CredentialPerMember})

	if err := refuseUnitIdentity(composed, "mine", "mine_chat"); err != nil {
		t.Errorf("binding an account on the unit's own transport was refused (%v)", err)
	}
	// A record identified by address binds nothing, and must still land.
	if err := refuseUnitIdentity(composed, "mine", ""); err != nil {
		t.Errorf("a record with no channel identity was refused (%v); it identifies its counterparty by address", err)
	}
	if err := refuseUnitIdentity(composed, "mine", "telegram"); err == nil {
		t.Error("a unit bound an account under telegram; the next reply on that contact's conversation would go to the unit's account")
	} else if !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("the refusal is %v, want extension.ErrInvalid", err)
	}

	// Its reach is the half that regresses. Only the ingress door maps a
	// Counterparty today, because the published core-write request carries no
	// channel identity. This pins that door and fails if it stops calling the gate.
	src, err := os.ReadFile("extingress.go")
	if err != nil {
		t.Fatalf("reading extingress.go: %v", err)
	}
	if !strings.Contains(string(src), "refuseUnitIdentity(") {
		t.Error("extingress.go maps a counterparty for a unit and does not call refuseUnitIdentity; that door can bind an account under a transport the unit does not supply")
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Both readers share the kernel parser; this checks key precedence and typed operands
// across the access gate and the final wire projection.

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/dealstatus"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The shapes a card's arguments map really takes, plus the malformed ones a
// stored payload can carry after a build changes under it.
var recordArgumentCases = []struct {
	name  string
	args  map[string]any
	named bool
}{
	// The two verbs that act on a record: an unanswered mail, a booked meeting.
	{"a well-formed record", map[string]any{"activity_id": "01a05500-0000-7000-8000-0000000000a1"}, true},
	{"a request source", map[string]any{"request_activity_id": "01a05500-0000-7000-8000-0000000000a1"}, true},
	{"a malformed request source", map[string]any{"request_activity_id": 42}, false},
	{"a typed record", map[string]any{"activity_id": ids.MustParse("01a05500-0000-7000-8000-0000000000a1")}, true},
	{"a contract record", map[string]any{"activity_id": crmcontracts.Id(ids.MustParse("01a05500-0000-7000-8000-0000000000a1"))}, true},
	{"a malformed request wins over a different activity", map[string]any{"request_activity_id": 42, "activity_id": "01a05500-0000-7000-8000-0000000000a1"}, false},
	// A generic task acts on its subject and links, without source evidence.
	{"a verb with other arguments", map[string]any{"subject": "Agree the next step", "source": "manual"}, false},
	// firstOutreach builds its move with no arguments at all; the card
	// normalizes that to an empty object rather than null.
	{"an opening outreach", map[string]any{}, false},
	// A payload written by a build that spelled the key differently.
	{"a non-string value", map[string]any{"activity_id": 42}, false},
	{"a null value", map[string]any{"activity_id": nil}, false},
	{"an unparseable id", map[string]any{"activity_id": "not-a-uuid"}, false},
	{"an empty id", map[string]any{"activity_id": ""}, false},
}

func TestBothSidesReadTheRecordArgumentAlike(t *testing.T) {
	t.Parallel()
	for _, c := range recordArgumentCases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			args := c.args
			move := crmcontracts.DealStatusCardMove{Action: "draft_email", Arguments: &args}

			gateID, gateNamed := dealstatus.NamedActivity(move)
			wireID, wireNamed := attention.NamedActivityArgument(*move.Arguments)

			if gateNamed != c.named || wireNamed != c.named {
				t.Fatalf("the gate says named=%v and the wire says named=%v, wanted %v — "+
					"the wire naming a record the gate did not judge is an activity id served past the audience check",
					gateNamed, wireNamed, c.named)
			}
			if gateID != wireID {
				t.Fatalf("the gate judged %s and the wire named %s — one record checked, a different one served",
					gateID, wireID)
			}
		})
	}
}

// The corpus above is a list, and a list can fall short of what production
// writes. This is the case that must never silently pass: a key the wire reads
// and the gate does not.
//
// It is asserted from the ONE direction that discloses. If a future edit
// loosened the wire's parse — accepting a uuid with surrounding space, say —
// this fails, because the gate would still refuse it.
func TestTheWireNeverNamesARecordTheGateRefused(t *testing.T) {
	t.Parallel()
	loosenings := []any{
		" 01a05500-0000-7000-8000-0000000000a1",
		"01a05500-0000-7000-8000-0000000000a1 ",
		"{01a05500-0000-7000-8000-0000000000a1}",
		"urn:uuid:01a05500-0000-7000-8000-0000000000a1",
		"01A05500000070008000-0000000000A1",
	}
	for _, value := range loosenings {
		args := map[string]any{"activity_id": value}
		move := crmcontracts.DealStatusCardMove{Action: "draft_email", Arguments: &args}

		_, gateNamed := dealstatus.NamedActivity(move)
		wireID, wireNamed := attention.NamedActivityArgument(*move.Arguments)

		if wireNamed && !gateNamed {
			t.Fatalf("the wire named %s from %q while the gate refused it — "+
				"that id reaches a reader whose audience was never checked", wireID, value)
		}
	}
}

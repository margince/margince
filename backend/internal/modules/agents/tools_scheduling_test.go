// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What the scheduling verbs claim, and what they refuse to. Split from
// tools_comms_test.go alongside the source they cover: booking anchors on no
// row, so everything about it here is the records it ATTACHES to, and
// availability answers about a calendar this product may not even hold — which
// is a different subject from the mail and channel sends next door.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A booking with no links is refused, at BOTH doors.
//
// crm.yaml's bookMeeting body requires `links`, and this surface advertises it
// as required with minItems 1 — but an InputSchema on the MCP surface is
// documentation, never validation, so the rule holds only where the code puts
// it. It used to stage instead: an approval with no target, which is a human
// asked to release a meeting attached to nothing, with no record to show them
// and no version to pin it against.
//
// The execute door is covered too, because it is reached with an approval
// already redeemed — a call that never passed staging would otherwise book.
func TestABookingThatNamesNoRecordIsRefusedAtBothDoors(t *testing.T) {
	const noLinks = `{"start":"2026-08-03T09:00:00Z","end":"2026-08-03T09:30:00Z"}`
	for _, tc := range []struct {
		name string
		ctx  context.Context
	}{
		{"staging", sendCtx()},
		{"after an approval was redeemed", withApprovalRedeemed(sendCtx(), 0, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			approvals := &recordingApprovals{}
			comms := &recordingComms{}
			registry := NewRegistry(approvals, auth.NewGate(fullSeatAuthority{}))
			RegisterCommsTools(registry, comms, &multiLinkProvider{})

			_, err := registry.Invoke(tc.ctx, "book_meeting", json.RawMessage(noLinks))

			var badArgs *BadArgsError
			if !errors.As(err, &badArgs) {
				t.Fatalf("Invoke err = %v, want a BadArgsError naming `links`", err)
			}
			if !strings.Contains(err.Error(), "`links`") {
				t.Errorf("err = %v, want it to name the field", err)
			}
			if len(approvals.staged) != 0 {
				t.Errorf("staged %d approvals, want none — nothing was approvable", len(approvals.staged))
			}
			if comms.booked != nil {
				t.Error("a booking attached to nothing reached the comms seam")
			}
		})
	}
}

// Every link is checked, not just the one the inbox displays. A booking that
// mixes a local deal with a mirrored company is exactly what a
// first-link-only guard would wave through into an approval nobody could
// release.
func TestABookingRefusesAMirroredLinkBehindALocalOne(t *testing.T) {
	local, mirrored := ids.NewV7(), ids.NewV7()
	p := &multiLinkProvider{heldElsewhere: map[ids.UUID]bool{mirrored: true}}

	_, err := bookMeetingTool{comms: &recordingComms{}, p: p}.StageInfo(context.Background(),
		json.RawMessage(fmt.Sprintf(
			`{"start":"2026-08-03T09:00:00Z","end":"2026-08-03T09:30:00Z","links":[{"entity_type":"deal","entity_id":%q},{"entity_type":"company","entity_id":%q}]}`,
			local, mirrored)))

	if !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
		t.Fatalf("StageInfo err = %v, want ErrUnsupportedBySoR — the second link was never validated", err)
	}
	if len(p.read) != 2 {
		t.Errorf("read %d links, want both", len(p.read))
	}
}

// Each link costs its own row-scoped read in its own transaction, and the array
// is chosen freely by the caller — at the body limit one tools/call could carry
// thousands, spending tens of thousands of queries against a 16-connection pool
// before any human has approved anything, because staging runs on the refusal
// path. Deduplicating matters as much as the cap: the same id repeated is the
// cheapest way to turn one call into N reads.
func TestABookingIsBoundedAndDeduplicatedBeforeItReadsAnything(t *testing.T) {
	one := ids.NewV7()
	slot := `"start":"2026-08-10T09:00:00Z","end":"2026-08-10T09:30:00Z"`

	t.Run("refuses more links than a meeting could mean", func(t *testing.T) {
		links := make([]string, maxRecordLinks+1)
		for i := range links {
			links[i] = fmt.Sprintf(`{"entity_type":"deal","entity_id":%q}`, ids.NewV7())
		}
		p := &multiLinkProvider{}
		_, err := bookMeetingTool{comms: &recordingComms{}, p: p}.StageInfo(context.Background(),
			json.RawMessage(fmt.Sprintf(`{%s,"links":[%s]}`, slot, strings.Join(links, ","))))

		var bad *BadArgsError
		if !errors.As(err, &bad) {
			t.Fatalf("StageInfo err = %v, want a BadArgsError refusing the oversized link array", err)
		}
		if len(p.read) != 0 {
			t.Errorf("read %d records before refusing — the cap must precede the reads it exists to prevent", len(p.read))
		}
	})

	t.Run("reads a repeated link once", func(t *testing.T) {
		repeated := make([]string, 10)
		for i := range repeated {
			repeated[i] = fmt.Sprintf(`{"entity_type":"deal","entity_id":%q}`, one)
		}
		p := &multiLinkProvider{}
		if _, err := (bookMeetingTool{comms: &recordingComms{}, p: p}).StageInfo(context.Background(),
			json.RawMessage(fmt.Sprintf(`{%s,"links":[%s]}`, slot, strings.Join(repeated, ",")))); err != nil {
			t.Fatalf("StageInfo err = %v", err)
		}
		if len(p.read) != 1 {
			t.Errorf("read %d times for one distinct link, want 1", len(p.read))
		}
	})
}

// A booking that ends before it starts is refused at BOTH doors. The store
// refuses it as well, but reaching that refusal costs the human's approval on
// the way past: redemption is consumed before the handler runs.
func TestABookingWithNoDurationIsRefusedAtBothDoors(t *testing.T) {
	const backwards = `{"start":"2026-08-03T09:30:00Z","end":"2026-08-03T09:00:00Z",` +
		`"links":[{"entity_type":"deal","entity_id":"019ff000-0000-7000-8000-000000000001"}]}`
	comms := &recordingComms{}
	tool := bookMeetingTool{comms: comms, p: &multiLinkProvider{}}
	doors := map[string]func() error{
		"staging": func() error {
			_, err := tool.StageInfo(context.Background(), json.RawMessage(backwards))
			return err
		},
		"after an approval was redeemed": func() error {
			_, err := tool.Handle(withApprovalRedeemed(sendCtx(), 0, false), json.RawMessage(backwards))
			return err
		},
	}
	for door, call := range doors {
		t.Run(door, func(t *testing.T) {
			var bad *BadArgsError
			if err := call(); !errors.As(err, &bad) {
				t.Fatalf("err = %v, want a BadArgsError naming the window", err)
			}
			if comms.booked != nil {
				t.Error("a booking with no duration reached the comms seam")
			}
		})
	}
}

// availabilityComms answers the free/busy question with one prepared window and
// leaves the rest of the seam to the double the send tests already use — the
// scheduling verbs are the only thing under test here.
type availabilityComms struct {
	*recordingComms
	answer AvailabilityResult
}

func (c availabilityComms) Availability(context.Context, *ids.UUID, time.Time, time.Time, int) (AvailabilityResult, error) {
	return c.answer, nil
}

// aWorkdayOfSlots is the shape both cases answer with: a full day free. It is
// the same list in each, because the defect is that the two states are
// indistinguishable from the slots alone.
func aWorkdayOfSlots() []FreeSlot {
	start := time.Date(2026, time.September, 11, 7, 0, 0, 0, time.UTC)
	slots := make([]FreeSlot, 0, 16)
	for i := 0; i < 16; i++ {
		at := start.Add(time.Duration(i) * 30 * time.Minute)
		slots = append(slots, FreeSlot{Start: at, End: at.Add(30 * time.Minute)})
	}
	return slots
}

func checkAvailabilityWindow(t *testing.T, answer AvailabilityResult) Envelope {
	t.Helper()
	registry := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	registry.Register(checkAvailability{comms: availabilityComms{
		recordingComms: &recordingComms{}, answer: answer,
	}})
	sealed, err := registry.Invoke(scopedAgentCtx(principal.ScopeRead), "check_availability",
		json.RawMessage(`{"from":"2026-09-11T00:00:00Z","to":"2026-09-12T00:00:00Z","duration_minutes":30}`))
	if err != nil {
		t.Fatalf("check_availability: %v", err)
	}
	return sealedEnvelope(t, sealed)
}

// An unconnected calendar must not answer as an empty one. A host whose diary
// this product was never shown returns exactly the slots a host with a clear
// day returns, so the only thing separating the two is what the envelope says
// about them: the warning that names the conclusion not to draw, and the
// authority claim an answer computed without its source may not make.
func TestAnUnconnectedCalendarDoesNotAnswerAsAnEmptyOne(t *testing.T) {
	env := checkAvailabilityWindow(t, AvailabilityResult{
		Slots: aWorkdayOfSlots(), CalendarBacking: CalendarUnbacked,
	})

	warning, warned := warningNamed(env, warningNoCalendarConnected)
	if !warned {
		t.Fatalf("a free day read off an unconnected calendar carries no warning: %v", env.Warnings)
	}
	// The clause its own comment calls load-bearing, and the one a reader
	// reached past unprompted in every measured run: a free slot is not
	// evidence that a meeting does not exist.
	if !strings.Contains(warning.Message, "no evidence that a meeting") {
		t.Errorf("the warning no longer names the conclusion not to draw: %q", warning.Message)
	}
	if env.Freshness.Authoritative {
		t.Error("an answer computed without the calendar it reports on claims to be authoritative")
	}
}

// And a calendar this product does read says nothing of the sort: a genuinely
// free day is a real answer, and a warning on it would train a reader to
// discount the ones that matter.
func TestAGenuinelyFreeDayCarriesNoCalendarWarning(t *testing.T) {
	env := checkAvailabilityWindow(t, AvailabilityResult{
		Slots: aWorkdayOfSlots(), CalendarBacking: CalendarBacked,
	})

	if _, warned := warningNamed(env, warningNoCalendarConnected); warned {
		t.Errorf("a connected calendar's free day is reported as unread: %v", env.Warnings)
	}
	if !env.Freshness.Authoritative {
		t.Error("a window read off the host's own calendar disclaims its own authority")
	}
}

// A result whose backing was never set is treated as unestablished, not as a
// calendar read.
//
// CalendarBacking's zero value is the empty string, so a caller that forgets
// the field gets it. Silence there would publish freshness.authoritative on a
// window nothing established — this file's own defect, reached by omission
// rather than by intent.
func TestAnUnsetCalendarBackingStillCarriesTheCaveat(t *testing.T) {
	env := checkAvailabilityWindow(t, AvailabilityResult{Slots: aWorkdayOfSlots()})

	if _, warned := warningNamed(env, warningNoCalendarConnected); !warned {
		t.Fatalf("a window with no backing set carries no caveat: %v", env.Warnings)
	}
	if env.Freshness.Authoritative {
		t.Error("a window whose source was never established claims to be authoritative")
	}
}

// A host who is NOT the acting seat carries the same caveat and says nothing
// about that contact's account.
//
// Whether a colleague has connected a calendar is their account's business, and
// this tool takes any host_user_id — so an answer that reported it would let
// anyone holding read walk the roster and learn who has connected Google or
// Microsoft, and whose grant has since stopped working. The window is still
// only what this CRM holds, which is what the reader is owed and all they get.
func TestAForeignHostsWindowCarriesTheCaveatWithoutTheirConnectorState(t *testing.T) {
	env := checkAvailabilityWindow(t, AvailabilityResult{
		Slots: aWorkdayOfSlots(), CalendarBacking: CalendarBackingUnknown,
	})

	warning, warned := warningNamed(env, warningNoCalendarConnected)
	if !warned {
		t.Fatalf("a foreign host's free day carries no caveat, so it reads as their diary: %v", env.Warnings)
	}
	if strings.Contains(warning.Message, "No calendar is connected") {
		t.Errorf("the caveat states another seat's connector state: %q", warning.Message)
	}
	if env.Freshness.Authoritative {
		t.Error("a window computed without the host's calendar claims to be authoritative")
	}
}

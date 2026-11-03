// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Capture files a reply under every lead whose address is on it. Only the lead
// who sent it has replied, so only that lead may be promoted on it.
func TestOnlyTheSenderOfACapturedReplyIsPromotedOnIt(t *testing.T) {
	l := setupLeadMail(t)
	sender := l.lead(t, "ana@prospect.example")
	copied := l.lead(t, "ben@prospect.example")
	reply := l.mail(t, owedMail{from: "ana@prospect.example", to: l.seat, cc: "ben@prospect.example", subject: "Re: Pricing"}, true)
	if !l.filedUnderLead(t, reply, sender) || !l.filedUnderLead(t, reply, copied) {
		t.Fatal("capture did not file the reply under both leads on it, so this run proves nothing")
	}

	copiedRead, err := l.e.Contacts.GetLead(l.e.Admin(), copied, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if copiedRead.QualificationEvidence != nil {
		t.Errorf("the lead read offers %+v as the copied lead's evidence, though that lead never replied", *copiedRead.QualificationEvidence)
	}
	cited := ids.From[ids.ActivityKind](reply)
	_, _, err = l.e.Contacts.PromoteLead(l.e.Admin(), copied, contacts.PromoteLeadInput{Trigger: "inbound_reply", EvidenceActivityID: &cited})
	if _, refused := errors.AsType[*contacts.PromoteEvidenceError](err); !refused {
		t.Errorf("promoting the copied lead on the sender's reply → %v, want PromoteEvidenceError", err)
	}
	if _, _, err := l.e.Contacts.PromoteLead(l.e.Admin(), sender, contacts.PromoteLeadInput{Trigger: "inbound_reply", EvidenceActivityID: &cited}); err != nil {
		t.Errorf("promoting the sender on their own reply: %v", err)
	}
}

// A captured meeting names its attendees, and an attendee lead took part.
func TestACapturedMeetingsAttendeeLeadIsPromotedOnIt(t *testing.T) {
	l := setupLeadMail(t)
	attendee := l.lead(t, "cara@prospect.example")
	meeting := ids.From[ids.ActivityKind](l.meeting(t, "cara@prospect.example"))
	if _, _, err := l.e.Contacts.PromoteLead(l.e.Admin(), attendee, contacts.PromoteLeadInput{Trigger: "meeting_booked", EvidenceActivityID: &meeting}); err != nil {
		t.Errorf("promoting an attendee lead on the captured meeting: %v", err)
	}
}

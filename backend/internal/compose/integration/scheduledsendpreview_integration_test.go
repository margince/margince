// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A scheduled message reads back what a surface needs to ask the engine the
// same question the fire will ask: the records it names, and what its sender
// claimed. Through the real writer and the real read, because the frozen
// payload is one shape and the wire is another, and a field dropped between
// them fails nowhere else — the queue would simply fall silent on the message.

import (
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scheduledOnTheWire is the slice of the wire a preview is asked with.
type scheduledOnTheWire struct {
	ID             string `json:"id"`
	AnchorActivity string `json:"anchor_activity_id"`
	Links          []struct {
		EntityType string `json:"entity_type"`
		EntityID   string `json:"entity_id"`
	} `json:"links"`
	CommunicationContext string `json:"communication_context"`
	ConsentPurpose       string `json:"consent_purpose"`
	Evidence             *struct {
		ActivityID string `json:"activity_id"`
	} `json:"evidence"`
}

func (p *preflightEnv) scheduledOnTheWire(t *testing.T, id ids.UUID) scheduledOnTheWire {
	t.Helper()
	var got scheduledOnTheWire
	if status := p.Call(t, "GET", "/v1/scheduled-sends/"+id.String(), nil, nil, &got); status != http.StatusOK {
		t.Fatalf("reading the scheduled send → %d", status)
	}
	return got
}

func TestAScheduledAccountSendReadsBackWhatItWillAskTheEngine(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	var scheduled struct {
		ID string `json:"id"`
	}
	status := p.Call(t, "POST", "/v1/emails", AnyMap{
		"subject": "Your quote", "body": "As discussed.",
		"to":              []string{"buyer@preflight.test"},
		"consent_purpose": "transactional",
		"links": []AnyMap{
			{"entity_type": "person", "entity_id": p.personID},
		},
		"evidence":     AnyMap{"activity_id": p.activityID},
		"scheduled_at": time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339),
		"scheduled_tz": "Europe/Berlin",
	}, nil, &scheduled)
	if status != http.StatusCreated {
		t.Fatalf("scheduling an account send → %d, want 201", status)
	}
	id, err := ids.Parse(scheduled.ID)
	if err != nil {
		t.Fatalf("scheduling returned no id: %v", err)
	}

	got := p.scheduledOnTheWire(t, id)
	if len(got.Links) != 1 || got.Links[0].EntityType != "person" || got.Links[0].EntityID != p.personID {
		t.Errorf("the records the send named came back as %+v, want the one person it was filed under: "+
			"the account-send preview refuses a message naming no records, so a row without them "+
			"cannot be asked about", got.Links)
	}
	if got.ConsentPurpose != "transactional" {
		t.Errorf("consent purpose came back %q, want transactional: the fire consults it where the record "+
			"supports no category, so a preview without it asks a different question", got.ConsentPurpose)
	}
	if got.Evidence == nil || got.Evidence.ActivityID != p.activityID {
		t.Errorf("evidence came back %+v, want the activity named: evidence is what makes a claimed "+
			"category supported", got.Evidence)
	}
	if got.AnchorActivity != "" {
		t.Errorf("an account-started send names anchor %q, want none", got.AnchorActivity)
	}
}

// A reply's records come from its anchor and it freezes none of its own, so the
// wire names the anchor and no records. The NULL origin_links column the
// origin-shape CHECK requires of a reply row is the case this holds: read as
// "none", never as a row the list cannot show.
func TestAScheduledReplyNamesItsAnchorAndNoRecords(t *testing.T) {
	p := setupPreflight(t)
	p.connect(t, gmailReadonlyScope, gmailSendScope)

	id := p.scheduleFor(t, time.Now().Add(2*time.Hour))
	got := p.scheduledOnTheWire(t, id)
	if got.AnchorActivity != p.activityID {
		t.Errorf("a reply names anchor %q, want %q", got.AnchorActivity, p.activityID)
	}
	if len(got.Links) != 0 {
		t.Errorf("a reply came back naming records of its own: %+v", got.Links)
	}
	if got.ConsentPurpose != "transactional" {
		t.Errorf("consent purpose came back %q, want transactional", got.ConsentPurpose)
	}
}

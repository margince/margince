// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// firstDrafter is a drafter with a brain and an envelope resolver and nothing
// else: DraftFirstEmail reads no store, which is the property the seam has that
// the reply seam does not.
// firstMessageContext is the floor's own reading of a first message.
//
// It is a value rather than a literal at each assertion because a test that
// spelled its own context could assert the WRONG floor and pass: the
// deterministic renderer answers for any context handed to it, so a fallback
// test is only about the fallback if it asks for the one the code asks for.
var firstMessageContext = activities.DraftContext{Band: convstate.BandFresh, Threaded: false}

func firstDrafter(brain completer) replyDrafter {
	return replyDrafter{brain: brain, envelope: draftfloor.NewResolver()}
}

// TestAFirstMessageIsComposedRatherThanTemplated is the ticket's own case: an
// intent placed between two fixed lines is a template the caller must rewrite,
// and it was what this path returned for every deployment, model or not.
func TestAFirstMessageIsComposedRatherThanTemplated(t *testing.T) {
	brain := &replyBrainStub{response: model.Response{
		Text: `{"subject":"Following up on K5","body":"It was good to meet you at K5 last week."}`,
	}}
	intent := "introduce ourselves after meeting at K5"

	subject, body, err := firstDrafter(brain).DraftFirstEmail(context.Background(), intent)
	if err != nil {
		t.Fatalf("DraftFirstEmail: %v", err)
	}
	if subject != "Following up on K5" || body != "It was good to meet you at K5 last week." {
		t.Fatalf("the model's draft did not reach the caller: subject=%q body=%q", subject, body)
	}
	deterministicSubject, deterministicBody := activities.DeterministicEmailDraft(firstMessageContext, intent)
	if body == deterministicBody && subject == deterministicSubject {
		t.Error("the deterministic floor answered while a model was configured, which is the defect this closes")
	}
}

// TestAFirstMessageCarriesNoThreadEvidence holds what makes it a first message
// rather than a reply with its evidence missing.
//
// The fields are asserted on the REQUEST rather than on the reply: a subject or
// a body reaching the lane would be a claim about correspondence that does not
// exist, and "Re:" on a message nobody sent us is the exact failure the thread
// flag exists to prevent.
func TestAFirstMessageCarriesNoThreadEvidence(t *testing.T) {
	brain := &replyBrainStub{response: model.Response{Text: `{"subject":"Hello","body":"A short note."}`}}

	if _, _, err := firstDrafter(brain).DraftFirstEmail(context.Background(), "ask for a call"); err != nil {
		t.Fatalf("DraftFirstEmail: %v", err)
	}
	if len(brain.request.Messages) != 1 {
		t.Fatalf("the lane was asked %d messages, want one", len(brain.request.Messages))
	}
	sent := brain.request.Messages[0].Content
	for field, spelling := range map[string]string{
		"a subject being answered": `"subject"`,
		"a body being answered":    `"body"`,
		"an inbound mail thread":   "inbound_mail",
	} {
		if strings.Contains(sent, spelling) {
			t.Errorf("a first message carried %s (%s) into the lane: %s", field, spelling, sent)
		}
	}
	if !strings.Contains(sent, "ask for a call") {
		t.Errorf("the caller's intent — the whole of the subject material — did not reach the lane: %s", sent)
	}
}

// TestAFirstMessageFallsBackToTheFloorRatherThanFailing holds the degrade.
//
// A model that is down must not cost the caller their draft: the floor is a
// real message they can edit, and it is what this path returned before the lane
// was reachable at all.
func TestAFirstMessageFallsBackToTheFloorRatherThanFailing(t *testing.T) {
	intent := "introduce ourselves after meeting at K5"
	wantSubject, wantBody := activities.DeterministicEmailDraft(firstMessageContext, intent)

	for what, drafter := range map[string]replyDrafter{
		"no model in this deployment": firstDrafter(nil),
		"a model that refuses":        firstDrafter(&replyBrainStub{err: errors.New("over budget")}),
		"a model answering nonsense":  firstDrafter(&replyBrainStub{response: model.Response{Text: "not json"}}),
	} {
		t.Run(what, func(t *testing.T) {
			subject, body, err := drafter.DraftFirstEmail(context.Background(), intent)
			if err != nil {
				t.Fatalf("a caller was refused their draft: %v", err)
			}
			if subject != wantSubject || body != wantBody {
				t.Errorf("the floor did not answer: subject=%q body=%q", subject, body)
			}
		})
	}
}

// replyOnlyDrafter implements the reply seam and nothing else — the shape a
// deployment binds when it runs no model, and the shape every drafter had
// before this seam existed.
type replyOnlyDrafter struct{}

func (replyOnlyDrafter) DraftEmail(context.Context, ids.UUID, string) (string, string, error) {
	return "", "", errors.New("this drafter answers replies only")
}

// firstSeamDrafter records that the first-message seam was the one asked.
type firstSeamDrafter struct {
	replyOnlyDrafter
	asked string
}

func (d *firstSeamDrafter) DraftFirstEmail(_ context.Context, intent string) (string, string, error) {
	d.asked = intent
	return "Composed", "A message written for this intent.", nil
}

// TestTheAccountDraftAsksTheFirstMessageSeamWhenTheDrafterHasOne holds the
// WIRING, which is the half a test of DraftFirstEmail alone cannot see.
//
// The defect this closes was never that the lane could not compose — it was
// that nothing asked it to. A drafter that can open a conversation and an
// adapter that never calls it reads exactly like the deterministic path it
// replaced, and every other test here would stay green.
func TestTheAccountDraftAsksTheFirstMessageSeamWhenTheDrafterHasOne(t *testing.T) {
	links := []agents.RecordLink{{EntityType: "person", EntityID: ids.NewV7()}}
	intent := "introduce ourselves after meeting at K5"

	drafter := &firstSeamDrafter{}
	subject, body, err := commsAdapter{draft: drafter}.DraftAccountEmail(context.Background(), links, intent)
	if err != nil {
		t.Fatalf("DraftAccountEmail: %v", err)
	}
	if drafter.asked != intent {
		t.Errorf("the first-message seam was asked %q, want the caller's intent %q", drafter.asked, intent)
	}
	if subject != "Composed" || body != "A message written for this intent." {
		t.Errorf("the composed draft did not reach the caller: subject=%q body=%q", subject, body)
	}

	// And the floor for a drafter that cannot: a deployment running no model
	// still gets a real message rather than a refusal.
	floorSubject, floorBody := activities.DeterministicEmailDraft(firstMessageContext, intent)
	subject, body, err = commsAdapter{draft: replyOnlyDrafter{}}.DraftAccountEmail(context.Background(), links, intent)
	if err != nil {
		t.Fatalf("DraftAccountEmail on a reply-only drafter: %v", err)
	}
	if subject != floorSubject || body != floorBody {
		t.Errorf("a reply-only drafter did not fall to the floor: subject=%q body=%q", subject, body)
	}
}

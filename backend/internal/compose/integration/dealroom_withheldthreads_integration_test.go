// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A buyer's conversation about a file that stops being shared is withheld from
// their list, and both sides are told. The buyer learns that one of THEIR
// conversations is withheld, with no count and no file. The seller's list marks
// the thread as no longer visible to the buyer.

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/blobstore"
)

type withheldRoom struct {
	e            *apptest.AppEnv
	room         buyerRoom
	laura, mira  string
	attachmentID string
	docID        string
	activityID   string
}

// roomWithOneSharedFile opens a room with two buyer seats (Laura and Mira),
// both signed in, and one file shared in it.
func roomWithOneSharedFile(t *testing.T) withheldRoom {
	t.Helper()
	return roomSharing(t, false)
}

// roomSharing shares an uploaded deal file, or a file that arrived on an
// email linked to the deal when mail is true.
func roomSharing(t *testing.T, mail bool) withheldRoom {
	t.Helper()
	blob := blobstore.NewMemory()
	e := apptest.SetupAppWithOptions(t, compose.WithBlobstore(blob))
	e.BootstrapWorkspace(t)
	room := openRoomWithABuyer(t, e)
	var roomRow AnyMap
	if status := e.Call(t, "GET", "/v1/deal-rooms/"+room.roomID, nil, nil, &roomRow); status != http.StatusOK {
		t.Fatalf("room = %d", status)
	}
	dealID, _ := roomRow["deal_id"].(string)
	var attachmentID, activityID string
	if mail {
		activityID, attachmentID = captureFileOnEmail(t, e, blob, dealID)
	} else {
		attachmentID = uploadDealFile(t, e, dealID, "terms.pdf", []byte("%PDF-TERMS"))
	}
	var doc AnyMap
	if status := e.Call(t, "POST", "/v1/deal-rooms/"+room.roomID+"/documents", AnyMap{
		"attachment_id": attachmentID, "group_key": "legal", "source": "manual",
	}, nil, &doc); status != http.StatusCreated {
		t.Fatalf("add document = %d %v", status, doc)
	}
	docID, _ := doc["id"].(string)
	mira := inviteAnotherBuyer(t, e, room.roomID, "mira@buyer.example")
	return withheldRoom{
		e: e, room: room, attachmentID: attachmentID, docID: docID, activityID: activityID,
		laura: signIn(t, e, room.credential), mira: signIn(t, e, mira.credential),
	}
}

func signIn(t *testing.T, e *apptest.AppEnv, credential string) string {
	t.Helper()
	var session AnyMap
	if status := publicCall(t, e, "POST", "/v1/public/rooms/exchange", AnyMap{"credential": credential}, nil, &session); status != http.StatusOK {
		t.Fatalf("exchange = %d", status)
	}
	token, _ := session["session_token"].(string)
	return token
}

// buyerOpens opens a thread as one buyer; docID "" is a room-level thread.
func (w withheldRoom) buyerOpens(t *testing.T, token, docID, body string) string {
	t.Helper()
	in := AnyMap{"body": body}
	if docID != "" {
		in["document_id"] = docID
	}
	var thread AnyMap
	if status := publicCall(t, w.e, "POST", "/v1/public/rooms/threads", in, bearer(token), &thread); status != http.StatusCreated {
		t.Fatalf("buyer opens thread = %d %v", status, thread)
	}
	id, _ := thread["id"].(string)
	return id
}

func (w withheldRoom) sellerOpens(t *testing.T, docID, body string) string {
	t.Helper()
	var thread AnyMap
	if status := w.e.Call(t, "POST", "/v1/deal-rooms/"+w.room.roomID+"/threads", AnyMap{
		"document_id": docID, "body": body, "source": "manual",
	}, nil, &thread); status != http.StatusCreated {
		t.Fatalf("seller opens thread = %d %v", status, thread)
	}
	id, _ := thread["id"].(string)
	return id
}

// buyerList returns the thread ids the buyer reads and the withheld-own flag.
func (w withheldRoom) buyerList(t *testing.T, token string) (threadIDs []string, hiddenOwn bool) {
	t.Helper()
	return w.buyerListFor(t, token, "")
}

// buyerListFor is buyerList narrowed to one document id, as the query allows.
func (w withheldRoom) buyerListFor(t *testing.T, token, docID string) (threadIDs []string, hiddenOwn bool) {
	t.Helper()
	path := "/v1/public/rooms/threads"
	if docID != "" {
		path += "?document_id=" + docID
	}
	var got AnyMap
	if status := publicCall(t, w.e, "GET", path, nil, bearer(token), &got); status != http.StatusOK {
		t.Fatalf("buyer threads = %d %v", status, got)
	}
	list, _ := got["data"].([]any)
	for _, item := range list {
		row, _ := item.(map[string]any)
		id, _ := row["id"].(string)
		threadIDs = append(threadIDs, id)
		if row["visible_to_buyer"] != true {
			t.Errorf("a thread on the buyer's own list says visible_to_buyer=%v", row["visible_to_buyer"])
		}
	}
	hidden, present := got["hidden_own_conversations"].(bool)
	if !present {
		t.Fatalf("the buyer's list carries no boolean hidden_own_conversations: %v", got)
	}
	return threadIDs, hidden
}

// sellerVisibleToBuyer maps thread id to the seller list's visible_to_buyer.
func (w withheldRoom) sellerVisibleToBuyer(t *testing.T) map[string]bool {
	t.Helper()
	var got AnyMap
	if status := w.e.Call(t, "GET", "/v1/deal-rooms/"+w.room.roomID+"/threads", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("seller threads = %d %v", status, got)
	}
	out := map[string]bool{}
	list, _ := got["data"].([]any)
	for _, item := range list {
		row, _ := item.(map[string]any)
		id, _ := row["id"].(string)
		v, present := row["visible_to_buyer"].(bool)
		if !present {
			t.Fatalf("seller thread %s carries no boolean visible_to_buyer", id)
		}
		out[id] = v
	}
	return out
}

func (w withheldRoom) withdraw(t *testing.T, how string) {
	t.Helper()
	switch how {
	case "archived":
		if status := w.e.Call(t, "DELETE", "/v1/attachments/"+w.attachmentID, nil, nil, nil); status != http.StatusNoContent {
			t.Fatalf("archive attachment = %d", status)
		}
	case "unlinked":
		// No endpoint removes a message's link to a deal, so the row goes directly.
		if _, err := w.e.Pool.Exec(context.Background(),
			`DELETE FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal' AND deal_id = $2`,
			w.activityID, w.room.dealID); err != nil {
			t.Fatalf("unlink the email from the deal: %v", err)
		}
	case "hidden":
		if status := w.e.Call(t, "PUT", "/v1/deals/"+w.room.dealID+"/documents/"+w.attachmentID+"/hide", nil, nil, nil); status != http.StatusNoContent {
			t.Fatalf("hide = %d", status)
		}
	}
}

func TestAWithdrawnFileAnnouncesTheBuyersOwnConversationToBothSides(t *testing.T) {
	for _, how := range []string{"archived", "hidden"} {
		t.Run(how, func(t *testing.T) {
			w := roomWithOneSharedFile(t)
			own := w.buyerOpens(t, w.laura, w.docID, "Is clause 4 negotiable?")
			roomLevel := w.buyerOpens(t, w.laura, "", "When does rollout start?")

			ids, hidden := w.buyerList(t, w.laura)
			if len(ids) != 2 || hidden {
				t.Fatalf("before the withdrawal: threads=%v hidden_own=%v, want both threads and no line", ids, hidden)
			}

			w.withdraw(t, how)

			ids, hidden = w.buyerList(t, w.laura)
			if len(ids) != 1 || ids[0] != roomLevel {
				t.Errorf("buyer list = %v, want only the room-level thread %s", ids, roomLevel)
			}
			if !hidden {
				t.Errorf("the buyer's own conversation was withheld without a word")
			}
			seller := w.sellerVisibleToBuyer(t)
			if seller[own] || !seller[roomLevel] {
				t.Errorf("seller marks = %v, want %s withheld and %s visible", seller, own, roomLevel)
			}
			if status := publicCall(t, w.e, "POST", "/v1/public/rooms/threads/"+own+"/comments",
				AnyMap{"body": "still there?"}, bearer(w.laura), nil); status != http.StatusNotFound {
				t.Errorf("reply in a withheld thread = %d, want 404", status)
			}
		})
	}
}

func TestAConversationOnlyTheSellerWroteInRevealsNothingToTheBuyer(t *testing.T) {
	w := roomWithOneSharedFile(t)
	sellerOnly := w.sellerOpens(t, w.docID, "Internal view: this file is not final")
	w.withdraw(t, "archived")

	ids, hidden := w.buyerList(t, w.laura)
	if len(ids) != 0 || hidden {
		t.Errorf("buyer list = %v hidden_own=%v, want nothing: this buyer never wrote in the thread", ids, hidden)
	}
	if seller := w.sellerVisibleToBuyer(t); seller[sellerOnly] {
		t.Errorf("the seller's list calls a withheld thread visible to the buyer")
	}
}

func TestTwoBuyersAreToldOnlyAboutConversationsTheyWroteIn(t *testing.T) {
	w := roomWithOneSharedFile(t)
	w.buyerOpens(t, w.laura, w.docID, "Laura's question")
	w.buyerOpens(t, w.mira, "", "Mira asks about rollout")
	w.withdraw(t, "archived")

	if _, hidden := w.buyerList(t, w.laura); !hidden {
		t.Errorf("Laura wrote in the withheld thread and was not told")
	}
	if _, hidden := w.buyerList(t, w.mira); hidden {
		t.Errorf("Mira never wrote in a withheld thread and was told one exists")
	}
}

func TestACommentInAnotherBuyersThreadCountsAsHavingWrittenInIt(t *testing.T) {
	w := roomWithOneSharedFile(t)
	lauras := w.buyerOpens(t, w.laura, w.docID, "Laura's question")
	if status := publicCall(t, w.e, "POST", "/v1/public/rooms/threads/"+lauras+"/comments",
		AnyMap{"body": "Mira adds a point"}, bearer(w.mira), nil); status != http.StatusOK && status != http.StatusCreated {
		t.Fatalf("Mira replies = %d", status)
	}
	w.withdraw(t, "archived")

	if _, hidden := w.buyerList(t, w.mira); !hidden {
		t.Errorf("Mira commented in the withheld thread and was not told")
	}
}

func TestAPausedRoomWithholdsNoNewLineAndReopensWithItsThreads(t *testing.T) {
	w := roomWithOneSharedFile(t)
	own := w.buyerOpens(t, w.laura, w.docID, "Is clause 4 negotiable?")

	if status := w.e.Call(t, "POST", "/v1/deal-rooms/"+w.room.roomID+"/pause", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("pause = %d", status)
	}
	ids, hidden := w.buyerList(t, w.laura)
	if len(ids) != 0 || hidden {
		t.Errorf("paused: threads=%v hidden_own=%v, want an empty list and no line: the buyer is told the room's state instead", ids, hidden)
	}
	if seller := w.sellerVisibleToBuyer(t); !seller[own] {
		t.Errorf("a pause marked the thread as withdrawn; room state is not part of the mark")
	}

	if status := w.e.Call(t, "POST", "/v1/deal-rooms/"+w.room.roomID+"/resume", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("resume = %d", status)
	}
	ids, hidden = w.buyerList(t, w.laura)
	if len(ids) != 1 || ids[0] != own || hidden {
		t.Errorf("reopened: threads=%v hidden_own=%v, want the thread back and no line", ids, hidden)
	}
}

func TestAMailFileLeavingTheDealsFilesAreaWithholdsTheBuyersConversation(t *testing.T) {
	w := roomSharing(t, true)
	own := w.buyerOpens(t, w.laura, w.docID, "Is the redline final?")
	w.withdraw(t, "unlinked")

	ids, hidden := w.buyerList(t, w.laura)
	if len(ids) != 0 || !hidden {
		t.Errorf("after the email left the deal: threads=%v hidden_own=%v, want none and the line", ids, hidden)
	}
	if w.sellerVisibleToBuyer(t)[own] {
		t.Errorf("the seller's list calls the withheld thread visible")
	}
}

func TestAWithdrawnFileSharedAgainBringsTheConversationBack(t *testing.T) {
	w := roomWithOneSharedFile(t)
	own := w.buyerOpens(t, w.laura, w.docID, "Is clause 4 negotiable?")
	w.withdraw(t, "hidden")
	if _, hidden := w.buyerList(t, w.laura); !hidden {
		t.Fatalf("the withdrawal was not announced")
	}

	if status := w.e.Call(t, "DELETE", "/v1/deals/"+w.room.dealID+"/documents/"+w.attachmentID+"/hide", nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("unhide = %d", status)
	}
	ids, hidden := w.buyerList(t, w.laura)
	if len(ids) != 1 || ids[0] != own || hidden {
		t.Errorf("shared again: threads=%v hidden_own=%v, want the thread back and no line", ids, hidden)
	}
	if !w.sellerVisibleToBuyer(t)[own] {
		t.Errorf("the seller's badge did not clear")
	}
}

func TestAClosedRoomServesItsConversationAndTheLineFollows(t *testing.T) {
	w := roomWithOneSharedFile(t)
	own := w.buyerOpens(t, w.laura, w.docID, "Is clause 4 negotiable?")
	room := w.buyerOpens(t, w.laura, "", "When does rollout start?")
	if status := w.e.Call(t, "POST", "/v1/deal-rooms/"+w.room.roomID+"/close", AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("close = %d", status)
	}
	ids, hidden := w.buyerList(t, w.laura)
	if len(ids) != 2 || hidden {
		t.Errorf("closed: threads=%v hidden_own=%v, want both threads and no line", ids, hidden)
	}
	w.withdraw(t, "archived")
	ids, hidden = w.buyerList(t, w.laura)
	if len(ids) != 1 || ids[0] != room || !hidden {
		t.Errorf("closed and withdrawn: threads=%v hidden_own=%v, want only %s and the line (thread %s)", ids, hidden, room, own)
	}
}

func TestAnExpiredRoomServesNoThreadsAndNoLine(t *testing.T) {
	w := roomWithOneSharedFile(t)
	w.buyerOpens(t, w.laura, w.docID, "Is clause 4 negotiable?")
	w.withdraw(t, "archived")
	expireRoom(t, w.e, w.room.roomID)

	ids, hidden := w.buyerList(t, w.laura)
	if len(ids) != 0 || hidden {
		t.Errorf("expired: threads=%v hidden_own=%v, want an empty list and no line", ids, hidden)
	}
}

func TestTheLineIsTheSameWhicheverDocumentTheBuyerAsksAbout(t *testing.T) {
	w := roomWithOneSharedFile(t)
	otherAttachment := uploadDealFile(t, w.e, w.room.dealID, "annex.pdf", []byte("%PDF-ANNEX"))
	var other AnyMap
	if status := w.e.Call(t, "POST", "/v1/deal-rooms/"+w.room.roomID+"/documents", AnyMap{
		"attachment_id": otherAttachment, "group_key": "legal", "source": "manual",
	}, nil, &other); status != http.StatusCreated {
		t.Fatalf("add second document = %d", status)
	}
	otherDoc, _ := other["id"].(string)
	w.buyerOpens(t, w.laura, w.docID, "About the first file")
	w.withdraw(t, "archived")
	if status := w.e.Call(t, "DELETE", "/v1/attachments/"+otherAttachment, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("archive second attachment = %d", status)
	}

	for _, filter := range []string{"", w.docID, otherDoc} {
		if _, hidden := w.buyerListFor(t, w.laura, filter); !hidden {
			t.Errorf("document_id=%q: no line, but the answer must not depend on the filter", filter)
		}
		if _, hidden := w.buyerListFor(t, w.mira, filter); hidden {
			t.Errorf("document_id=%q: Mira was told of a conversation she never wrote in", filter)
		}
	}
}

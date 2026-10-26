// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import (
	"sort"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
)

// The input's bounds. A room is small, but a long negotiation can carry many
// threads, and past these the prompt is spent on history nobody is waiting on.
const (
	maxThreads        = 12
	maxThreadComments = 4
	maxDocuments      = 30
	maxBuyers         = 20
	maxCommentRunes   = 400
)

// The two sides a comment can come from, as the prompt names them. The
// reader's colleagues are "your_side", so a model cannot attribute a
// colleague's words to the buyer.
const (
	speakerBuyer    = "buyer"
	speakerYourSide = "your_side"
)

// The contract's own words for a comment's side and a settled thread.
const (
	sideBuyer      = "buyer"
	threadResolved = "resolved"
)

// Input is the room as the model is shown it. Dates are calendar days in UTC,
// the form the grounding filter reads a stated date against.
type Input struct {
	Title        string       `json:"title"`
	State        string       `json:"state"`
	AccessEndsOn string       `json:"access_ends_on,omitempty"`
	Buyers       []BuyerIn    `json:"buyers"`
	Documents    []DocumentIn `json:"documents"`
	Threads      []ThreadIn   `json:"threads"`
}

// BuyerIn is one seat a buyer still holds. Revoked seats are left out: nobody
// is waiting on a buyer who can no longer enter.
type BuyerIn struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	SignedIn          bool     `json:"signed_in"`
	LastSeenOn        string   `json:"last_seen_on,omitempty"`
	AskedForNewLinkOn string   `json:"asked_for_new_link_on,omitempty"`
	Downloaded        []string `json:"downloaded,omitempty"`
}

// DocumentIn is one shared document.
type DocumentIn struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Group string `json:"group"`
}

// ThreadIn is one conversation, with its newest comments.
type ThreadIn struct {
	ID string `json:"id"`
	// DocumentID and About are empty for a thread about the whole room.
	DocumentID     string      `json:"document_id,omitempty"`
	About          string      `json:"about,omitempty"`
	Resolved       bool        `json:"resolved"`
	WaitingOnYou   bool        `json:"waiting_on_you"`
	RequiredChange bool        `json:"required_change,omitempty"`
	Comments       []CommentIn `json:"comments"`
	latest         time.Time
}

// CommentIn is one comment, newest last.
type CommentIn struct {
	Speaker string `json:"speaker"`
	Author  string `json:"author"`
	On      string `json:"on"`
	Body    string `json:"body"`
}

// FromRoom folds the room's reads into the input.
func FromRoom(
	room crmcontracts.DealRoom,
	participants []crmcontracts.DealRoomParticipant,
	documents []crmcontracts.DealRoomDocument,
	threads []crmcontracts.DealRoomThread,
) Input {
	in := Input{Title: room.Title, State: string(room.State)}
	if room.ExpiresAt != nil {
		in.AccessEndsOn = day(*room.ExpiresAt)
	}
	in.Buyers = buyersFrom(participants)
	titles := make(map[string]string, len(documents))
	for _, doc := range documents {
		titles[doc.Id.String()] = doc.Title
		if len(in.Documents) < maxDocuments {
			in.Documents = append(in.Documents, DocumentIn{
				ID: doc.Id.String(), Title: doc.Title, Group: string(doc.GroupKey),
			})
		}
	}
	in.Threads = threadsFrom(threads, titles)
	return in
}

func buyersFrom(participants []crmcontracts.DealRoomParticipant) []BuyerIn {
	out := make([]BuyerIn, 0, min(len(participants), maxBuyers))
	for _, p := range participants {
		if p.RevokedAt != nil || len(out) == maxBuyers {
			continue
		}
		buyer := BuyerIn{ID: p.Id.String(), Name: p.FullName, SignedIn: p.HasSignedIn != nil && *p.HasSignedIn}
		if p.LastSeenAt != nil {
			buyer.LastSeenOn = day(*p.LastSeenAt)
		}
		if linkStillOwed(p) {
			buyer.AskedForNewLinkOn = day(*p.LinkRequestedAt)
		}
		if p.DocumentsDownloaded != nil {
			buyer.Downloaded = *p.DocumentsDownloaded
		}
		out = append(out, buyer)
	}
	return out
}

// linkStillOwed reports a link request the buyer has not been in since. The
// request is kept as history, so a buyer who asked and then signed in again
// is no longer waiting on a link.
func linkStillOwed(p crmcontracts.DealRoomParticipant) bool {
	if p.LinkRequestedAt == nil {
		return false
	}
	return p.LastSeenAt == nil || p.LinkRequestedAt.After(*p.LastSeenAt)
}

// threadsFrom keeps the threads a reader most needs: the ones waiting on them
// first, then the most recently active.
func threadsFrom(threads []crmcontracts.DealRoomThread, titles map[string]string) []ThreadIn {
	out := make([]ThreadIn, 0, len(threads))
	for _, thread := range threads {
		folded := ThreadIn{
			ID:             thread.Id.String(),
			Resolved:       thread.State == threadResolved,
			RequiredChange: thread.RequiredChange,
			latest:         thread.CreatedAt,
		}
		if thread.DocumentId != nil {
			folded.DocumentID = thread.DocumentId.String()
			folded.About = titles[folded.DocumentID]
		}
		comments := thread.Comments
		if len(comments) > 0 {
			last := comments[len(comments)-1]
			folded.latest = last.CreatedAt
			folded.WaitingOnYou = !folded.Resolved && last.Author.Side == sideBuyer
		}
		for _, comment := range comments[max(0, len(comments)-maxThreadComments):] {
			folded.Comments = append(folded.Comments, commentFrom(comment))
		}
		out = append(out, folded)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].WaitingOnYou != out[j].WaitingOnYou {
			return out[i].WaitingOnYou
		}
		return out[i].latest.After(out[j].latest)
	})
	return out[:min(len(out), maxThreads)]
}

func commentFrom(comment crmcontracts.DealRoomComment) CommentIn {
	speaker := speakerYourSide
	if comment.Author.Side == sideBuyer {
		speaker = speakerBuyer
	}
	return CommentIn{
		Speaker: speaker,
		Author:  comment.Author.Name,
		On:      day(comment.CreatedAt),
		Body:    textcut.Runes(comment.Body, maxCommentRunes),
	}
}

func day(at time.Time) string {
	return at.UTC().Format(time.DateOnly)
}

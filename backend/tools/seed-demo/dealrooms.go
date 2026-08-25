// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The buyer's side of a deal: a room, the paper it puts in front of them, the
// people invited to read it, and the questions they asked.
//
// Everything else this tool writes is the SELLER's view. A deal room is the
// one surface a customer sees, and a demo that opens one to an empty page
// cannot show what the product is for -- the room is only worth anything when
// there are documents in it and somebody has asked something about one.
//
// The documents are next door in dealroompaper.go, because a room's paper
// turned out to be a concept of its own: a room document cannot reuse the
// contract PDF the account already holds, and the reasons fill a file.
//
// Rooms are seeded LIVE. A room is created live and reaches nobody until
// somebody is invited, so an uninvited room is private without a second gate.
// The invitations here are the demo's whole point, so they follow immediately.

import (
	"fmt"
	"net/url"
	"strings"
)

// demoDealRoom is one buyer-facing room, named by the deal it projects.
type demoDealRoom struct {
	Ref  string `json:"ref"`
	Deal string `json:"deal"`
	// Title and Welcome are what the buyer reads first. Both are written in
	// the CUSTOMER's language, like the contract PDFs and unlike the internal
	// notes -- a room is correspondence, not a working note.
	Title   string `json:"title"`
	Welcome string `json:"welcome_message"`
	// Steward is a user ref. Omitted, the deal's owner keeps it, which is what
	// the API does on its own; naming one is for the case where the person a
	// buyer should ask is not the person carrying the number.
	Steward string `json:"steward"`
	// Participants are the buyer's people. Every one of them is somebody who
	// already exists on the account, named by email so the dataset cannot
	// invent a contact the room would then be alone in knowing.
	Participants []demoRoomParticipant `json:"participants"`
	// Documents are contracts by ref. The room shows the paper of the deal it
	// belongs to, so each named contract must sit on that same deal.
	Documents []demoRoomDocument `json:"documents"`
	// Threads are the questions asked and what was answered.
	Threads []demoRoomThread `json:"threads"`
}

type demoRoomParticipant struct {
	// Email identifies a person already seeded on the account. FullName is
	// what the room shows; it is restated rather than looked up because the
	// invitation carries its own copy and a buyer sees that one.
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	// Capability is `view` or `comment`. Left empty the API defaults to view,
	// which is the least that lets somebody read the room.
	Capability string `json:"capability"`
}

type demoRoomDocument struct {
	Contract string `json:"contract"`
	Group    string `json:"group"`
	Title    string `json:"title"`
}

// demoRoomThread is one exchange. Opener is the question, Replies are what
// followed it in order, and the seeder posts them as the SELLER's side --
// which is the only side this tool can write, since a buyer comment arrives
// through a credential nobody here holds.
type demoRoomThread struct {
	// Document names a room document by CONTRACT ref, so a thread hangs off
	// the paper it is about. Omitted, the thread is room-level.
	Document string `json:"document"`
	// RequiredChange marks a thread the seller still owes an answer on. Only
	// meaningful on a document thread.
	RequiredChange bool     `json:"required_change"`
	Body           string   `json:"body"`
	Replies        []string `json:"replies"`
	// Resolved closes the thread: the seller saying the point is settled.
	Resolved bool `json:"resolved"`
}

// dealRoomCounts is what this phase wrote.
type dealRoomCounts struct {
	rooms, documents, participants, threads, comments int
}

// seedDealRooms opens a room per dataset entry and fills it.
//
// Runs after seedPaper, and must: a room document points at an attachment on
// the deal, and the contracts have to exist before their paper can be
// rendered from them.
func seedDealRooms(c *client, cfg demoConfig, refs pipelineRefs, mode runMode) (dealRoomCounts, error) {
	var n dealRoomCounts
	for _, room := range cfg.DealRooms {
		dealID, err := dealIDFor(c, cfg, refs, room.Deal)
		if err != nil {
			return n, fmt.Errorf("deal room %s: %w", room.Ref, err)
		}
		if dealID == "" {
			return n, fmt.Errorf("deal room %s names deal %q, which is not seeded", room.Ref, room.Deal)
		}
		if mode == modeDryRun {
			n.rooms++
			n.documents += len(room.Documents)
			n.participants += len(room.Participants)
			n.threads += len(room.Threads)
			continue
		}

		roomID, created, err := ensureDealRoom(c, room, dealID, refs)
		if err != nil {
			return n, fmt.Errorf("deal room %s: %w", room.Ref, err)
		}
		if created {
			n.rooms++
		}

		docIDs, added, err := seedRoomDocuments(c, room, roomID, dealID, refs)
		if err != nil {
			return n, fmt.Errorf("deal room %s: %w", room.Ref, err)
		}
		n.documents += added

		invited, err := seedRoomParticipants(c, room, roomID)
		if err != nil {
			return n, fmt.Errorf("deal room %s: %w", room.Ref, err)
		}
		n.participants += invited

		threads, comments, err := seedRoomThreads(c, room, roomID, docIDs)
		if err != nil {
			return n, fmt.Errorf("deal room %s: %w", room.Ref, err)
		}
		n.threads += threads
		n.comments += comments
	}
	return n, nil
}

// ensureDealRoom opens the room, or finds the one a previous run opened.
//
// Convergence is the deal: at most one active room per deal, and a second
// create answers 409 `deal_room_already_open`. So the listing by deal_id is
// the whole of the check -- there is no name to match on and no second room
// to confuse it with.
func ensureDealRoom(c *client, room demoDealRoom, dealID string, refs pipelineRefs) (id string, created bool, err error) {
	existing, err := findDealRoom(c, dealID)
	if err != nil {
		return "", false, err
	}
	if existing != "" {
		return existing, false, nil
	}

	body := jsonBody{
		"deal_id": dealID,
		"title":   room.Title,
		"source":  seedSource,
	}
	addIfSet(body, "welcome_message", room.Welcome)
	if steward, ok := refs.usersByRef[room.Steward]; ok {
		body["steward_user_id"] = steward
	}

	var out struct {
		ID string `json:"id"`
	}
	if err := c.post("/v1/deal-rooms", body, &out); err != nil {
		// The already-open refusal names nothing, so re-read rather than
		// parse it: a room created moments ago is exactly the case a search
		// behind an index has not caught up with, and the listing by deal is
		// a direct read rather than a search.
		if isConflict(err) {
			again, findErr := findDealRoom(c, dealID)
			if findErr != nil {
				return "", false, findErr
			}
			if again != "" {
				return again, false, nil
			}
		}
		return "", false, fmt.Errorf("opening the room: %w", err)
	}
	return out.ID, true, nil
}

func findDealRoom(c *client, dealID string) (string, error) {
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	query := url.Values{"deal_id": {dealID}, "limit": {"5"}}
	if err := c.get("/v1/deal-rooms", query, &page); err != nil {
		return "", fmt.Errorf("listing rooms for deal %s: %w", dealID, err)
	}
	if len(page.Data) == 0 {
		return "", nil
	}
	return page.Data[0].ID, nil
}

// seedRoomParticipants invites the buyer's people.
//
// Re-inviting an address already in the room is refused
// (409 `deal_room_participant_already_invited`), which is the convergence:
// the listing says who is in, and anybody already there is left alone rather
// than re-invited, because a fresh invitation invalidates the link the last
// one sent.
func seedRoomParticipants(c *client, room demoDealRoom, roomID string) (int, error) {
	if len(room.Participants) == 0 {
		return 0, nil
	}
	onFile, err := dealRoomParticipants(c, roomID)
	if err != nil {
		return 0, err
	}
	invited := 0
	for _, person := range room.Participants {
		if onFile[strings.ToLower(person.Email)] {
			continue
		}
		body := jsonBody{
			"full_name": person.FullName,
			"email":     person.Email,
			"source":    seedSource,
		}
		addIfSet(body, "capability", person.Capability)
		if err := c.post("/v1/deal-rooms/"+roomID+"/participants", body, nil); err != nil {
			if isConflict(err) {
				continue
			}
			return invited, fmt.Errorf("inviting %s: %w", person.Email, err)
		}
		invited++
	}
	return invited, nil
}

func dealRoomParticipants(c *client, roomID string) (map[string]bool, error) {
	var page struct {
		Data []struct {
			Email string `json:"email"`
		} `json:"data"`
	}
	if err := c.get("/v1/deal-rooms/"+roomID+"/participants", nil, &page); err != nil {
		return nil, fmt.Errorf("listing the room's participants: %w", err)
	}
	out := make(map[string]bool, len(page.Data))
	for _, row := range page.Data {
		out[strings.ToLower(row.Email)] = true
	}
	return out, nil
}

// seedRoomThreads opens the questions and answers them.
//
// Convergence is the opening comment's body: a thread is identified by what
// it asked, because the API mints the id and the dataset has no way to name
// one. Two threads opening with the same sentence would be one thread here,
// which is a constraint the dataset note states rather than a bug to work
// around -- two identical questions in one room is not a demo anybody wants.
func seedRoomThreads(c *client, room demoDealRoom, roomID string, docIDs map[string]string) (threads, comments int, err error) {
	if len(room.Threads) == 0 {
		return 0, 0, nil
	}
	onFile, err := dealRoomThreads(c, roomID)
	if err != nil {
		return 0, 0, err
	}

	for _, thread := range room.Threads {
		existing, seen := onFile[thread.Body]
		threadID := existing.id
		if !seen {
			body := jsonBody{"body": thread.Body, "source": seedSource}
			if thread.Document != "" {
				docID, ok := docIDs[thread.Document]
				if !ok {
					return threads, comments, fmt.Errorf(
						"thread names document %q, which this room does not show", thread.Document)
				}
				body["document_id"] = docID
				if thread.RequiredChange {
					body["required_change"] = true
				}
			}
			var out struct {
				ID string `json:"id"`
			}
			if err := c.post("/v1/deal-rooms/"+roomID+"/threads", body, &out); err != nil {
				return threads, comments, fmt.Errorf("opening a thread: %w", err)
			}
			threadID = out.ID
			threads++
		}

		// Replies are matched by body too, so a re-run adds only what is
		// missing rather than saying everything twice.
		for _, reply := range thread.Replies {
			if seen && existing.comments[reply] {
				continue
			}
			if err := c.post("/v1/deal-rooms/"+roomID+"/threads/"+threadID+"/comments",
				jsonBody{"body": reply, "source": seedSource}, nil); err != nil {
				return threads, comments, fmt.Errorf("replying in a thread: %w", err)
			}
			comments++
		}

		// Resolving an already resolved thread answers 200 unchanged, so this
		// needs no check of its own.
		if thread.Resolved && !existing.resolved {
			if err := c.post("/v1/deal-rooms/"+roomID+"/threads/"+threadID+"/resolve", jsonBody{}, nil); err != nil {
				return threads, comments, fmt.Errorf("resolving a thread: %w", err)
			}
		}
	}
	return threads, comments, nil
}

// roomThread is one thread as it stands: its id, what has been said in it,
// and whether it is closed.
type roomThread struct {
	id       string
	comments map[string]bool
	resolved bool
}

func dealRoomThreads(c *client, roomID string) (map[string]roomThread, error) {
	var page struct {
		Data []struct {
			ID       string `json:"id"`
			Resolved bool   `json:"resolved"`
			Comments []struct {
				Body string `json:"body"`
			} `json:"comments"`
		} `json:"data"`
	}
	if err := c.get("/v1/deal-rooms/"+roomID+"/threads", nil, &page); err != nil {
		return nil, fmt.Errorf("listing the room's threads: %w", err)
	}
	out := make(map[string]roomThread, len(page.Data))
	for _, row := range page.Data {
		if len(row.Comments) == 0 {
			continue
		}
		said := make(map[string]bool, len(row.Comments))
		for _, comment := range row.Comments {
			said[comment.Body] = true
		}
		// Keyed by the OPENING comment, which is what the dataset writes as
		// the thread's body.
		out[row.Comments[0].Body] = roomThread{id: row.ID, comments: said, resolved: row.Resolved}
	}
	return out, nil
}

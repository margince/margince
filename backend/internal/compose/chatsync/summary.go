// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// Summaries: a stretch of channel talk is read again from the chat. The
// chat_summary task's episode site judges it, and it is filed as one note.
// The messages live only in memory for the call; the note keeps the summary.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/chatsummary"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// summariser asks the episode site on its lane, reading the extra context
// from pool.
type summariser struct {
	lane       ai.Completer
	pool       *pgxpool.Pool
	rbac       identity.RbacObjects
	restricted func(context.Context, []string) (map[string]bool, error)
}

// errNoModel is an installation with no chat_summary lane: nothing is filed.
var errNoModel = errors.New("chat summary: no model is bound to the chat_summary task")

// errUnmatchableAuthor passes an episode over without filing it. A judged
// message carried no address to match against a restriction, or every author
// was restricted, so nothing can be safely summarised. Callers treat it like
// errNoModel: file nothing, log it as passed over rather than failed.
var errUnmatchableAuthor = errors.New("chat summary: an episode's authors cannot be cleared for summary")

// restrictedMarker tells the model the episode is partial, with no name and no
// text. The note it writes does not then read as the whole conversation.
const restrictedMarker = "Some messages were left out because a participant restricted processing of their data."

// summarise asks the episode site about messages of the link's channel. A lane
// that cannot answer at all reads as errNoModel. p.restricted answers which
// author addresses to withhold; a message from one is dropped before the model
// sees the talk.
func (p summariser) summarise(ctx context.Context, link chatlinks.Link, record chatRecord, messages []dispact.Message, chosen bool) (chatsummary.Reply, error) {
	if p.lane == nil {
		return chatsummary.Reply{}, errNoModel
	}
	messages, dropped, err := clearedMessages(ctx, messages, p.restricted)
	if err != nil {
		return chatsummary.Reply{}, err
	}
	in := chatsummary.Input{Record: record.kind + " " + record.name, Channel: link.ChannelName, Chosen: chosen}
	if record.stage != "" {
		in.Facts = append(in.Facts, "Stage: "+record.stage)
	}
	if dropped {
		in.Facts = append(in.Facts, restrictedMarker)
	}
	extra, err := readSummaryContext(ctx, p.pool, p.rbac, link, record, messages[0].PostedAt)
	if err != nil {
		return chatsummary.Reply{}, err
	}
	in.Facts = append(in.Facts, extra.facts...)
	in.Earlier = extra.earlier
	in.Messages = transcript(messages, extra.place)
	about := &model.Subjects{}
	if link.DealID != nil {
		about.Deals = append(about.Deals, *link.DealID)
	}
	if link.ProjectID != nil {
		about.Projects = append(about.Projects, *link.ProjectID)
	}
	check := func(raw string) error {
		reply, err := chatsummary.Parse(raw)
		if err == nil {
			err = chatsummary.Check(reply)
		}
		if err == nil && chosen && !reply.Relevant {
			err = errors.New("a colleague chose this talk, so summarise it")
		}
		return err
	}
	ctx = ai.WithSubject(ctx, recordRef(record), record.name)
	res, err := ai.Ask(ctx, p.lane, chatsummary.Request(in, about), check)
	if errors.Is(err, ai.ErrUnconfiguredModel) {
		return chatsummary.Reply{}, errNoModel
	}
	if err != nil {
		return chatsummary.Reply{}, err
	}
	// Judged again: a lane that cannot re-ask hands back the reply unchecked.
	if err := check(res.Text); err != nil {
		return chatsummary.Reply{}, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err)
	}
	return chatsummary.Parse(res.Text)
}

// transcript is the talk as the model reads it. The email the chat gives an
// author only places them and is never sent.
func transcript(messages []dispact.Message, place placing) []chatsummary.Message {
	out := make([]chatsummary.Message, 0, len(messages))
	for _, m := range messages {
		said := chatsummary.Message{ID: m.TS, Author: authorOf(m), Side: place.sideOf(m.UserEmail), At: m.PostedAt, Text: m.Text}
		if m.ThreadTS != "" && m.ThreadTS != m.TS {
			said.ReplyTo = m.ThreadTS
		}
		for _, f := range m.Files {
			said.Files = append(said.Files, chatsummary.File{Name: f.Name, Type: f.MimeType})
		}
		out = append(out, said)
	}
	return out
}

// authorOf is the name the chat shows. An author with no name is "someone",
// never their email.
func authorOf(m dispact.Message) string {
	if m.UserName != "" {
		return m.UserName
	}
	return "someone"
}

// placing is what tells a colleague: the seat addresses and the vouched-for
// own domains.
type placing struct {
	seats storekit.SeatAddresses
	own   []string
}

// sideOf places an author by their chat email. A proven seat address is a
// colleague, as for any channel address with no mailbox owner. Otherwise the
// deal-evidence rule judges the domain (deals.AddressSide).
func (p placing) sideOf(email string) chatsummary.Side {
	if email == "" {
		return chatsummary.SideUnknown
	}
	if p.seats.Covers(email, ids.Nil) {
		return chatsummary.SideColleague
	}
	switch deals.AddressSide(email, p.own) {
	case deals.AuthorSeller:
		return chatsummary.SideColleague
	case deals.AuthorUnknown:
		return chatsummary.SideUnknown
	}
	return chatsummary.SideOutside
}

// authorEmail is the address a chat message carries for matching a restriction,
// lowercased, empty when the chat named none. authorOf keeps its own rule for
// what a reader sees; this is only for the match.
func authorEmail(m dispact.Message) string {
	return strings.ToLower(strings.TrimSpace(m.UserEmail))
}

// authorEmails returns the match addresses the messages carry, lowercased and
// oldest first, skipping a message the chat named no address for. Such a message
// stays in rather than passing the whole episode over, so a channel with
// non-contact authors is still summarised. A restricted contact is matched by
// its own address.
func authorEmails(messages []dispact.Message) []string {
	emails := make([]string, 0, len(messages))
	for _, m := range messages {
		if email := authorEmail(m); email != "" {
			emails = append(emails, email)
		}
	}
	return emails
}

// keepUnrestricted drops every message whose author is restricted. dropped
// marks that at least one was left out, so the caller can tell the model the
// talk it reads is partial.
func keepUnrestricted(messages []dispact.Message, restricted map[string]bool) (kept []dispact.Message, dropped bool) {
	kept = make([]dispact.Message, 0, len(messages))
	for _, m := range messages {
		if restricted[authorEmail(m)] {
			dropped = true
			continue
		}
		kept = append(kept, m)
	}
	return kept, dropped
}

// clearedMessages drops every message whose author the summary must withhold,
// and reports whether any was dropped so the caller can mark the talk partial.
// It passes the episode over with errUnmatchableAuthor only when every message
// was dropped, so nothing is left to summarise.
func clearedMessages(ctx context.Context, messages []dispact.Message,
	restricted func(context.Context, []string) (map[string]bool, error),
) (kept []dispact.Message, dropped bool, err error) {
	blocked, err := restricted(ctx, authorEmails(messages))
	if err != nil {
		return nil, false, err
	}
	kept, dropped = keepUnrestricted(messages, blocked)
	if len(kept) == 0 {
		return nil, false, errUnmatchableAuthor
	}
	return kept, dropped, nil
}

// lookupRestricted answers which of emails the summary must withhold, reading
// contacts in one workspace transaction off pool. Both summary callers hold a
// pool and the contacts store, so each builds its checker from this.
func lookupRestricted(ctx context.Context, pool *pgxpool.Pool, store *contacts.Store, emails []string) (map[string]bool, error) {
	// The restriction check is a privacy enforcement the chat engine owns, not
	// the asking member's query. It runs under the engine's system actor on both
	// the pull and the file-message path, and discloses nothing.
	ctx = principal.WithActor(ctx, chatLinksPrincipal)
	var blocked map[string]bool
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		blocked, err = store.RestrictedEmailsTx(ctx, tx, emails)
		return err
	})
	return blocked, err
}

// talk is what a note says about the messages it summarises. It holds no
// message text.
type talk struct {
	key     string
	count   int
	started time.Time
	ended   time.Time
	url     string
}

// talkOf is what a note says about messages, oldest first.
func talkOf(key, url string, messages []dispact.Message) talk {
	return talk{
		key: key, url: url, count: len(messages),
		started: messages[0].PostedAt, ended: messages[len(messages)-1].PostedAt,
	}
}

func recordRef(record chatRecord) ids.Ref { return ids.Ref{Type: record.kind, ID: record.id} }

// errSummaryKeyTaken is a note under the talk's key that the chat engine did
// not write, which must never stand in for its summary.
var errSummaryKeyTaken = fmt.Errorf("chat summary: another writer holds this talk's key: %w", apperrors.ErrConflict)

// summarySource is the source of every chat summary note; a row under a
// summary key with any other source was not written here.
const summarySource = "chat_links"

// filing is what a note needs besides the summary: the transaction it is
// written in, the stores, and the link it is filed under.
type filing struct {
	pool  *pgxpool.Pool
	store *chatlinks.Store
	acts  *activities.Store
	link  chatlinks.Link
}

// fileNote files the summary as one note on the link's records. It is keyed
// on the talk and the record, so a replay returns the note already there. A
// record that takes the channel over files its own note. ctx carries the actor: the
// chat engine for a pull, the member for a message they chose. The link is
// checked live in the same transaction, so a note never lands after an unlink.
func fileNote(ctx context.Context, f filing, said talk, reply chatsummary.Reply) (crmcontracts.Activity, error) {
	link := f.link
	system := provenance.ChatSummarySource
	raw := map[string]any{
		"message_count": said.count,
		"ended_at":      said.ended.UTC().Format(time.RFC3339),
		"url":           said.url,
	}
	record := linkRecord(link)
	key := said.key + ":" + record.Type + ":" + record.ID.String()
	body := noteBody(reply)
	// Both the title and the summary are the model's words, in the talk's language.
	written := textlang.Detect(reply.Summary)
	in := activities.LogActivityInput{
		Kind: string(crmcontracts.ActivityKindNote),
		SystemText: &systemtext.Message{
			Version: 1,
			Subject: systemtext.Prose(systemtext.Generated, written), Body: systemtext.Prose(systemtext.Generated, written),
		},
		Subject:      &reply.Title,
		Body:         &body,
		OccurredAt:   &said.started,
		SourceSystem: &system,
		SourceID:     &key,
		Raw:          &raw,
		Source:       summarySource,
		Origin:       activities.OriginAgent,
	}
	var note crmcontracts.Activity
	err := database.WithWorkspaceTx(ctx, f.pool, func(tx pgx.Tx) error {
		if err := f.store.HoldLiveLinkTx(ctx, tx, link); err != nil {
			return err
		}
		filed, links, err := priorNote(ctx, tx, link, said.key)
		created := false
		switch {
		case err != nil:
			return err
		case !filed.IsZero():
			note, err = f.acts.GetActivityTx(ctx, tx, ids.ActivityID{UUID: filed}, storekit.IncludeArchived, activities.CustomColumns{})
		default:
			in.Links = links
			note, created, err = f.acts.LogActivityTx(ctx, tx, in)
		}
		if err == nil && !created && note.Source != summarySource {
			return errSummaryKeyTaken
		}
		return err
	})
	if err != nil {
		return crmcontracts.Activity{}, fmt.Errorf("chat summary: filing the note for %s: %w", key, err)
	}
	return note, nil
}

// priorNote finds a note of the talk on the link's own record. It looks under
// every key a summary of it was filed with, the old one without a record too.
// With none, it answers the records a new note is linked to. A project that
// already holds the talk through another deal's note is left out.
func priorNote(ctx context.Context, tx pgx.Tx, link chatlinks.Link, talkKey string) (ids.UUID, []activities.ActivityLinkInput, error) {
	engine := principal.WithActor(ctx, chatLinksPrincipal)
	escaped := storekit.EscapeLike(talkKey)
	keys := []string{escaped, escaped + ":" + chatlinks.RecordDeal + ":%", escaped + ":" + chatlinks.RecordProject + ":%"}
	record := linkRecord(link)
	filed, err := activities.SourceFiledOnTx(engine, tx, provenance.ChatSummarySource, keys, record.Type, record.ID)
	if err != nil || !filed.IsZero() {
		return filed, nil, err
	}
	links := []activities.ActivityLinkInput{{EntityType: record.Type, EntityID: record.ID}}
	if link.ProjectID == nil || record.Type == chatlinks.RecordProject {
		return ids.Nil, links, nil
	}
	held, err := activities.SourceFiledOnTx(engine, tx, provenance.ChatSummarySource, keys, chatlinks.RecordProject, *link.ProjectID)
	if err != nil || !held.IsZero() {
		return ids.Nil, links, err
	}
	return ids.Nil, append(links, activities.ActivityLinkInput{EntityType: chatlinks.RecordProject, EntityID: *link.ProjectID}), nil
}

// noteHeadings name the lists under a summary, in the language it is written in.
var noteHeadings = map[textlang.Lang][2]string{
	textlang.English:    {"Decisions", "Open questions"},
	textlang.German:     {"Entscheidungen", "Offene Fragen"},
	textlang.Vietnamese: {"Quyết định", "Câu hỏi còn mở"},
}

// noteBody is the summary, then the decisions and the open questions.
func noteBody(reply chatsummary.Reply) string {
	headings, ok := noteHeadings[textlang.Detect(reply.Summary)]
	if !ok {
		headings = noteHeadings[textlang.English]
	}
	var body strings.Builder
	body.WriteString(reply.Summary)
	for i, items := range [][]string{reply.Decisions, reply.OpenQuestions} {
		if len(items) == 0 {
			continue
		}
		body.WriteString("\n\n" + headings[i] + ":")
		for _, item := range items {
			body.WriteString("\n- " + item)
		}
	}
	return body.String()
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Reading the text of the notes and transcripts a meeting carries, for the
// meeting-notes connection that runs the sync. This side decides whose
// connection may read and which Docs; the activities module owns the meeting,
// its document links and the transcript the text becomes.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// meetingNotesWindow is how far back a meeting's notes are still read. Google
// attaches them within hours of the call, so an older meeting whose Doc never
// opened is not asked about again.
const meetingNotesWindow = 14 * 24 * time.Hour

// maxMeetingNotesPerSync bounds the Docs one sync opens.
const maxMeetingNotesPerSync = 20

// actionMeetingNotesRefused is the system_log action of a refused read grant.
const actionMeetingNotesRefused = "capture_meeting_notes_refused"

// MeetingNotesQuery asks for the notes Docs one owner's meetings carry.
type MeetingNotesQuery struct {
	Owner ids.UserID
	// Domain is the owner's own domain; only a meeting organized on it counts.
	Domain       string
	Since, Until time.Time
	Limit        int
}

// MeetingNotesLink is one notes Doc link on a meeting, by its address.
type MeetingNotesLink struct {
	Meeting ids.ActivityID
	URL     string
}

// MeetingNotesLanding is one Doc's text, for the meeting that carries it.
type MeetingNotesLanding struct {
	Meeting ids.ActivityID
	URL     string
	Owner   ids.UserID
	Domain  string
	Title   string
	// Revision is the Doc revision the text was read from.
	Revision string
	Text     string
}

// MeetingNotesWriter is the activities module's side of reading notes.
type MeetingNotesWriter interface {
	// DueTx answers the notes links that no transcript holds yet.
	DueTx(ctx context.Context, tx pgx.Tx, q MeetingNotesQuery) ([]MeetingNotesLink, error)
	// LandTx checks the meeting still carries the link and lands the text once.
	// It answers false when nothing landed.
	LandTx(ctx context.Context, tx pgx.Tx, in MeetingNotesLanding) (bool, error)
}

// WithNotesLanding returns a copy that reads meeting notes through notes.
func (s *Sink) WithNotesLanding(notes MeetingNotesWriter) *Sink {
	c := *s
	c.meetingNotes = notes
	return &c
}

// WithClock returns a copy that dates the meeting-notes window by now.
func (s *Sink) WithClock(now func() time.Time) *Sink {
	c := *s
	c.now = now
	return &c
}

func (s *Sink) clock() time.Time {
	if s.now == nil {
		return time.Now()
	}
	return s.now()
}

var _ connector.MeetingNotesKeeper = (*Sink)(nil)

// notesReader is the connection a sync runs for, when it may read notes.
type notesReader struct {
	owner  ids.UserID
	domain string
}

// meetingNotesReaderTx answers the connection the sync runs for, if it may read
// notes now. It must be live and hold the read grant. It must also be on the
// account of its owner's live calendar connection, because notes are read only
// for that calendar's meetings.
func meetingNotesReaderTx(ctx context.Context, tx pgx.Tx) (notesReader, bool, error) {
	run, bound := ctx.Value(mailboxConnectionKey{}).(mailboxConnection)
	if !bound {
		return notesReader{}, false, nil
	}
	var (
		provider string
		owner    ids.UserID
		label    *string
		scopes   []string
	)
	err := tx.QueryRow(ctx, `
		SELECT provider, user_id, account_label, provider_scopes FROM capture_connection
		 WHERE id = @id AND generation = @generation
		   AND status IN ('connected','error') AND archived_at IS NULL`,
		pgx.NamedArgs{"id": run.ID, "generation": run.Generation}).Scan(&provider, &owner, &label, &scopes)
	if errors.Is(err, pgx.ErrNoRows) {
		return notesReader{}, false, nil
	}
	if err != nil {
		return notesReader{}, false, fmt.Errorf("capture: reading the meeting-notes connection: %w", err)
	}
	calendar, pairs := meetingNotesCalendars[provider]
	if !pairs || !MeetingNotesGranted(provider, scopes) || label == nil {
		return notesReader{}, false, nil
	}
	var calendarLabel *string
	err = tx.QueryRow(ctx, `
		SELECT account_label FROM capture_connection
		 WHERE user_id = $1 AND provider = $2 AND status IN ('connected','error') AND archived_at IS NULL`,
		owner, calendar).Scan(&calendarLabel)
	if errors.Is(err, pgx.ErrNoRows) {
		return notesReader{}, false, nil
	}
	if err != nil {
		return notesReader{}, false, fmt.Errorf("capture: reading the calendar a notes connection pairs with: %w", err)
	}
	account := strings.ToLower(bareAddress(*label))
	if calendarLabel == nil || account == "" || account != strings.ToLower(bareAddress(*calendarLabel)) {
		return notesReader{}, false, nil
	}
	domain := owndomain.OfAddress(account)
	if domain == "" {
		return notesReader{}, false, nil
	}
	return notesReader{owner: owner, domain: domain}, true, nil
}

// MeetingNotesDue answers the notes Docs on the reader's own meetings of the
// last meetingNotesWindow that no transcript holds yet.
func (s *Sink) MeetingNotesDue(ctx context.Context) ([]connector.MeetingNotesDoc, error) {
	if s.meetingNotes == nil {
		return nil, nil
	}
	var out []connector.MeetingNotesDoc
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		reader, ok, err := meetingNotesReaderTx(ctx, tx)
		if err != nil || !ok {
			return err
		}
		now := s.clock()
		links, err := s.meetingNotes.DueTx(ctx, tx, MeetingNotesQuery{
			Owner: reader.owner, Domain: reader.domain,
			Since: now.Add(-meetingNotesWindow), Until: now, Limit: maxMeetingNotesPerSync,
		})
		if err != nil {
			return err
		}
		for _, link := range links {
			if fileID, ok := meetingDocumentFileID(link.URL); ok {
				out = append(out, connector.MeetingNotesDoc{Meeting: link.Meeting.UUID, FileID: fileID})
			}
		}
		return nil
	})
	return out, err
}

// KeepMeetingNotes lands one Doc's text on its meeting. Under the write it asks
// again about the connection and the member's right to capture. The meeting is
// asked again whether it still carries the Doc.
func (s *Sink) KeepMeetingNotes(ctx context.Context, doc connector.MeetingNotesDoc, notes connector.MeetingNotesText) error {
	if s.meetingNotes == nil {
		return nil
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		ctx, err := s.mailboxRunAllowedTx(ctx, tx)
		if err != nil {
			return err
		}
		reader, ok, err := meetingNotesReaderTx(ctx, tx)
		if err != nil || !ok {
			return err
		}
		landed, err := s.meetingNotes.LandTx(ctx, tx, MeetingNotesLanding{
			Meeting: ids.From[ids.ActivityKind](doc.Meeting), URL: meetingDocumentURL(doc.FileID),
			Owner: reader.owner, Domain: reader.domain,
			Title: notes.Title, Revision: notes.Revision, Text: notes.Text,
		})
		if err == nil && !landed {
			slog.InfoContext(ctx, "capture: a meeting's notes document was read and not landed",
				"meeting_id", doc.Meeting.String())
		}
		return err
	})
}

// MeetingNotesRefused takes the read grant off the connection's recorded
// scopes. Connections then shows the row cannot read notes and offers a
// reconnect, and no later sync opens a Doc until the owner grants it again.
func (s *Sink) MeetingNotesRefused(ctx context.Context) error {
	run, bound := ctx.Value(mailboxConnectionKey{}).(mailboxConnection)
	if !bound {
		return nil
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var provider string
		var scopes []string
		err := tx.QueryRow(ctx, `
			SELECT provider, coalesce(provider_scopes, '{}') FROM capture_connection
			 WHERE id = $1 AND generation = $2 AND archived_at IS NULL FOR UPDATE`,
			run.ID, run.Generation).Scan(&provider, &scopes)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("capture: locking the connection whose notes grant was refused: %w", err)
		}
		kept := slices.DeleteFunc(slices.Clone(scopes), func(scope string) bool {
			return slices.Contains(meetingNotesScopes[provider], scope)
		})
		if len(kept) == len(scopes) {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE capture_connection SET provider_scopes = $2 WHERE id = $1`, run.ID, kept); err != nil {
			return fmt.Errorf("capture: recording the refused notes grant: %w", err)
		}
		_, err = storekit.LogSystem(ctx, tx, actionMeetingNotesRefused, map[string]any{
			detailConnectionID: run.ID.String(), "provider": provider,
		})
		return err
	})
}

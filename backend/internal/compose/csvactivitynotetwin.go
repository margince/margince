// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One import run files one copy of a repeated note. Another CRM's export lists
// a note once for each record it is associated with, each under its own id.
//
// The identity map holds a key made from the run and the note's content. A twin
// in a later slice finds the note an earlier slice filed through it. A key names
// its run, and a note imported by two runs is two imports.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// noteTwinObject is the identity-map object a content key is filed under. A
// row's object is its part's, one of the importable objects, so no row id can
// land in or be read from this key space.
const noteTwinObject = "activity_note_twin"

// noteTwinKey names the note a row files within one run: its records, subject,
// body, time, author and owner. It is empty for every kind but a note.
// The dry run adds the links cell as spelled, because a row of the same upload
// has no record yet.
func noteTwinKey(runID migration.RunID, in activities.LogActivityInput, spelled string) string {
	if in.Kind != string(crmcontracts.ActivityKindNote) || in.OccurredAt == nil {
		return ""
	}
	links := make([]string, 0, len(in.Links))
	for _, link := range in.Links {
		links = append(links, link.EntityType+":"+link.EntityID.String())
	}
	slices.Sort(links)
	owner := ""
	if in.Author.AuthorID != nil {
		owner = in.Author.AuthorID.String()
	}
	content := strings.Join([]string{
		runID.String(), strings.Join(links, ";"), spelled,
		derefString(in.Subject), derefString(in.Body), in.OccurredAt.UTC().Format(time.RFC3339Nano),
		derefString(in.Author.AuthorName), owner,
	}, "\x00")
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// filedNoteTwin answers the note this run already filed for the same content.
// A note archived since still counts: the copy must not bring it back.
func (w *csvWriters) filedNoteTwin(ctx context.Context, twin string) (ids.UUID, bool, error) {
	if twin == "" {
		return ids.UUID{}, false, nil
	}
	binding, found, err := w.identities.LookupBinding(ctx, w.sourceSystem, noteTwinObject, twin)
	if err != nil || !found || binding.RunID != w.runID {
		return ids.UUID{}, false, err
	}
	return binding.NativeID, true, nil
}

// collapseOntoNote binds a repeated row to the note its twin filed, so a
// re-upload finds the row landed, and reports the row as skipped.
func (w *csvWriters) collapseOntoNote(ctx context.Context, row migration.Row, kept ids.UUID) (migration.EnsureResult, error) {
	if err := w.bind(ctx, row, kept); err != nil {
		return migration.EnsureResult{}, err
	}
	return skippedFor(migration.Why(migration.ReasonNoteRepeated)), nil
}

// recordNoteTwinTx maps the content key to the note just filed, in the
// transaction that files it. Bound, not created: undo archives the note
// through its row's own entry, and only once.
func (w *csvWriters) recordNoteTwinTx(ctx context.Context, tx pgx.Tx, twin string, note ids.UUID) error {
	if twin == "" {
		return nil
	}
	return w.identities.RecordIdentityTx(ctx, tx, w.runID, w.sourceSystem, noteTwinObject, twin, note, migration.OriginBound)
}

// predictedNoteTwin reports whether the dry run already predicted this note
// filed, and remembers it when not.
func (w *csvWriters) predictedNoteTwin(twin string) bool {
	if twin == "" {
		return false
	}
	if w.predictedNotes == nil {
		w.predictedNotes = map[string]bool{}
	}
	seen := w.predictedNotes[twin]
	w.predictedNotes[twin] = true
	return seen
}

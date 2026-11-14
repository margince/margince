// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// gateShape is one combination of the columns the content gate reads.
type gateShape struct {
	kind, audience string
	sourceSystem   *string
	// source is the row's source message: none, the other seat's private
	// mail, or the reader's own.
	source   int
	assigned bool
	linked   bool
	imported bool
	// restricted links the row to a contact under a processing restriction.
	restricted bool
}

// taskFree is the row the message gate is for: no task standing for a source.
func (s gateShape) taskFree() bool {
	if s.sourceSystem != nil {
		switch *s.sourceSystem {
		case provenance.EmailRequestSource, provenance.CommitmentTaskSource, provenance.LeadRepliedSource:
			return false
		}
	}
	return s.kind != "task" || s.source == noSource
}

func everyGateShape() []gateShape {
	systems := []*string{nil, new(provenance.EmailRequestSource), new(provenance.CommitmentTaskSource), new(provenance.LeadRepliedSource)}
	var shapes []gateShape
	for _, kind := range []string{"email", "task", "meeting"} {
		for _, audience := range []string{"workspace", "participants"} {
			for _, system := range systems {
				for _, source := range []int{noSource, theirSource, ownSource} {
					for _, assigned := range []bool{false, true} {
						if assigned && kind != "task" {
							continue
						}
						for _, linked := range []bool{false, true} {
							for _, imported := range []bool{false, true} {
								for _, restricted := range []bool{false, true} {
									shapes = append(shapes, gateShape{kind, audience, system, source, assigned, linked, imported, restricted})
								}
							}
						}
					}
				}
			}
		}
	}
	return shapes
}

const (
	noSource = iota
	theirSource
	ownSource
)

// seedGateShape writes one row: a link is to the other seat's private
// contact, an import is the rep's.
func (e *loadEnv) seedGateShape(t *testing.T, s gateShape, sources map[int]ids.UUID, private, restricted ids.UUID) ids.UUID {
	t.Helper()
	row := ids.NewV7()
	var sourceID, assignee *ids.UUID
	if source, ok := sources[s.source]; ok {
		sourceID = &source
	}
	if s.assigned {
		assignee = &e.rep
	}
	e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, audience, source_system, source_activity_id,
		assignee_id, source, captured_by) VALUES ($1, $2, 'inbound', 'shape', now(), $3, $4, $5, $6, 'seed', 'system')`,
		row, s.kind, s.audience, s.sourceSystem, sourceID, assignee)
	if s.linked {
		e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id) VALUES ($1, $2, 'contact', $3)`,
			ids.NewV7(), row, private)
	}
	if s.restricted {
		e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id) VALUES ($1, $2, 'contact', $3)`,
			ids.NewV7(), row, restricted)
	}
	if s.imported {
		e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, row, e.rep)
	}
	return row
}

// readerAt is the rep holding every activity grant at one row scope.
func (e *loadEnv) readerAt(scope principal.RowScope, kind principal.PrincipalType) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	return principal.WithActor(ctx, principal.Principal{
		Type: kind, ID: principal.HumanIDPrefix + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}, "deal": {Read: true}, "company": {Read: true}, "lead": {Read: true}},
			RowScope: scope,
		},
	})
}

type clauseRenderer func(context.Context, string, func(any) int) (string, error)

func (e *loadEnv) admitted(ctx context.Context, t *testing.T, gate clauseRenderer, rows []ids.UUID) map[ids.UUID]bool {
	t.Helper()
	got := map[ids.UUID]bool{}
	err := storeKnowing(e).db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{rows}
		clause, err := gate(ctx, "a", func(v any) int { args = append(args, v); return len(args) })
		if err != nil {
			return err
		}
		admitted, err := tx.Query(ctx, `SELECT a.id FROM activity a WHERE a.id = ANY($1) AND `+clause, args...)
		if err != nil {
			return err
		}
		ids, err := pgx.CollectRows(admitted, pgx.RowTo[ids.UUID])
		for _, id := range ids {
			got[id] = true
		}
		return err
	})
	if err != nil {
		t.Fatalf("reading what the gate admits: %v", err)
	}
	return got
}

// The message gate answers as the content gate on every task-free row and
// refuses every other, for each row scope and principal kind.
func TestTheMessageContentGateIsTheContentGateOnTaskFreeRows(t *testing.T) {
	e := setupLoad(t)
	private := e.privateContactOf(t, e.other)
	sources := map[int]ids.UUID{theirSource: ids.NewV7(), ownSource: ids.NewV7()}
	for source, seat := range map[int]ids.UUID{theirSource: e.other, ownSource: e.rep} {
		e.exec(t, `INSERT INTO activity (id, kind, direction, subject, occurred_at, audience, source, captured_by)
			VALUES ($1, 'email', 'inbound', 'a source mail', now(), 'participants', 'seed', 'system')`, sources[source])
		e.exec(t, `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`, sources[source], seat)
	}
	restricted := e.buyer(t)
	e.exec(t, `INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level)
		VALUES ($1, 'processing_restriction', 'test', 'human:test', 'subject')`, restricted)
	shapes := everyGateShape()
	rows := make([]ids.UUID, len(shapes))
	for i, shape := range shapes {
		rows[i] = e.seedGateShape(t, shape, sources, private, restricted)
	}
	readers := map[string]context.Context{
		"own human":  e.readerAt(principal.RowScopeOwn, principal.PrincipalHuman),
		"all human":  e.readerAt(principal.RowScopeAll, principal.PrincipalHuman),
		"own agent":  e.readerAt(principal.RowScopeOwn, principal.PrincipalAgent),
		"the system": e.readerAt(principal.RowScopeAll, principal.PrincipalSystem),
	}
	for name, ctx := range readers {
		content := e.admitted(ctx, t, auth.ActivityContentClause, rows)
		message := e.admitted(ctx, t, auth.ActivityMessageContentClause, rows)
		taskOnly, denied, restrictedSeen := 0, 0, 0
		for i, shape := range shapes {
			want := content[rows[i]] && shape.taskFree()
			if message[rows[i]] != want {
				t.Errorf("%s: shape %s: message gate admits %v, content gate %v", name, describe(shape), message[rows[i]], content[rows[i]])
			}
			if content[rows[i]] && !shape.taskFree() {
				taskOnly++
			}
			if !content[rows[i]] && shape.taskFree() {
				denied++
			}
			if message[rows[i]] && shape.restricted {
				restrictedSeen++
			}
		}
		if len(message) == 0 || taskOnly == 0 {
			t.Errorf("%s: %d message rows, %d task rows only the content gate admits: the corpus proves nothing", name, len(message), taskOnly)
		}
		if name == "own human" && denied == 0 {
			t.Errorf("%s: the content gate denied no task-free row, so the corpus cannot tell the gates apart", name)
		}
		// Mail about a restricted contact is withheld from an agent only.
		if agent := name == "own agent"; agent != (restrictedSeen == 0) {
			t.Errorf("%s: the message gate admits %d rows about a restricted contact", name, restrictedSeen)
		}
	}
}

func describe(s gateShape) string {
	system := "none"
	if s.sourceSystem != nil {
		system = *s.sourceSystem
	}
	return fmt.Sprintf("%s/%s source_system=%s source=%d assigned=%v linked=%v imported=%v restricted=%v",
		s.kind, s.audience, system, s.source, s.assigned, s.linked, s.imported, s.restricted)
}

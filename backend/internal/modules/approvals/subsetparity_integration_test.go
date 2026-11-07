// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// parityWorld is one workspace holding a row of every shape a row probe tells
// apart. Rows are owned by the reader, a teammate or an outsider. Some are
// owner-private, shared for read or write, archived, unowned or missing.
type parityWorld struct {
	e                     *stagingEnv
	team                  ids.UUID
	teammate, outsider    ids.UUID
	rows                  map[string][]ids.UUID
	caseMember            ids.UUID
	promotedLead, newLead ids.UUID
	repOffer, otherOffer  ids.UUID
}

func (w *parityWorld) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := w.e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("seeding: %v\n%s", err, sql)
	}
}

func (w *parityWorld) insert(t *testing.T, table string, sql string, args ...any) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	w.exec(t, sql, append([]any{id}, args...)...)
	w.rows[table] = append(w.rows[table], id)
	return id
}

// grant shares a record with the reader, or with the reader's team.
func (w *parityWorld) grant(t *testing.T, table string, id, subject ids.UUID, subjectType, access string) {
	t.Helper()
	w.exec(t, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
		VALUES ($1, $2, $3, $4, $5, $6)`, table, id, subjectType, subject, access, w.outsider)
}

func (w *parityWorld) archive(t *testing.T, table string, id ids.UUID) {
	t.Helper()
	w.exec(t, fmt.Sprintf(`UPDATE %s SET archived_at = now() WHERE id = $1`, table), id)
}

func newParityWorld(t *testing.T) *parityWorld {
	t.Helper()
	e := setupStaging(t)
	w := &parityWorld{e: e, team: ids.NewV7(), teammate: e.secondRep(t), outsider: e.secondRep(t), rows: map[string][]ids.UUID{}}
	w.exec(t, `INSERT INTO team (id, name) VALUES ($1, 'Parity')`, w.team)
	w.exec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2), ($1, $3)`, w.team, e.rep, w.teammate)
	w.seedRecords(t, newDealSeeder(t, e))
	w.seedInherited(t)
	return w
}

func (w *parityWorld) seedRecords(t *testing.T, seeder *dealSeeder) {
	t.Helper()
	const contact = `INSERT INTO contact (id, full_name, source, captured_by, owner_id, visibility) VALUES ($1, 'P', 'manual', 'system', $2, $3)`
	const company = `INSERT INTO company (id, display_name, source, captured_by, owner_id, visibility) VALUES ($1, 'P', 'manual', 'system', $2, $3)`
	const lead = `INSERT INTO lead (id, source, captured_by, owner_id) VALUES ($1, 'manual', 'system', $2)`
	rep := w.e.rep
	for _, owner := range []ids.UUID{rep, w.teammate, w.outsider} {
		for _, visibility := range []string{"workspace", "owner"} {
			w.insert(t, tableContact, contact, owner, visibility)
			w.insert(t, tableCompany, company, owner, visibility)
		}
		w.insert(t, tableLead, lead, owner)
		w.rows[tableDeal] = append(w.rows[tableDeal], seeder.dealOwnedBy(t, &owner))
	}
	w.rows[tableDeal] = append(w.rows[tableDeal], seeder.dealOwnedBy(t, nil))
	for _, access := range []string{"read", "write"} {
		w.grant(t, tableContact, w.insert(t, tableContact, contact, w.outsider, "owner"), rep, "user", access)
		w.grant(t, tableCompany, w.insert(t, tableCompany, company, w.outsider, "owner"), w.team, "team", access)
		w.grant(t, tableLead, w.insert(t, tableLead, lead, w.outsider), rep, "user", access)
		shared := seeder.dealOwnedBy(t, &w.outsider)
		w.rows[tableDeal] = append(w.rows[tableDeal], shared)
		w.grant(t, tableDeal, shared, w.team, "team", access)
	}
	w.caseMember = w.insert(t, tableContact, contact, w.outsider, "workspace")
	w.archive(t, tableContact, w.insert(t, tableContact, contact, rep, "workspace"))
	w.archive(t, tableCompany, w.insert(t, tableCompany, company, rep, "workspace"))
	w.archive(t, tableLead, w.insert(t, tableLead, lead, rep))
	archived := seeder.dealOwnedBy(t, &rep)
	w.archive(t, tableDeal, archived)
	w.rows[tableDeal] = append(w.rows[tableDeal], archived)
	for _, table := range []string{tableContact, tableCompany, tableLead, tableDeal} {
		w.rows[table] = append(w.rows[table], ids.NewV7(), w.rows[table][0])
	}
	w.newLead = w.rows[tableLead][0]
	w.promotedLead = ids.NewV7()
	w.exec(t, `INSERT INTO lead (id, source, captured_by, owner_id, status, archived_at, promoted_contact_id)
		VALUES ($1, 'manual', 'system', $2, 'promoted', now(), $3)`, w.promotedLead, rep, w.rows[tableContact][1])
	w.rows[tableLead] = append(w.rows[tableLead], w.promotedLead)
}

// seedInherited writes the rows that take their scope from another: signals
// from their subject, edges from every endpoint, offers from their deal.
func (w *parityWorld) seedInherited(t *testing.T) {
	t.Helper()
	const signal = `INSERT INTO signal (id, kind, summary, source, captured_by, owner_id, visibility, entity_type, entity_id, resolution_state)
		VALUES ($1, 'other', 'P', 'manual', 'system', $2, $3, $4::text, $5::uuid,
		        CASE WHEN $4::text IS NULL THEN 'unresolved' ELSE 'resolved' END)`
	contacts, deals := w.rows[tableContact], w.rows[tableDeal]
	w.insert(t, targetSignal, signal, w.e.rep, "workspace", nil, nil)
	w.insert(t, targetSignal, signal, w.teammate, "owner", nil, nil)
	w.insert(t, targetSignal, signal, w.e.rep, "workspace", tableDeal, deals[0])
	w.insert(t, targetSignal, signal, w.outsider, "workspace", tableDeal, deals[2])
	w.insert(t, targetSignal, signal, w.outsider, "workspace", tableContact, contacts[5])
	w.archive(t, targetSignal, w.insert(t, targetSignal, signal, w.e.rep, "workspace", nil, nil))
	const edge = `INSERT INTO relationship (id, kind, source, captured_by, contact_id, company_id) VALUES ($1, 'employment', 'manual', 'system', $2, $3)`
	companies := w.rows[tableCompany]
	w.insert(t, targetRelationship, edge, contacts[1], companies[1])
	w.insert(t, targetRelationship, edge, contacts[3], companies[3])
	w.insert(t, targetRelationship, edge, contacts[5], companies[5])
	w.archive(t, targetRelationship, w.insert(t, targetRelationship, edge, contacts[0], companies[0]))
	const offer = `INSERT INTO offer (id, deal_id, offer_number, currency, source, captured_by) VALUES ($1, $2, $3, 'EUR', 'manual', 'system')`
	w.repOffer = w.insert(t, targetOffer, offer, deals[0], "A-1")
	w.otherOffer = w.insert(t, targetOffer, offer, deals[2], "A-2")
	for _, audience := range []string{"workspace", "selected", "workspace"} {
		w.rows[objectActivity] = append(w.rows[objectActivity], w.e.seedNote(t, audience))
	}
	w.archive(t, objectActivity, w.rows[objectActivity][0])
	for _, table := range []string{targetSignal, targetRelationship, objectActivity} {
		w.rows[table] = append(w.rows[table], ids.NewV7(), w.rows[table][1])
	}
}

// readers are the principals whose row rules differ. They are each scope tier,
// a read seat, a contact case admission, an agent and a system job.
func (w *parityWorld) readers(t *testing.T) map[string]context.Context {
	t.Helper()
	full := principal.ObjectGrant{Create: true, Read: true, Update: true, Delete: true}
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{tableContact, tableCompany, tableDeal, tableLead, targetSignal, objectActivity, targetRelationship, targetOffer} {
		grants[object] = full
	}
	as := func(p principal.Principal) context.Context {
		p.TeamIDs, p.Permissions.Objects = []ids.UUID{w.team}, grants
		ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), w.e.ws), ids.NewV7())
		return principal.WithActor(ctx, p)
	}
	human := func(scope principal.RowScope, seat principal.SeatType) principal.Principal {
		return principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + w.e.rep.String(), UserID: w.e.rep,
			SeatType: seat, Permissions: principal.Permissions{RowScope: scope}}
	}
	admitted, err := auth.WithContactCaseAdmission(as(human(principal.RowScopeOwn, principal.SeatFull)), []ids.UUID{w.caseMember, w.rows[tableContact][5]}, []ids.UUID{w.outsider})
	if err != nil {
		t.Fatal(err)
	}
	agent := human(principal.RowScopeOwn, principal.SeatFull)
	agent.Type, agent.ID, agent.OnBehalfOf = principal.PrincipalAgent, "agent:parity", w.e.rep
	return map[string]context.Context{
		"own scope":            as(human(principal.RowScopeOwn, principal.SeatFull)),
		"team scope":           as(human(principal.RowScopeTeam, principal.SeatFull)),
		"all scope":            as(human(principal.RowScopeAll, principal.SeatFull)),
		"read seat":            as(human(principal.RowScopeTeam, principal.SeatRead)),
		"case-member admitted": admitted,
		"agent":                as(agent),
		"system":               as(principal.Principal{Type: principal.PrincipalSystem, ID: "system:parity"}),
	}
}

// Every subset probe must answer what its single-row twin answers, for every
// reader over every row shape, duplicates and missing ids included.
func TestTheSubsetProbesAnswerAsTheSingleRowProbes(t *testing.T) {
	w := newParityWorld(t)
	rules := map[auth.RowRule]func(context.Context, pgx.Tx, string, ids.UUID) error{
		auth.RuleVisibleLive: auth.EnsureVisibleLive, auth.RuleWritableLive: auth.EnsureWritableLive, auth.RuleWritable: auth.EnsureWritable,
	}
	for name, reader := range w.readers(t) {
		err := w.e.svc.db.Tx(reader, func(tx pgx.Tx) error {
			for _, table := range []string{tableContact, tableCompany, tableDeal, tableLead} {
				for rule, single := range rules {
					subset, err := auth.PassingSubset(reader, tx, table, w.rows[table], rule)
					if err := agreeRowByRow(w.rows[table], subset, err, func(id ids.UUID) error { return single(reader, tx, table, id) }); err != nil {
						return fmt.Errorf("%s, rule %d: %w", table, rule, err)
					}
				}
			}
			notes := w.rows[objectActivity]
			for live, single := range map[bool]func(context.Context, pgx.Tx, ids.UUID) error{
				false: auth.EnsureActivityContentVisible, true: auth.EnsureActivityContentVisibleLive,
			} {
				subset, err := auth.ActivityContentSubset(reader, tx, notes, live)
				if err := agreeRowByRow(notes, subset, err, func(id ids.UUID) error { return single(reader, tx, id) }); err != nil {
					return fmt.Errorf("activity content live=%t: %w", live, err)
				}
			}
			signals, edges := w.rows[targetSignal], w.rows[targetRelationship]
			subset, err := auth.SignalVisibleLiveSubset(reader, tx, signals)
			if err := agreeRowByRow(signals, subset, err, func(id ids.UUID) error { return auth.EnsureSignalVisibleLive(reader, tx, id) }); err != nil {
				return fmt.Errorf("signals: %w", err)
			}
			subset, err = auth.RelationshipVisibleSubset(reader, tx, edges)
			if err := agreeRowByRow(edges, subset, err, func(id ids.UUID) error { return auth.EnsureRelationshipVisible(reader, tx, id) }); err != nil {
				return fmt.Errorf("relationships: %w", err)
			}
			return nil
		})
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func agreeRowByRow(rowIDs []ids.UUID, subset map[ids.UUID]bool, subsetErr error, single func(ids.UUID) error) error {
	if subsetErr != nil {
		return subsetErr
	}
	if len(subset) == 0 || len(subset) == len(rowIDs) {
		return fmt.Errorf("%d of %d rows passed; a probe that admits all or none proves nothing", len(subset), len(rowIDs))
	}
	for i, id := range rowIDs {
		alone, err := probeAnswer(single(id))
		if err != nil {
			return err
		}
		if alone != subset[id] {
			return fmt.Errorf("row %d answered %t alone and %t in the subset", i, alone, subset[id])
		}
	}
	return nil
}

// A batch mixes target types, a repeated target, an unknown type, offers and
// lead reversals. Each row answers as it answers alone, and as the rule says.
func TestAMixedBatchAnswersEachRowAsItAnswersAlone(t *testing.T) {
	w := newParityWorld(t)
	target := func(kind, targetType string, id ids.UUID) row {
		return row{ID: ids.From[ids.ApprovalKind](ids.NewV7()), Kind: kind, TargetType: &targetType, TargetID: &id}
	}
	deals, contacts := w.rows[tableDeal], w.rows[tableContact]
	batch := []row{
		target("advance_deal", tableDeal, deals[0]),
		target("advance_deal", tableDeal, deals[2]),
		target("advance_deal", tableDeal, deals[0]),
		target("update_record", tableContact, contacts[1]),
		target("update_record", tableContact, contacts[3]),
		target("update_record", tableContact, contacts[2]),
		target("update_record", "chartreuse", ids.NewV7()),
		target("update_record", targetOffer, w.repOffer),
		target("update_record", targetOffer, w.otherOffer),
		target("demote_lead", tableLead, w.promotedLead),
		target("demote_lead", tableLead, w.newLead),
	}
	// A teammate's owner-private contact is refused to the team scope by capture
	// privacy alone, which the write arm would admit.
	want := map[string][]bool{
		"own scope":  {true, false, true, true, false, false, false, true, false, true, false},
		"team scope": {true, false, true, true, false, true, false, true, false, true, false},
	}
	for name, reader := range w.readers(t) {
		p, _ := principal.Actor(reader)
		err := w.e.svc.db.Tx(reader, func(tx pgx.Tx) error {
			together, err := decidableRows(reader, tx, p, batch)
			if err != nil {
				return err
			}
			for i, a := range batch {
				alone, err := decidable(reader, tx, p, a)
				if err != nil {
					return err
				}
				if alone != together[i] {
					return fmt.Errorf("row %d answered %t alone and %t in the batch", i, alone, together[i])
				}
			}
			for i, expected := range want[name] {
				if together[i] != expected {
					return fmt.Errorf("row %d (%s on %s) answered %t, want %t", i, batch[i].Kind, *batch[i].TargetType, together[i], expected)
				}
			}
			return nil
		})
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

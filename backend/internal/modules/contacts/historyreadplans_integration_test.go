// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type historyPlanTrace struct {
	needle, query string
	args          []any
}

func (r *historyPlanTrace) record(query string, args []any) {
	if r.needle != "" && strings.Contains(query, r.needle) {
		r.query = query
		r.args = append([]any(nil), args...)
	}
}

type historyPlanTx struct {
	pgx.Tx
	trace *historyPlanTrace
}

func (tx historyPlanTx) Query(ctx context.Context, query string, args ...any) (pgx.Rows, error) {
	tx.trace.record(query, args)
	return tx.Tx.Query(ctx, query, append([]any{pgx.QueryExecModeCacheStatement}, args...)...)
}

func (tx historyPlanTx) QueryRow(ctx context.Context, query string, args ...any) pgx.Row {
	tx.trace.record(query, args)
	return tx.Tx.QueryRow(ctx, query, append([]any{pgx.QueryExecModeCacheStatement}, args...)...)
}

func (n mailboxPlanNode) boundsHistoryRows(limit int) bool {
	if (n.ActualRows+n.FilteredRows+n.RecheckedRows)*n.ActualLoops > limit {
		return false
	}
	for _, child := range n.Plans {
		if !child.boundsHistoryRows(limit) {
			return false
		}
	}
	return true
}

func TestHistoryReadersSeekPastACompleteUnrelatedMailbox(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	connected := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	e.connectMailbox(ctx, t, connected)
	e.sentBy(ctx, t, e.rep, "hot@bounded-history.test", connected.Add(-time.Hour))
	wanted := e.sentBy(ctx, t, e.rep, "wanted@bounded-history.test", connected.Add(-time.Hour))
	e.copiedOn(ctx, t, "received@bounded-history.test", connected.Add(-2*time.Hour), connected.Add(-time.Hour), false)
	e.plainContact(ctx, t, "Related Variant", "variant+red@bounded-history.test")
	legacy := e.plainContact(ctx, t, "Legacy Subject", "legacy@bounded-history.test")
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO contact_acquisition_evidence(contact_id,kind,captured_by) VALUES(@contact,'unknown_legacy','connector:gmail')`, pgx.NamedArgs{"contact": legacy})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	e.seedHistoryReadBackground(ctx, t, connected)
	analyzeAsOwner(t, "activity", "activity_participant", "capture_import", "contact", "contact_email")
	recorder := &historyPlanTrace{}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		for _, kind := range []string{"sent-present", "sent-absent", "received-absent", "received-present", "acquisition", "variant", "subject-present", "subject-absent", "legacy", "sent-hot", "sent-hot-absent"} {
			t.Run(mode+"/"+kind, func(t *testing.T) {
				if err := e.store.tx(ctx, func(nativeTx pgx.Tx) error {
					tx := historyPlanTx{Tx: nativeTx, trace: recorder}
					if _, err := tx.Exec(ctx, `SELECT set_config('plan_cache_mode',@mode,true)`, pgx.NamedArgs{"mode": mode}); err != nil {
						return err
					}
					recorder.query = ""
					recorder.args = nil
					switch kind {
					case "sent-present", "sent-absent", "sent-hot", "sent-hot-absent":
						recorder.needle = "SELECT " + sentHistorySQL
						address := "wanted@bounded-history.test"
						if kind == "sent-absent" {
							address = "absent@bounded-history.test"
						}
						if kind == "sent-hot" {
							address = "hot@bounded-history.test"
						}
						if kind == "sent-hot-absent" {
							address = "hot-absent@bounded-history.test"
						}
						held, err := writtenToBeforeConnectedTx(ctx, tx, address)
						if err != nil {
							return err
						}
						if held != (kind == "sent-present" || kind == "sent-hot") {
							return fmt.Errorf("sent history=%v", held)
						}
					case "received-absent", "received-present":
						recorder.needle = "SELECT " + receivedHistorySQL
						address := "absent@bounded-history.test"
						if kind == "received-present" {
							address = "received@bounded-history.test"
						}
						held, err := receivedBeforeConnectedTx(ctx, tx, address)
						if err != nil {
							return err
						}
						if held != (kind == "received-present") {
							return fmt.Errorf("received history=%v", held)
						}
					case "acquisition":
						recorder.needle = "SELECT min(a.occurred_at)"
						at := acquisitionTimeFor(ctx, tx, "wanted@bounded-history.test", wanted.ActivityID)
						if at == nil || !at.Equal(connected.Add(-time.Hour)) {
							return fmt.Errorf("acquisition time=%v", at)
						}
					case "subject-present", "subject-absent":
						recorder.needle = "WITH senders AS MATERIALIZED"
						address, want := "sender@elsewhere.test", AcquiredSubjectInitiated
						if kind == "subject-absent" {
							address, want = "absent@bounded-history.test", AcquiredUnknownLegacy
						}
						acquired, err := acquiredFromCaptureTx(ctx, tx, false, address)
						if err != nil {
							return err
						}
						if acquired != want {
							return fmt.Errorf("acquisition=%s want=%s", acquired, want)
						}
					case "legacy":
						recorder.needle = "UPDATE contact_acquisition_evidence e SET kind"
						system := principal.SystemActing(ctx, "system:history-plan")
						changed, _, err := RelabelHeldMailTx(system, tx, datatask.BatchIn{Limit: 100})
						if err != nil {
							return err
						}
						if len(changed) != 0 {
							return errors.New("narrow correction changed unrelated acquisitions")
						}
					case "variant":
						recorder.needle = "SELECT e.contact_id, array_agg(e.email)"
						matches, err := addressVariantContacts(ctx, tx, ContactCandidate{Emails: []string{"variant+blue@bounded-history.test"}})
						if err != nil {
							return err
						}
						if len(matches) != 1 {
							return fmt.Errorf("variant matches=%d", len(matches))
						}
					}
					recorder.needle = ""
					raw, err := testdb.ExplainCachedStatement(ctx, tx, recorder.query, recorder.args)
					if err != nil {
						return err
					}
					var plans []struct{ Plan mailboxPlanNode }
					if err := json.Unmarshal(raw, &plans); err != nil {
						return err
					}
					rowLimit, bufferLimit := 64, 256
					if strings.HasPrefix(kind, "sent-hot") {
						rowLimit = 1064
					}
					if kind == "sent-hot-absent" {
						bufferLimit = 24000
					}
					if len(plans) != 1 || !plans[0].Plan.boundsHistoryRows(rowLimit) || plans[0].Plan.SharedHits+plans[0].Plan.SharedReads > bufferLimit {
						return fmt.Errorf("history reader visits unrelated rows: %s", raw)
					}
					t.Logf("shared_buffers=%d plan=%s", plans[0].Plan.SharedHits+plans[0].Plan.SharedReads, raw)
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func (e *dedupeEnv) seedHistoryReadBackground(ctx context.Context, t *testing.T, connected time.Time) {
	t.Helper()
	// Participant evidence locks make one unbounded fixture transaction unlike a real import.
	for after := 0; after < 100000; after += 1000 {
		if err := e.store.tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `WITH background AS (
   INSERT INTO activity(kind,direction,source,captured_by,counterparty_email,counterparty_outbound_attested,occurred_at)
   SELECT 'email',CASE WHEN n%2=0 THEN 'outbound' ELSE 'inbound' END,'gmail','connector:gmail',
     'other-'||n||'@unrelated.test',n%2=0,@at::timestamptz FROM generate_series(@after::int+1,@through::int) n
   RETURNING id,direction,counterparty_email),
   imported AS(INSERT INTO capture_import(activity_id,user_id,provider_received_at) SELECT id,@user,@at FROM background)
   INSERT INTO activity_participant(activity_id,role,user_id,address)
   SELECT id,'from',CASE WHEN direction='outbound' THEN @user::uuid END,
     CASE WHEN direction='inbound' THEN counterparty_email ELSE 'owner@unrelated.test' END FROM background
   UNION ALL SELECT id,'to',CASE WHEN direction='inbound' THEN @user::uuid END,
     CASE WHEN direction='outbound' THEN counterparty_email ELSE 'owner@unrelated.test' END FROM background
   UNION ALL SELECT id,'cc',NULL::uuid,'copy-'||id||'@unrelated.test' FROM background`, pgx.NamedArgs{"at": connected.Add(-24 * time.Hour), "user": e.rep, "after": after, "through": after + 1000})
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `WITH background AS(
   INSERT INTO contact(full_name,source,captured_by) SELECT 'Unrelated '||n,'manual','test:bounded-history' FROM generate_series(1,100000) n RETURNING id)
   INSERT INTO contact_email(contact_id,email,source,captured_by) SELECT id,id::text||'@unrelated.test','manual','test:bounded-history' FROM background`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `WITH hot AS(
          INSERT INTO activity(kind,direction,source,captured_by,counterparty_email,counterparty_outbound_attested,occurred_at)
          SELECT 'email','outbound','gmail','connector:gmail',
           CASE WHEN n%2=0 THEN 'hot@bounded-history.test' ELSE 'hot-absent@bounded-history.test' END,true,@late::timestamptz
          FROM generate_series(1,2000) n RETURNING id),
          imported AS(INSERT INTO capture_import(activity_id,user_id) SELECT id,@user FROM hot)
          INSERT INTO activity_participant(activity_id,role,user_id) SELECT id,'from',@user FROM hot`, pgx.NamedArgs{"late": connected.Add(time.Hour), "user": e.rep})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

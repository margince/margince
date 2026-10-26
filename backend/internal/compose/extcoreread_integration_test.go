// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The extension core's read verbs against a real database, through the served
// entry point and on a pool of one connection. A verb that took a second
// connection would wait forever on the one its own transaction holds.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// readWorld is what the cases read. It holds two deals and two companies, one
// of each owned by a colleague, plus a signal, an activity and a custom field.
type readWorld struct {
	coreEnv
	colleague                  ids.UUID
	mineDeal, theirsDeal       ids.UUID
	mineCompany, theirsCompany ids.UUID
	signal, activity           ids.UUID
}

func setupReads(t *testing.T) readWorld {
	t.Helper()
	w := readWorld{
		coreEnv:       setupCore(t),
		colleague:     ids.NewV7(),
		mineDeal:      ids.NewV7(),
		theirsDeal:    ids.NewV7(),
		mineCompany:   ids.NewV7(),
		theirsCompany: ids.NewV7(),
		signal:        ids.NewV7(),
		activity:      ids.NewV7(),
	}
	ctx := context.Background()
	pipeline, stage := ids.NewV7(), ids.NewV7()
	seed := func(sql string, args ...any) {
		t.Helper()
		if _, err := w.owner.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("seeding the read fixture: %v\n%s", err, sql)
		}
	}
	seed(`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Colleague')`,
		w.colleague, "colleague-"+w.colleague.String()+"@ext.test")
	seed(`INSERT INTO pipeline (id, name) VALUES ($1, 'Reads')`, pipeline)
	seed(`INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		  VALUES ($1, $2, 'Open', 1, 'open', 50)`, stage, pipeline)
	seed(`INSERT INTO company (id, display_name, owner_id, source_system, source_id, source, captured_by)
		  VALUES ($1, 'Mine Co', $2, 'erp', 'C-1', 'manual', 'human:x')`, w.mineCompany, w.user)
	seed(`INSERT INTO company (id, display_name, owner_id, visibility, source, captured_by)
		  VALUES ($1, 'Theirs Co', $2, 'owner', 'manual', 'human:x')`, w.theirsCompany, w.colleague)
	seed(`ALTER TABLE deal ADD COLUMN IF NOT EXISTS cf_region text`)
	seed(`INSERT INTO custom_field (object, slug, label, type, column_name, created_by)
		  VALUES ('deal', 'region', 'Region', 'text', 'cf_region', $1)`, w.user)
	seed(`INSERT INTO deal (id, pipeline_id, stage_id, name, owner_id, status, source_system, source_id, cf_region, source, captured_by)
		  VALUES ($1, $2, $3, 'Mine Deal', $4, 'open', 'erp', 'D-1', 'emea', 'manual', 'human:x')`,
		w.mineDeal, pipeline, stage, w.user)
	seed(`INSERT INTO deal (id, pipeline_id, stage_id, name, owner_id, status, cf_region, source, captured_by)
		  VALUES ($1, $2, $3, 'Theirs Deal', $4, 'open', 'apac', 'manual', 'human:x')`,
		w.theirsDeal, pipeline, stage, w.colleague)
	seed(`INSERT INTO signal (id, kind, source_channel, entity_type, entity_id, resolved_company_id,
		                      resolution_state, severity, summary, status, detected_at, source, captured_by)
		  VALUES ($1, 'contract_ended', 'derived', 'company', $2, $2, 'resolved', 'warn', 'ended', 'open',
		          now(), 'signal-scan', 'agent:x')`, w.signal, w.mineCompany)
	seed(`INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		  VALUES ($1, 'note', 'a note', now(), 'manual', 'human:x')`, w.activity)
	seed(`INSERT INTO activity_link (activity_id, entity_type, company_id) VALUES ($1, 'company', $2)`,
		w.activity, w.mineCompany)
	return w
}

func readerPerms(scope principal.RowScope, objects ...string) principal.Permissions {
	grants := map[string]principal.ObjectGrant{}
	for _, o := range objects {
		grants[o] = principal.ObjectGrant{Read: true}
	}
	return principal.Permissions{RoleKeys: []string{"reader"}, Objects: grants, RowScope: scope}
}

var allReadable = []string{"deal", "company", "signal", "activity", "custom_field"}

// readAs runs fn on the port a call arrives with when the caller is user.
func (w readWorld) readAs(t *testing.T, user ids.UUID, perms principal.Permissions, fn func(context.Context, extension.Core) error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(w.invocation(), coreBudget)
	defer cancel()
	actor := principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + user.String(),
		UserID:      user,
		Permissions: perms,
	}
	ctx = principal.WithActor(ctx, actor)
	rt := runtimeFor(ctx, coreUnit, coreVersion, coreVia, extensionRuntimeBinding{pool: w.pool})
	return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error { return fn(ctx, tx.Core()) })
}

func TestAUnitReadsEachCoreRecordTypeThroughItsCallersAuthority(t *testing.T) {
	w := setupReads(t)
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, allReadable...), func(ctx context.Context, core extension.Core) error {
		deal, err := core.Deals().Get(ctx, w.mineDeal.String())
		if err != nil || deal.Name != "Mine Deal" {
			t.Errorf("Deals().Get = %q, %v; want the deal", deal.Name, err)
		}
		if deal.AdditionalProperties["cf_region"] != "emea" {
			t.Errorf("the deal's custom field came back as %v, want emea", deal.AdditionalProperties["cf_region"])
		}
		company, err := core.Companies().Get(ctx, w.mineCompany.String())
		if err != nil || company.DisplayName != "Mine Co" {
			t.Errorf("Companies().Get = %q, %v; want the company", company.DisplayName, err)
		}
		if signal, err := core.Signals().Get(ctx, w.signal.String()); err != nil || signal.Kind == "" {
			t.Errorf("Signals().Get = %+v, %v; want the signal", signal, err)
		}
		if act, err := core.Activities().Get(ctx, w.activity.String()); err != nil || string(act.Id) != w.activity.String() {
			t.Errorf("Activities().Get = %v, %v; want the activity", act.Id, err)
		}
		fields, err := core.CustomFields().List(ctx, "deal")
		if err != nil || len(fields) != 1 || fields[0].Name != "cf_region" || fields[0].Type != "text" {
			t.Errorf("CustomFields().List = %+v, %v; want cf_region", fields, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAListFiltersByCustomFieldOutsideIdKindAndRecordLink(t *testing.T) {
	w := setupReads(t)
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, allReadable...), func(ctx context.Context, core extension.Core) error {
		byField, err := core.Deals().List(ctx, extension.DealQuery{CustomFields: map[string]string{"cf_region": "apac"}})
		if err != nil || len(byField.Items) != 1 || byField.Items[0].Name != "Theirs Deal" {
			t.Errorf("deals by cf_region = %d, %v; want the apac deal", len(byField.Items), err)
		}
		byID, err := core.Deals().List(ctx, extension.DealQuery{SourceSystem: "erp", SourceID: "D-1"})
		if err != nil || len(byID.Items) != 1 || byID.Items[0].Name != "Mine Deal" {
			t.Errorf("deals by outside id = %d, %v; want Mine Deal", len(byID.Items), err)
		}
		companies, err := core.Companies().List(ctx, extension.CompanyQuery{SourceSystem: "erp", SourceID: "C-1"})
		if err != nil || len(companies.Items) != 1 {
			t.Errorf("companies by outside id = %d, %v; want one", len(companies.Items), err)
		}
		sigs, err := core.Signals().List(ctx, extension.SignalQuery{Kind: "contract_ended", CompanyID: w.mineCompany.String()})
		if err != nil || len(sigs.Items) != 1 {
			t.Errorf("signals by kind and company = %d, %v; want one", len(sigs.Items), err)
		}
		acts, err := core.Activities().List(ctx, extension.ActivityQuery{Kind: "note", EntityType: "company", EntityID: w.mineCompany.String()})
		if err != nil || len(acts.Items) != 1 {
			t.Errorf("activities by kind and link = %d, %v; want one", len(acts.Items), err)
		}
		if _, err := core.Deals().List(ctx, extension.DealQuery{CustomFields: map[string]string{"cf_nope": "x"}}); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("a filter on an unknown custom field answered %v, want ErrInvalid", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Deals and companies are shared across the workspace. What a caller cannot see
// is what the product hides: a record a colleague's mailbox captured and has
// not promoted.
func TestARecordTheCallerCannotSeeReadsAsNotFoundAndIsLeftOutOfLists(t *testing.T) {
	w := setupReads(t)
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeOwn, allReadable...), func(ctx context.Context, core extension.Core) error {
		if _, err := core.Companies().Get(ctx, w.theirsCompany.String()); !errors.Is(err, extension.ErrNotFound) {
			t.Errorf("a colleague's private company answered %v, want ErrNotFound", err)
		}
		page, err := core.Companies().List(ctx, extension.CompanyQuery{})
		if err != nil || len(page.Items) != 1 || page.Items[0].DisplayName != "Mine Co" {
			t.Errorf("the company list holds %d companies (%v), want only the visible one", len(page.Items), err)
		}
		if deal, err := core.Deals().Get(ctx, w.theirsDeal.String()); err != nil || deal.Name != "Theirs Deal" {
			t.Errorf("a colleague's deal answered %v, want it read: deals are shared across the workspace", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestACallerWithoutTheObjectGrantIsForbidden(t *testing.T) {
	w := setupReads(t)
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, "company"), func(ctx context.Context, core extension.Core) error {
		if _, err := core.Deals().List(ctx, extension.DealQuery{}); !errors.Is(err, extension.ErrForbidden) {
			t.Errorf("a deal list without deal:read answered %v, want ErrForbidden", err)
		}
		if _, err := core.Activities().List(ctx, extension.ActivityQuery{}); !errors.Is(err, extension.ErrForbidden) {
			t.Errorf("an activity list without activity:read answered %v, want ErrForbidden", err)
		}
		if _, err := core.Signals().List(ctx, extension.SignalQuery{}); !errors.Is(err, extension.ErrForbidden) {
			t.Errorf("a signal list without signal:read answered %v, want ErrForbidden", err)
		}
		if _, err := core.CustomFields().List(ctx, "deal"); !errors.Is(err, extension.ErrForbidden) {
			t.Errorf("the field list without custom_field:read answered %v, want ErrForbidden", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A unit that lists and then fails has still read the records, so its export
// entry must survive the rollback it chose.
func TestAnExportEntrySurvivesTheUnitRollingBack(t *testing.T) {
	w := setupReads(t)
	unitFailed := errors.New("the unit's own row could not be written")
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, allReadable...), func(ctx context.Context, core extension.Core) error {
		if _, err := core.Companies().List(ctx, extension.CompanyQuery{}); err != nil {
			return err
		}
		return unitFailed
	})
	if !errors.Is(err, unitFailed) {
		t.Fatalf("the transaction answered %v, want the unit's own failure", err)
	}
	var rows int
	if err := w.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM system_log WHERE action = 'export' AND detail->>'table' = 'company'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("%d export entries after a rolled-back list, want 1: the unit read personal data and erased the trace", rows)
	}
}

// A unit that panics after a list has read the records too.
func TestAnExportEntrySurvivesTheUnitPanicking(t *testing.T) {
	w := setupReads(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("the panic did not reach the caller, want it re-raised after the entry is written")
			}
		}()
		err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, allReadable...), func(ctx context.Context, core extension.Core) error {
			if _, err := core.Companies().List(ctx, extension.CompanyQuery{}); err != nil {
				return err
			}
			panic("the unit failed")
		})
		t.Errorf("the transaction returned %v, want the panic to propagate", err)
	}()
	var rows int
	if err := w.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM system_log WHERE action = 'export' AND detail->>'table' = 'company'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("%d export entries after a panicking unit, want 1", rows)
	}
}

// A cancelled call must not drop the entry either.
func TestAnExportEntrySurvivesTheCallBeingCancelled(t *testing.T) {
	w := setupReads(t)
	ctx, cancel := context.WithTimeout(w.invocation(), coreBudget)
	defer cancel()
	ctx = principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + w.user.String(),
		UserID:      w.user,
		Permissions: readerPerms(principal.RowScopeAll, allReadable...),
	})
	rt := runtimeFor(ctx, coreUnit, coreVersion, coreVia, extensionRuntimeBinding{pool: w.pool})
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		if _, err := tx.Core().Companies().List(ctx, extension.CompanyQuery{}); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if err == nil {
		t.Fatal("a transaction committed on a cancelled context, want an error")
	}
	var rows int
	if err := w.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM system_log WHERE action = 'export' AND detail->>'table' = 'company'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("%d export entries after a cancelled call, want 1", rows)
	}
}

func TestABulkReadIsAuditedAsAnExportCarriedByTheUnit(t *testing.T) {
	w := setupReads(t)
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, allReadable...), func(ctx context.Context, core extension.Core) error {
		_, err := core.Deals().List(ctx, extension.DealQuery{Search: "Mine"})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var raw []byte
	if err := w.owner.QueryRow(context.Background(),
		`SELECT detail FROM system_log WHERE action = 'export'`).Scan(&raw); err != nil {
		t.Fatalf("reading the export row: %v", err)
	}
	var detail struct {
		Extension string   `json:"extension"`
		Table     string   `json:"table"`
		RowCount  int      `json:"row_count"`
		Filters   []string `json:"filters"`
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Extension != coreUnit || detail.Table != "deal" || detail.RowCount != 1 ||
		len(detail.Filters) != 1 || detail.Filters[0] != "search" {
		t.Errorf("the export row says %+v, want the unit, the table, one row and the filter name", detail)
	}
	if strings.Contains(string(raw), "Mine") {
		t.Errorf("the export row carries the search term: %s", raw)
	}
}

// A transaction the database has aborted answers every later statement with an
// error. A verb must turn that into a refusal class and not a panic or a
// success, whichever statement it was on.
func TestEveryReadVerbAndTheNoticeDoorRefuseOnAnAbortedTransaction(t *testing.T) {
	w := setupReads(t)
	company := w.mineCompany.String()
	err := w.readAs(t, w.user, readerPerms(principal.RowScopeAll, allReadable...), func(ctx context.Context, core extension.Core) error {
		c, ok := core.(extensionCore)
		if !ok {
			t.Fatalf("the port is a %T, want extensionCore", core)
		}
		if _, err := c.tx.Exec(ctx, `SELECT 1/0`); err == nil {
			t.Fatal("the probe statement did not fail")
		}
		_, dealGet := core.Deals().Get(ctx, w.mineDeal.String())
		_, dealList := core.Deals().List(ctx, extension.DealQuery{})
		_, companyGet := core.Companies().Get(ctx, company)
		_, companyList := core.Companies().List(ctx, extension.CompanyQuery{})
		_, signalGet := core.Signals().Get(ctx, w.signal.String())
		_, signalList := core.Signals().List(ctx, extension.SignalQuery{CompanyID: company})
		_, activityGet := core.Activities().Get(ctx, w.activity.String())
		_, activityList := core.Activities().List(ctx, extension.ActivityQuery{EntityType: "company", EntityID: company})
		_, fields := core.CustomFields().List(ctx, "deal")
		_, notice := core.Notices().Send(ctx, extension.NoticeRequest{
			Member: w.user.String(), Record: extension.RecordRef{Type: "deal", ID: w.mineDeal.String()}, Subject: "s",
		})
		for name, err := range map[string]error{
			"deal get": dealGet, "deal list": dealList, "company get": companyGet, "company list": companyList,
			"signal get": signalGet, "signal list": signalList, "activity get": activityGet,
			"activity list": activityList, "custom fields": fields, "notice": notice,
		} {
			if err == nil {
				t.Errorf("%s succeeded on an aborted transaction", name)
			}
		}
		return nil
	})
	_ = err // the transaction is aborted, so its own rollback is the unit's to report
}

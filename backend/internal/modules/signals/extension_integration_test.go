// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package signals

// An extension files a signal of a kind it declared. The signal is read back
// through the real reader. The repeat goes through the real writer. The claims
// are about what the unique index, the audit log and the evidence column hold.

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/signaltypes"
	"github.com/margince/margince/backend/pkg/extension"
)

type extensionSignalFixture struct {
	ctx     context.Context
	owner   *pgx.Conn
	store   *Store
	company ids.UUID
	actorID string
}

func newExtensionSignalFixture(t *testing.T) extensionSignalFixture {
	t.Helper()
	ownerDSN, appDSN := os.Getenv("MARGINCE_TEST_DSN"), os.Getenv("MARGINCE_TEST_APP_DSN")
	if ownerDSN == "" || appDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN / MARGINCE_TEST_APP_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}
	ws, admin, company := ids.NewV7(), ids.NewV7(), ids.NewV7()
	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES ($1)`, ws); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Admin')`,
		admin, "admin-"+admin.String()+"@signals.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx,
		`INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Acme', 'manual', 'human:test')`,
		company); err != nil {
		t.Fatal(err)
	}
	pool, err := testdb.Pool(ctx, appDSN)
	if err != nil {
		t.Fatal(err)
	}
	testdb.AssertPoolsQuiesced(t)

	signaltypes.Register("sigtest", []extension.SignalTypeDeclaration{{
		Kind: "loss", Label: "Loss reported",
		Evidence: []extension.SignalEvidenceField{
			{Name: "amount", Label: "Amount", Type: extension.SignalFieldNumber, Required: true},
			{Name: "reporter", Label: "Reporter", Type: extension.SignalFieldText, Personal: true},
		},
	}})
	actorID := "human:" + admin.String()
	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: actorID, UserID: admin,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects:  map[string]principal.ObjectGrant{"signal": {Create: true, Read: true, Update: true}, "company": {Read: true}},
		},
	})
	return extensionSignalFixture{
		ctx: ctx, owner: owner, company: company, actorID: actorID,
		store: NewStore(database.BindTo(pool, ids.From[ids.WorkspaceKind](ws)), nil),
	}
}

func (f extensionSignalFixture) file(in ExtensionSignal) (created bool, id ids.UUID, err error) {
	in.EntityType, in.EntityID = "company", f.company
	err = f.store.tx(f.ctx, func(tx pgx.Tx) error {
		sig, made, err := RecordExtensionTx(f.ctx, tx, in)
		created, id = made, ids.UUID(sig.Id)
		return err
	})
	return created, id, err
}

func loss(fingerprint string) ExtensionSignal {
	return ExtensionSignal{
		Unit: "sigtest", Kind: "ext:sigtest:loss", Fingerprint: fingerprint,
		Fields: map[string]any{"amount": 120.5, "reporter": "A. Reporter"},
	}
}

func TestAnExtensionSignalCarriesItsLayoutAndCoreStampedProvenance(t *testing.T) {
	f := newExtensionSignalFixture(t)
	_, id, err := f.file(loss("provenance-1"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.store.GetSignal(f.ctx, ids.From[ids.SignalKind](id), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != "ext:sigtest:loss" || len(got.Evidence) != 1 {
		t.Fatalf("the signal reads kind %q with %d evidence claims", got.Kind, len(got.Evidence))
	}
	claim := got.Evidence[0]
	if claim.Fields == nil || (*claim.Fields)["amount"] != 120.5 {
		t.Errorf("the evidence layout members read %v", claim.Fields)
	}
	if claim.Provenance == nil || claim.Provenance.Unit != "sigtest" || claim.Provenance.CapturedBy != f.actorID {
		t.Errorf("the provenance reads %+v, want unit sigtest and captured_by %s", claim.Provenance, f.actorID)
	}
}

func TestTheSameFingerprintFromTwoReportersIsOneSignal(t *testing.T) {
	f := newExtensionSignalFixture(t)
	createdFirst, first, err := f.file(loss("same-event"))
	if err != nil {
		t.Fatal(err)
	}
	createdSecond, second, err := f.file(loss("same-event"))
	if err != nil {
		t.Fatal(err)
	}
	if !createdFirst || createdSecond || first != second {
		t.Errorf("first created=%v id=%s, repeat created=%v id=%s; want one signal answered twice", createdFirst, first, createdSecond, second)
	}
	var rows, audits int
	if err := f.owner.QueryRow(f.ctx, `SELECT count(*) FROM signal WHERE kind = 'ext:sigtest:loss' AND fingerprint LIKE '%same-event'`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := f.owner.QueryRow(f.ctx, `SELECT count(*) FROM audit_log WHERE entity_type = 'signal' AND entity_id = $1`, first).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if rows != 1 || audits != 1 {
		t.Errorf("%d signal rows and %d audit rows, want one of each", rows, audits)
	}
}

func TestADismissedExtensionSignalIsNotRaisedAgainByAReplay(t *testing.T) {
	f := newExtensionSignalFixture(t)
	_, first, err := f.file(loss("dismissed-event"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.owner.Exec(f.ctx, `UPDATE signal SET status = 'dismissed' WHERE id = $1`, first); err != nil {
		t.Fatal(err)
	}
	created, again, err := f.file(loss("dismissed-event"))
	if err != nil || created || again != first {
		t.Fatalf("replay of a dismissed signal: created=%v id=%s err=%v, want the dismissed row and no new one", created, again, err)
	}
}

func TestTheSameKeyOnTwoSubjectsIsTwoSignals(t *testing.T) {
	f := newExtensionSignalFixture(t)
	if _, _, err := f.file(loss("shared-key")); err != nil {
		t.Fatal(err)
	}
	other := ids.NewV7()
	if _, err := f.owner.Exec(f.ctx,
		`INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Other', 'manual', 'human:test')`, other); err != nil {
		t.Fatal(err)
	}
	f.company = other
	created, _, err := f.file(loss("shared-key"))
	if err != nil || !created {
		t.Fatalf("the same key on another subject: created=%v err=%v, want a second signal", created, err)
	}
}

func TestAnExtensionSignalThatBreaksTheRulesIsRefused(t *testing.T) {
	f := newExtensionSignalFixture(t)
	mutate := func(fn func(*ExtensionSignal)) ExtensionSignal {
		in := loss("refused")
		in.Fields = map[string]any{"amount": 1.0}
		fn(&in)
		return in
	}
	cases := map[string]ExtensionSignal{
		"another unit's kind":    mutate(func(in *ExtensionSignal) { in.Unit = "other" }),
		"a core kind":            mutate(func(in *ExtensionSignal) { in.Kind = "risk" }),
		"an undeclared kind":     mutate(func(in *ExtensionSignal) { in.Kind = "ext:sigtest:unknown" }),
		"a required member gone": mutate(func(in *ExtensionSignal) { in.Fields = map[string]any{} }),
		"an unknown member":      mutate(func(in *ExtensionSignal) { in.Fields["provenance"] = "forged" }),
		"no fingerprint":         mutate(func(in *ExtensionSignal) { in.Fingerprint = " " }),
	}
	for name, in := range cases {
		_, _, err := f.file(in)
		if _, ok := errors.AsType[*InvalidExtensionSignalError](err); !ok {
			t.Errorf("%s: got %v, want InvalidExtensionSignalError", name, err)
		}
	}
}

func TestTheSignalKindCheckAdmitsOnlyCoreKindsAndTheExtensionSpelling(t *testing.T) {
	f := newExtensionSignalFixture(t)
	insert := func(kind string) error {
		_, err := f.owner.Exec(f.ctx, `
			INSERT INTO signal (kind, source_channel, resolution_state, severity, summary, evidence, status, detected_at, source, captured_by)
			VALUES ($1, 'derived', 'unresolved', 'info', 'x', '[]'::jsonb, 'open', now(), 'test', 'test')`, kind)
		return err
	}
	for _, kind := range []string{"risk", "technical_change", "ext:unit-a:loss", "ext:a:x_9"} {
		if err := insert(kind); err != nil {
			t.Errorf("kind %q refused: %v", kind, err)
		}
	}
	for _, kind := range []string{"made_up", "ext:Unit:loss", "ext:unit:", "ext:unit:9lives", "extra:unit:loss"} {
		if err := insert(kind); err == nil {
			t.Errorf("kind %q admitted", kind)
		}
	}
}

func TestAnExtensionSignalCannotBeMovedToACoreKindOrHaveItsEvidenceReplaced(t *testing.T) {
	f := newExtensionSignalFixture(t)
	_, id, err := f.file(loss("frozen-1"))
	if err != nil {
		t.Fatal(err)
	}
	sid := ids.From[ids.SignalKind](id)
	kind := "risk"
	var invalidKind *InvalidSignalKindError
	if _, err := f.store.UpdateSignal(f.ctx, sid, UpdateSignalInput{Kind: &kind}); !errors.As(err, &invalidKind) {
		t.Errorf("moving an extension signal to a core kind returned %v, want InvalidSignalKindError", err)
	}
	evidence := []crmcontracts.SignalEvidence{{Snippet: "replaced"}}
	var invalidEvidence *InvalidExtensionSignalError
	if _, err := f.store.UpdateSignal(f.ctx, sid, UpdateSignalInput{Evidence: &evidence}); !errors.As(err, &invalidEvidence) {
		t.Errorf("replacing an extension signal's evidence returned %v, want InvalidExtensionSignalError", err)
	}
}

func TestACallerCannotForgeExtensionEvidenceThroughTheHumanPaths(t *testing.T) {
	f := newExtensionSignalFixture(t)
	forged := []crmcontracts.SignalEvidence{{Snippet: "x", Provenance: &crmcontracts.SignalProvenance{Unit: "sigtest", CapturedBy: "someone"}}}
	_, err := f.store.CreateSignal(f.ctx, CreateSignalInput{Kind: "risk", Summary: "s", Evidence: forged})
	var refusal *InvalidExtensionSignalError
	if !errors.As(err, &refusal) {
		t.Errorf("a request carrying provenance returned %v, want InvalidExtensionSignalError", err)
	}
	_, err = f.store.CreateSignal(f.ctx, CreateSignalInput{Kind: "ext:sigtest:loss", Summary: "s"})
	if !errors.As(err, &refusal) {
		t.Errorf("a request naming an extension kind returned %v, want InvalidExtensionSignalError", err)
	}
}

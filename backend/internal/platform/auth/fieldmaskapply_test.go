// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth_test

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// pricedRow stands in for the wire records the modules mask. It is declared
// here because platform owns no contract type and must not learn one: what
// ApplyFieldMasks knows about a row is the accessors its caller hands over.
type pricedRow struct {
	id          ids.UUID
	amount      *int64
	arr         *int64
	unitPrice   *int64
	currency    *string
	companyID   *ids.UUID
	partnerID   *ids.UUID
	attribution *string
	masked      []string
	writable    bool
}

func rowID(r pricedRow) ids.UUID { return r.id }

func setMasked(r *pricedRow, names []string) { r.masked = names }

// pricedWithholds is the registry a module hands over: one deliberate act per
// field. project_id is deliberately absent — it is the name nothing withholds.
var pricedWithholds = map[string]func(*pricedRow){
	"amount_minor":        func(r *pricedRow) { r.amount = nil },
	"expected_arr_minor":  func(r *pricedRow) { r.arr = nil },
	"unit_price_minor":    func(r *pricedRow) { r.unitPrice = nil },
	"currency":            func(r *pricedRow) { r.currency = nil },
	"company_id":          func(r *pricedRow) { r.companyID = nil },
	"partner_company_id":  func(r *pricedRow) { r.partnerID = nil },
	"partner_attribution": func(r *pricedRow) { r.attribution = nil },
}

func pricedPage(rowIDs ...ids.UUID) []pricedRow {
	page := make([]pricedRow, 0, len(rowIDs))
	for _, id := range rowIDs {
		amount, arr, price, currency := int64(1200), int64(400), int64(99), "EUR"
		company, partner, attribution := ids.NewV7(), ids.NewV7(), "sourced"
		page = append(page, pricedRow{
			id: id, amount: &amount, arr: &arr, unitPrice: &price,
			currency: &currency, companyID: &company,
			partnerID: &partner, attribution: &attribution,
		})
	}
	return page
}

// A conditioned mask on an object whose rows carry no owner and no grant has no
// question to ask, and WritableSubset refuses that table outright. The pass must
// withhold on every row instead: a 500 out of the function whose whole job is
// safe rendering is the worst of both answers.
func TestAConditionedMaskOnAnUnshareableObjectWithholdsEveryRow(t *testing.T) {
	t.Parallel()
	ctx := maskedActor(principal.FieldMask{
		Object: "product", Field: "unit_price_minor", Condition: principal.MaskOutsideWriteAuthority,
	})
	page := pricedPage(ids.NewV7(), ids.NewV7())
	tx := &writableRowsTx{}

	if err := auth.ApplyFieldMasks(ctx, tx, "product", page,
		rowID, pricedWithholds, setMasked, nil, nil); err != nil {
		t.Fatalf("ApplyFieldMasks refused a product page instead of withholding it: %v", err)
	}
	if tx.queries != 0 {
		t.Errorf("the pass asked the database %d times about a table that answers no write authority", tx.queries)
	}
	for i, row := range page {
		if row.unitPrice != nil {
			t.Errorf("row %d kept its unit price under a condition nothing can lift", i)
		}
		if !slices.Equal(row.masked, []string{"unit_price_minor"}) {
			t.Errorf("row %d named %v, want the price the condition could not lift on", i, row.masked)
		}
	}
}

// The wire contract in one row: the value goes out null and the record names
// the field, so a reader tells withheld from empty — and the group travels with
// it, so no currency is left standing beside the amount just withheld.
func TestAWithheldFieldIsNulledAndNamed(t *testing.T) {
	t.Parallel()
	ctx := maskedActor(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways,
	})
	page := pricedPage(ids.NewV7())
	tx := &writableRowsTx{}

	if err := auth.ApplyFieldMasks(ctx, tx, "deal", page, rowID, pricedWithholds, setMasked, nil, nil); err != nil {
		t.Fatalf("ApplyFieldMasks: %v", err)
	}
	if tx.queries != 0 {
		t.Errorf("an unconditioned page cost %d statements; it must cost none", tx.queries)
	}
	row := page[0]
	if row.amount != nil || row.arr != nil || row.currency != nil {
		t.Errorf("the money survived the mask: amount=%v arr=%v currency=%v", row.amount, row.arr, row.currency)
	}
	want := []string{"amount_minor", "expected_arr_minor", "currency"}
	if !slices.Equal(row.masked, want) {
		t.Errorf("masked_fields = %v, want %v in that order", row.masked, want)
	}
}

// Write authority is resolved in ONE statement for the whole page, and the mask
// lifts exactly on the rows that statement names.
func TestAConditionedMaskLiftsOnARowTheCallerCouldChange(t *testing.T) {
	t.Parallel()
	ctx := maskedActor(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority,
	})
	mine, theirs := ids.NewV7(), ids.NewV7()
	page := pricedPage(mine, theirs)
	tx := &writableRowsTx{writable: []ids.UUID{mine}}

	if err := auth.ApplyFieldMasks(ctx, tx, "deal", page, rowID, pricedWithholds, setMasked, nil, nil); err != nil {
		t.Fatalf("ApplyFieldMasks: %v", err)
	}
	if tx.queries != 1 {
		t.Errorf("the page asked the database %d times; write authority is one statement per page", tx.queries)
	}
	if page[0].amount == nil || len(page[0].masked) != 0 {
		t.Errorf("a deal the caller could change was masked anyway: masked=%v", page[0].masked)
	}
	if page[1].amount != nil {
		t.Error("a deal outside the caller's write authority kept its amount")
	}
	if !slices.Contains(page[1].masked, "amount_minor") {
		t.Errorf("masked_fields = %v, want the withheld amount named", page[1].masked)
	}
}

// Archiving a record ends the caller's edit, not their authority over it. The
// flag and the authority map answer differently on exactly that row, and the
// masks read the map: a rep who archives their own deal still reads its amount,
// while nothing offers them the edit every mutation would refuse.
func TestAnArchivedRowLosesItsEditAffordanceAndKeepsItsValue(t *testing.T) {
	t.Parallel()
	ctx := maskedActor(principal.FieldMask{
		Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority,
	})
	mine := ids.NewV7()
	page := pricedPage(mine)
	tx := &writableRowsTx{writable: []ids.UUID{mine}, archived: []ids.UUID{mine}}

	writable, err := auth.StampWritable(ctx, tx, "deal", page, rowID,
		func(r *pricedRow, may bool) { r.writable = may })
	if err != nil {
		t.Fatalf("StampWritable: %v", err)
	}
	if page[0].writable {
		t.Error("an archived row was offered as editable; every mutation refuses it")
	}
	if !writable[mine] {
		t.Error("archiving a row withdrew the authority the masks resolve against")
	}

	if err := auth.ApplyFieldMasks(ctx, tx, "deal", page, rowID,
		pricedWithholds, setMasked, nil, writable); err != nil {
		t.Fatalf("ApplyFieldMasks: %v", err)
	}
	if page[0].amount == nil || len(page[0].masked) != 0 {
		t.Errorf("archiving a deal hid its amount from its own owner: masked=%v", page[0].masked)
	}
	if tx.queries != 2 {
		t.Errorf("the page cost %d statements; authority and liveness are one each, and a supplied map asks neither again", tx.queries)
	}
}

// A name nothing withholds is dropped rather than reported: naming a field in
// masked_fields while still sending its value is a worse answer than either
// half alone. The names a module collected itself arrive the same way.
func TestANameWithNoWithholdFuncIsDroppedRatherThanReported(t *testing.T) {
	t.Parallel()
	ctx := maskedActor(principal.FieldMask{
		Object: "deal", Field: "currency", Condition: principal.MaskAlways,
	})
	page := pricedPage(ids.NewV7())
	tx := &writableRowsTx{}

	err := auth.ApplyFieldMasks(ctx, tx, "deal", page, rowID, pricedWithholds, setMasked,
		func(int) []string { return []string{"company_id", "project_id"} }, nil)
	if err != nil {
		t.Fatalf("ApplyFieldMasks: %v", err)
	}
	row := page[0]
	if row.companyID != nil {
		t.Error("a reference the caller may not open was named but still sent")
	}
	want := []string{"currency", "company_id"}
	if !slices.Equal(row.masked, want) {
		t.Errorf("masked_fields = %v, want %v — project_id has no withhold func and must not be reported", row.masked, want)
	}
	if row.amount == nil {
		t.Error("a currency mask dragged the amount along; the group is directed, not symmetric")
	}
}

// A name the MODULE collected carries the group with it, like a configured
// mask does. The reason a field is withheld does not change what giving it
// back would disclose: a deal pointing at a partner this reader cannot open
// withholds the partner, and "sourced" left standing beside the null says some
// partner brought the deal.
func TestANameTheModuleCollectedWithholdsItsGroupToo(t *testing.T) {
	t.Parallel()
	ctx := maskedActor()
	page := pricedPage(ids.NewV7())
	tx := &writableRowsTx{}

	err := auth.ApplyFieldMasks(ctx, tx, "deal", page, rowID, pricedWithholds, setMasked,
		func(int) []string { return []string{"partner_company_id"} }, nil)
	if err != nil {
		t.Fatalf("ApplyFieldMasks: %v", err)
	}
	row := page[0]
	if row.attribution != nil {
		t.Errorf("attribution = %q beside a withheld partner", *row.attribution)
	}
	want := []string{"partner_company_id", "partner_attribution"}
	if !slices.Equal(row.masked, want) {
		t.Errorf("masked_fields = %v, want %v — a field nulled and not named reads as one nobody filled in",
			row.masked, want)
	}
}

// writableRowsTx is the database boundary of these passes: it answers the
// write-authority statement with the ids the caller could change and the
// liveness statement with the ids that are archived, and counts the asking so a
// page that must pay nothing can prove it.
type writableRowsTx struct {
	writable []ids.UUID
	archived []ids.UUID
	queries  int
}

func (t *writableRowsTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	t.queries++
	if strings.Contains(sql, "archived_at IS NOT NULL") {
		return &uuidRows{remaining: t.archived}, nil
	}
	return &uuidRows{remaining: t.writable}, nil
}

func (t *writableRowsTx) Begin(context.Context) (pgx.Tx, error) {
	panic("writableRowsTx: Begin not implemented")
}

func (t *writableRowsTx) Commit(context.Context) error {
	panic("writableRowsTx: Commit not implemented")
}

func (t *writableRowsTx) Rollback(context.Context) error {
	panic("writableRowsTx: Rollback not implemented")
}

func (t *writableRowsTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("writableRowsTx: CopyFrom not implemented")
}

func (t *writableRowsTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("writableRowsTx: SendBatch not implemented")
}

func (t *writableRowsTx) LargeObjects() pgx.LargeObjects {
	panic("writableRowsTx: LargeObjects not implemented")
}

func (t *writableRowsTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("writableRowsTx: Prepare not implemented")
}

func (t *writableRowsTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("writableRowsTx: Exec not implemented")
}

func (t *writableRowsTx) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("writableRowsTx: QueryRow not implemented")
}

func (t *writableRowsTx) Conn() *pgx.Conn { panic("writableRowsTx: Conn not implemented") }

// uuidRows replays a list of row ids for the single-column SELECT the write
// authority statement issues.
type uuidRows struct {
	remaining []ids.UUID
	current   ids.UUID
}

func (r *uuidRows) Next() bool {
	if len(r.remaining) == 0 {
		return false
	}
	r.current, r.remaining = r.remaining[0], r.remaining[1:]
	return true
}

func (r *uuidRows) Scan(dest ...any) error {
	if len(dest) != 1 {
		return fmt.Errorf("uuidRows: the statement selects one id, got %d destinations", len(dest))
	}
	id, ok := dest[0].(*ids.UUID)
	if !ok {
		return fmt.Errorf("uuidRows: destination %T is not a row id", dest[0])
	}
	*id = r.current
	return nil
}

func (r *uuidRows) Close()                                       {}
func (r *uuidRows) Err() error                                   { return nil }
func (r *uuidRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *uuidRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *uuidRows) Conn() *pgx.Conn                              { return nil }

func (r *uuidRows) Values() ([]any, error) { panic("uuidRows: Values not implemented") }

func (r *uuidRows) RawValues() [][]byte { panic("uuidRows: RawValues not implemented") }

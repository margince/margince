// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// Every advertised target must survive BOTH paths — the create and the patch.
// A field the import offers, accepts, and then drops on one of the two is worse
// than one the screen never showed, and the only way that stays true as the
// stores change is to check the round trip rather than the list.
func TestEveryImportTargetRoundTripsThroughCreateAndUpdate(t *testing.T) {
	// Swept here because this is the test that walks every object's full target
	// set: an exemption matching nothing would otherwise sit on ratifying a
	// target that no longer exists.
	defer nonFieldTargets.AssertAllMatched(t)
	for object, build := range map[string]func(map[string]string) (created, patched map[string]bool){
		migration.ObjectLead: func(fields map[string]string) (map[string]bool, map[string]bool) {
			in := leadCreateFrom(fields, "import:csv", "ext-1", "src")
			up := leadUpdateFrom(fields)
			return map[string]bool{
					"full_name":    in.FullName != nil,
					"email":        in.Email != nil,
					"title":        in.Title != nil,
					"company_name": in.CompanyName != nil,
				}, map[string]bool{
					"full_name":    up.FullName != nil,
					"email":        up.Email != nil,
					"title":        up.Title != nil,
					"company_name": up.CompanyName != nil,
				}
		},
		migration.ObjectOrganization: func(fields map[string]string) (map[string]bool, map[string]bool) {
			in := organizationCreateFrom(fields, "src")
			up := organizationUpdateFrom(fields)
			return map[string]bool{
					"display_name":        in.DisplayName != "",
					"legal_name":          in.LegalName != nil,
					"industry":            in.Industry != nil,
					"size_band":           in.SizeBand != nil,
					"description":         in.Description != nil,
					"domain":              len(in.Domains) > 0,
					"address.line1":       in.Address != nil && in.Address.Line1 != nil,
					"address.line2":       in.Address != nil && in.Address.Line2 != nil,
					"address.city":        in.Address != nil && in.Address.City != nil,
					"address.region":      in.Address != nil && in.Address.Region != nil,
					"address.postal_code": in.Address != nil && in.Address.PostalCode != nil,
					"address.country":     in.Address != nil && in.Address.Country != nil,
				}, map[string]bool{
					"display_name": up.DisplayName != nil,
					"legal_name":   up.LegalName != nil,
					"industry":     up.Industry != nil,
					"size_band":    up.SizeBand != nil,
					"description":  up.Description != nil,
					// The replace-set behind a pointer: non-nil means the file
					// carried a domain column, and only then is the stored set
					// rewritten.
					"domain":              up.Domains != nil && len(*up.Domains) > 0,
					"address.line1":       up.Address != nil && up.Address.Line1 != nil,
					"address.line2":       up.Address != nil && up.Address.Line2 != nil,
					"address.city":        up.Address != nil && up.Address.City != nil,
					"address.region":      up.Address != nil && up.Address.Region != nil,
					"address.postal_code": up.Address != nil && up.Address.PostalCode != nil,
					"address.country":     up.Address != nil && up.Address.Country != nil,
				}
		},
		migration.ObjectPerson: func(fields map[string]string) (map[string]bool, map[string]bool) {
			in := personCreateFrom(fields, "src")
			up := personUpdateFrom(fields)
			return map[string]bool{
					"full_name":           in.FullName != "",
					"first_name":          in.FirstName != nil,
					"last_name":           in.LastName != nil,
					"title":               in.Title != nil,
					"email":               len(in.Emails) > 0,
					"address.line1":       in.Address != nil && in.Address.Line1 != nil,
					"address.line2":       in.Address != nil && in.Address.Line2 != nil,
					"address.city":        in.Address != nil && in.Address.City != nil,
					"address.region":      in.Address != nil && in.Address.Region != nil,
					"address.postal_code": in.Address != nil && in.Address.PostalCode != nil,
					"address.country":     in.Address != nil && in.Address.Country != nil,
				}, map[string]bool{
					"full_name":  up.FullName != nil,
					"first_name": up.FirstName != nil,
					"last_name":  up.LastName != nil,
					"title":      up.Title != nil,
					// The half that did not exist before this object did: a
					// person's emails are child rows, and the patch input
					// carried no member for them at all.
					"email":               len(up.Emails) > 0,
					"address.line1":       up.Address != nil && up.Address.Line1 != nil,
					"address.line2":       up.Address != nil && up.Address.Line2 != nil,
					"address.city":        up.Address != nil && up.Address.City != nil,
					"address.region":      up.Address != nil && up.Address.Region != nil,
					"address.postal_code": up.Address != nil && up.Address.PostalCode != nil,
					"address.country":     up.Address != nil && up.Address.Country != nil,
				}
		},
	} {
		t.Run(object, func(t *testing.T) {
			targets, err := importTargets(object)
			if err != nil {
				t.Fatalf("importTargets: %v", err)
			}
			fields := make(map[string]string, len(targets))
			for _, target := range targets {
				fields[target] = "value"
			}
			created, patched := build(fields)
			for _, target := range targets {
				if nonFieldTargets.Waived(t, target) {
					// Not a field, so the round-trip rule does not apply. Each
					// exemption is asserted the other way round below: advertised,
					// and reaching neither input on purpose. Naming the reason here
					// is what stops a future target being exempted by accident —
					// a target with no entry is held to the rule.
					continue
				}
				if !created[target] {
					t.Errorf("%s: target %q is advertised but never reaches the create input", object, target)
				}
				if !patched[target] {
					t.Errorf("%s: target %q is advertised but never reaches the update input", object, target)
				}
			}
			if selectsByID(object) {
				assertNonFieldTarget(t, object, csvTargetID, targets, created, patched,
					"it names the record a row IS, and writing it would let a file move a company onto another company's id",
					"a corrections file naming its records would be refused")
			}
			if linksEmployer(object) {
				assertNonFieldTarget(t, object, csvEmployerName, targets, created, patched,
					"it names the company a person works AT, and writing it would put a company's name in a field on the person",
					"a contact file naming employers would be refused")
			}
		})
	}
}

// A custom-field column is not offered, because the caller-opened transaction
// the import writes through refuses custom fields: offering one would accept a
// column, report the row as written, and drop the value.
func TestImportTargetsOfferNoCustomFields(t *testing.T) {
	for _, object := range []string{migration.ObjectLead, migration.ObjectOrganization, migration.ObjectPerson} {
		targets, err := importTargets(object)
		if err != nil {
			t.Fatalf("importTargets(%s): %v", object, err)
		}
		for _, target := range targets {
			if len(target) > 3 && target[:3] == "cf_" {
				t.Errorf("%s advertises %q, which the import write path cannot carry", object, target)
			}
		}
	}
}

func TestChangedFieldsComparesEmailAsTheStoreWillHoldIt(t *testing.T) {
	stored := []byte(`{"email":"ada@lovelace.example","full_name":"Ada Lovelace"}`)

	// The store lowercases email on write. Compared raw, a file spelling it
	// differently would rewrite the row on every single re-import.
	changed, err := changedFields(stored, map[string]string{"email": "Ada@Lovelace.Example"})
	if err != nil {
		t.Fatalf("changedFields: %v", err)
	}
	if len(changed) != 0 {
		t.Fatalf("changed = %v, want none: the stored value IS this value", changed)
	}

	// A real change is still a change.
	changed, err = changedFields(stored, map[string]string{"email": "ada@newplace.example"})
	if err != nil {
		t.Fatalf("changedFields: %v", err)
	}
	if changed["email"] != "ada@newplace.example" {
		t.Fatalf("changed = %v, want the new address", changed)
	}

	// Case is NOT folded on a field the store keeps verbatim.
	changed, err = changedFields(stored, map[string]string{"full_name": "ADA LOVELACE"})
	if err != nil {
		t.Fatalf("changedFields: %v", err)
	}
	if changed["full_name"] != "ADA LOVELACE" {
		t.Fatalf("changed = %v, want the renamed value: only email is canonicalized", changed)
	}
}

func TestMappingFromRefusesTwoColumnsOntoOneField(t *testing.T) {
	_, err := mappingFrom(migration.ObjectLead, crmcontracts.CreateImportRunRequest{
		Mapping: map[string]string{"Work Email": "email", "Personal Email": "email"},
	})
	if err == nil {
		t.Fatal("two columns onto one field were accepted; the row builder would pick one at random")
	}
}

func TestMappingFromRefusesATargetTheObjectDoesNotHave(t *testing.T) {
	_, err := mappingFrom(migration.ObjectLead, crmcontracts.CreateImportRunRequest{
		Mapping: map[string]string{"Revenue": "annual_revenue"},
	})
	if err == nil {
		t.Fatal("an unknown target was accepted; the run would fail at the first row instead")
	}
}

// Both refusals below are dead ends without the vocabulary: the mapping targets
// are a closed set that no error, schema or tool description spells out, so a
// caller who guesses wrong has nowhere to look. An agent driving this over MCP
// cannot open the field catalog the way a screen can.
func TestMappingRefusalsNameTheFieldsTheObjectAccepts(t *testing.T) {
	targets, err := importTargets(migration.ObjectOrganization)
	if err != nil {
		t.Fatalf("importTargets: %v", err)
	}

	cases := map[string]map[string]string{
		// "city" reads like an obvious company field and is not one.
		"unknown target": {"City": "city"},
		// A header whose columns match nothing: SuggestMapping proposes
		// nothing by design, so the caller arrives here having sent no mapping.
		"nothing mapped": {},
	}

	for name, mapping := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := mappingFrom(migration.ObjectOrganization, crmcontracts.CreateImportRunRequest{
				Mapping: mapping,
			})
			if err == nil {
				t.Fatal("the mapping was accepted; expected a refusal")
			}
			for _, target := range targets {
				if !strings.Contains(err.Error(), target) {
					t.Errorf("the refusal does not name %q, so the caller cannot act on it: %v", target, err)
				}
			}
		})
	}
}

func TestMappingFromNeedsAColumnThatIdentifiesARow(t *testing.T) {
	// Nothing maps to email, and no explicit source key is given, so no row
	// could be recognized on a re-import or found by an undo.
	_, err := mappingFrom(migration.ObjectLead, crmcontracts.CreateImportRunRequest{
		Mapping: map[string]string{"Name": "full_name"},
	})
	if err == nil {
		t.Fatal("a mapping with no identifying column was accepted")
	}
}

func TestMappingFromRefusesASourceKeyThatIsNotMapped(t *testing.T) {
	key := "Some Other Column"
	_, err := mappingFrom(migration.ObjectLead, crmcontracts.CreateImportRunRequest{
		Mapping:   map[string]string{"Email": "email"},
		SourceKey: &key,
	})
	if err == nil {
		t.Fatal("a source key naming an unmapped column was accepted")
	}
}

func TestMappingFromDefaultsTheSourceKeyToTheIdentifyingColumn(t *testing.T) {
	mapping, err := mappingFrom(migration.ObjectLead, crmcontracts.CreateImportRunRequest{
		Mapping: map[string]string{"E-mail": "email", "Name": "full_name"},
	})
	if err != nil {
		t.Fatalf("mappingFrom: %v", err)
	}
	if mapping.SourceKey != "E-mail" {
		t.Fatalf("source key = %q, want the column mapped onto email", mapping.SourceKey)
	}
}

// A vanished upload is the caller's to fix by re-uploading, so it is named as
// missing rather than blamed on the server. Asserted through the sentinel the
// transport maps, not merely as "some error": a misclassified one answers 500.
func TestImportProblemNamesAVanishedUploadAsNotFound(t *testing.T) {
	err := importProblem(fmt.Errorf("import source %q: %w", "ws/import/x", blobstore.ErrNotFound))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want it to carry ErrNotFound so the transport answers 404", err)
	}

	// A file the customer can fix is a 422 naming the field, never a 500.
	unreadable := importProblem(fmt.Errorf("%w: line 4", migration.ErrSourceUnreadable))
	if errors.Is(unreadable, apperrors.ErrNotFound) {
		t.Fatalf("an unreadable file was classified as missing: %v", unreadable)
	}
	if unreadable == nil {
		t.Fatal("importProblem dropped the error")
	}
}

// The disposition is what a human judges an import by, and the contract states
// its invariant outright: the four counts sum to the rows read. A completed
// run's stored report carries BOTH the prediction and the outcome — the engine
// merges them so a resumed attempt keeps what the first one achieved — so a
// mapper that carried each figure through would report more rows than the file
// holds. This is the shape that failed live: 5 rows read, 6 unchanged.
func TestTheDispositionAlwaysSumsToTheRowsRead(t *testing.T) {
	// The prediction and the outcome carry DIFFERENT numbers on purpose: with
	// equal ones a mapper that read the wrong side would still look right, and
	// this test would prove only that it read something.
	merged := migration.Report{Objects: []migration.ObjectReport{{
		Object: migration.ObjectLead, MirrorCount: 5,
		// What the dry run predicted: four rows would be created.
		WillCreate: 4, WillUpdate: 0,
		// What the commit then did: three created, one updated — both stored,
		// because a run's report is merged into the dry run's so a resumed
		// attempt keeps what the first achieved.
		Created: 3, Updated: 1, Unchanged: 6,
		Skipped: []migration.SkippedRow{
			{ExternalID: "line 5", Reason: "no key"},
			{ExternalID: "line 5", Reason: "no key"},
		},
	}}}

	for _, tc := range []struct {
		status           string
		created, updated int
	}{
		{migration.StatusAwaitingApproval, 4, 0},
		{migration.StatusComplete, 3, 1},
		{migration.StatusFailed, 3, 1},
	} {
		t.Run(tc.status, func(t *testing.T) {
			got := toContractImportReport(migration.Run{
				Status: tc.status, Report: &merged,
				Mapping: &migration.RunMapping{Object: migration.ObjectLead},
			})
			d := got.Disposition
			if d.Created != tc.created || d.Updated != tc.updated {
				t.Fatalf("created/updated = %d/%d, want %d/%d — a %s run reports the numbers of its own side",
					d.Created, d.Updated, tc.created, tc.updated, tc.status)
			}
			// One row the human must go fix, not two reports of the same one.
			if d.Skipped != 1 || len(got.Issues) != 1 {
				t.Fatalf("skipped = %d with %d issues, want the one row named once", d.Skipped, len(got.Issues))
			}
			// Unchanged is what is left of the rows read, never the stored
			// figure — which carries both attempts and would report 6 of 5.
			if total := d.Created + d.Updated + d.Unchanged + d.Skipped; total != got.RowsRead {
				t.Fatalf("disposition sums to %d but %d rows were read: %+v", total, got.RowsRead, d)
			}
		})
	}
}

func TestToContractReportNeverSumsAPredictionWithAnOutcome(t *testing.T) {
	report := migration.Report{Objects: []migration.ObjectReport{{
		Object: migration.ObjectLead, MirrorCount: 3,
		WillCreate: 3, Created: 3,
	}}}

	awaiting := toContractImportReport(migration.Run{
		Status: migration.StatusAwaitingApproval, Report: &report,
		Mapping: &migration.RunMapping{Object: migration.ObjectLead},
	})
	if awaiting.Disposition.Created != 3 {
		t.Fatalf("awaiting created = %d, want the 3 it predicts", awaiting.Disposition.Created)
	}

	// The stored report carries both legs after a completed run merges them;
	// adding them would tell a human 6 rows landed out of a 3-row file.
	done := toContractImportReport(migration.Run{
		Status: migration.StatusComplete, Report: &report,
		Mapping: &migration.RunMapping{Object: migration.ObjectLead},
	})
	if done.Disposition.Created != 3 {
		t.Fatalf("completed created = %d, want the 3 that actually landed", done.Disposition.Created)
	}
}

// A flat file carries no edges, so an edge reaching the writer came from
// somewhere it does not understand. It is disclosed as not applied, never
// swallowed as done.
func TestCSVWritersDiscloseAnEdgeItCannotApply(t *testing.T) {
	w := &csvWriters{object: migration.ObjectLead}

	res, err := w.Associate(t.Context(), migration.Assoc{FromType: "lead", ToType: "organization"})
	if err != nil {
		t.Fatalf("Associate: %v", err)
	}
	if res.Applied {
		t.Fatal("an edge a delimited import cannot carry was reported as applied")
	}
	if res.Reason == "" {
		t.Fatal("the non-applied edge carries no reason, so the report cannot disclose it")
	}
}

// Nothing to repair: every landing commits the record and its identity row in
// one transaction, which is the answer the seam documents for such a writer.
func TestCSVWritersHaveNoIdentitiesToReconcile(t *testing.T) {
	w := &csvWriters{object: migration.ObjectLead}
	if err := w.ReconcileIdentities(t.Context()); err != nil {
		t.Fatalf("ReconcileIdentities: %v", err)
	}
}

// The run carries one object. A row for another one is an error rather than a
// quiet no-op: it would mean the source and the run disagree about what is
// being imported.
func TestCSVWritersRefuseARowForAnotherObject(t *testing.T) {
	w := &csvWriters{object: migration.ObjectLead, nativeIDs: map[string]ids.UUID{}}

	if _, err := w.Ensure(t.Context(), migration.ObjectOrganization, migration.Row{ExternalID: "x"}); err == nil {
		t.Fatal("a row for an object this run does not carry was accepted")
	}
}

// The stamp an imported row carries is the reserved import namespace, which the
// wire mappers refuse — so a client cannot pre-plant a row under a guessed
// import id and have the store hand it back as already imported.
func TestImportedRowsCarryTheReservedProvenance(t *testing.T) {
	w := &csvWriters{object: migration.ObjectLead}

	stamp := w.provenanceOf("ada@lovelace.example")
	if !strings.HasPrefix(stamp, provenance.ReservedSourceSystemPrefix) {
		t.Fatalf("provenance %q does not sit in the reserved import namespace", stamp)
	}
	if !strings.Contains(stamp, migration.ObjectLead) || !strings.Contains(stamp, "ada@lovelace.example") {
		t.Fatalf("provenance %q does not name the object and the source row", stamp)
	}
	if !strings.HasPrefix(csvSourceSystem(), provenance.ReservedSourceSystemPrefix) {
		t.Fatalf("source system %q is not reserved", csvSourceSystem())
	}
}

// A skip the SOURCE recorded never reached the writer, so nothing else in the
// report would mention it — it is folded into the object's own skips, with the
// line a human opens the file to.
func TestSkippedLinesReachTheObjectsReport(t *testing.T) {
	report := migration.Report{Objects: []migration.ObjectReport{{Object: migration.ObjectLead}}}

	out := withSkippedLines(report, migration.ObjectLead, []migration.SkippedLine{{Line: 7, Reason: "no key"}})

	if len(out.Objects[0].Skipped) != 1 {
		t.Fatalf("skips = %+v, want the source's own disclosure", out.Objects[0].Skipped)
	}
	if got := out.Objects[0].Skipped[0].Line; got != 7 {
		t.Fatalf("line = %d, want 7 — the report must send a human to the right line", got)
	}
	// An object the report does not carry cannot silently swallow them either.
	untouched := withSkippedLines(report, migration.ObjectOrganization, []migration.SkippedLine{{Line: 2}})
	if len(untouched.Objects[0].Skipped) != 1 {
		t.Fatalf("skips = %+v, want the lead's own single skip", untouched.Objects[0].Skipped)
	}
}

func TestColumnProfileReachesTheWireWithSamplesAndRate(t *testing.T) {
	out := toContractColumns(migration.Profile{Columns: []migration.Column{
		{Header: "Email", FillRate: 0.5, Samples: []string{"a@x.test"}},
		{Header: "Empty"},
	}})

	if len(out) != 2 {
		t.Fatalf("columns = %d, want 2", len(out))
	}
	if out[0].FillRate != 0.5 || len(out[0].Samples) != 1 {
		t.Fatalf("column = %+v, want the rate and the sample the profile carried", out[0])
	}
	// An empty column answers [], never null: the contract promises an array.
	if out[1].Samples == nil {
		t.Fatal("a column with no samples serialized as null")
	}
}

// nonFieldTargets are the advertised targets that write nothing to the record,
// each with the reason it is not held to the round-trip rule.
//
// A waiver rather than a plain map, so exempting a target is a deliberate entry
// with a reason held to a standard, and an entry that stops matching is reported
// rather than left behind. A target absent from here must reach both the create
// and the update input.
var nonFieldTargets = gatekit.Waive(map[string]string{
	csvTargetID:     "names the RECORD the row is — a selector, not a value",
	csvEmployerName: "names an EDGE from the record to a company — written as a relationship, not a column",
})

// assertNonFieldTarget holds one exemption to what it promises: accepted as a
// column, and reaching neither write input.
func assertNonFieldTarget(t *testing.T, object, target string, targets []string,
	created, patched map[string]bool, writeHarm, absenceHarm string,
) {
	t.Helper()
	if !nonFieldTargets.Waived(t, target) {
		t.Errorf("%s: %q is treated as a non-field target but has no entry in nonFieldTargets saying why", object, target)
	}
	if created[target] || patched[target] {
		t.Errorf("%s: %q reached a write input — %s", object, target, writeHarm)
	}
	if !slices.Contains(targets, target) {
		t.Errorf("%s: %q is not an accepted column, so %s", object, target, absenceHarm)
	}
}

// A report STORED before the line was carried still reads. Its JSON has no
// `line` field, so it decodes as zero — and for a skip the source disclosed,
// the id still spells "line N", which is where the line used to come from.
func TestAStoredReportsSkipStillNamesItsLine(t *testing.T) {
	if got := lineOf(migration.SkippedRow{ExternalID: "line 7"}); got != 7 {
		t.Errorf("line = %d, want 7 — a report written before the line was carried must still read", got)
	}
	// The carried line wins, which is what makes a file with its own key column
	// answer at all: its ids are real, and the old parse returned 0 for them.
	if got := lineOf(migration.SkippedRow{ExternalID: "b@x.test", Line: 4}); got != 4 {
		t.Errorf("line = %d, want the carried 4", got)
	}
	// The id is not always one of ours — a mirror source's rows carry the
	// incumbent's — so the fallback reads the WHOLE id or nothing. Unanchored
	// it reported "line 7 of the export" as line 7, which is a number the
	// reader would go looking for and not find.
	for _, foreign := range []string{"line 7 of the export", "outline 7", "line seven", "line"} {
		if got := lineOf(migration.SkippedRow{ExternalID: foreign}); got != 0 {
			t.Errorf("lineOf(%q) = %d, want 0 — that id is not this source's disclosure", foreign, got)
		}
	}
}

// A file whose key column legitimately holds the text a source disclosure uses
// keeps both rows in the report.
//
// The dedup folds one row's dry-run skip onto its commit skip, keyed on the id.
// A row the SOURCE could not identify is disclosed as `line 7`, so a real row
// whose key value is that text collided with it and one of the two vanished
// from a report whose whole job is to say which rows a person must go fix.
func TestARowKeyedLikeADisclosureIsNotFoldedOntoIt(t *testing.T) {
	var issues []crmcontracts.ImportRowIssue
	seen := map[string]bool{}
	added := appendIssues(&issues, seen, []migration.SkippedRow{
		// What the source writes for a row with no key at all, on line 7.
		{ExternalID: "line 7", Line: 7, Reason: "no key"},
		// A row on line 12 whose key column really does hold that text.
		{ExternalID: "line 7", Line: 12, Reason: "not-a-uuid"},
	})
	if added != 2 || len(issues) != 2 {
		t.Fatalf("issues = %+v (added %d), want both rows — they are two rows a person must go fix",
			issues, added)
	}
	// And the fold that DOES belong still happens: one row skipped twice.
	added = appendIssues(&issues, seen, []migration.SkippedRow{{ExternalID: "line 7", Line: 7, Reason: "no key"}})
	if added != 0 {
		t.Errorf("the same row was reported twice; the dry run's skip and the commit's are one row")
	}
}

// A resumed run keeps every refusal it recorded.
//
// A finished report can carry more outcomes than the file has rows, from two
// causes. A stale dry-run skip whose row then committed as a create is one, and
// dropping the skip is right. A resumed run double-counting after a checkpoint
// that did not persist is the other, and there the surplus is in
// created/updated — taking it out of `skipped` erases a row somebody has to go
// fix.
//
// Only a resume can cause the second, so the ATTEMPT count tells them apart.
// The object count cannot: attempts fold by class, and a CSV import is always
// exactly one class.
//
// TWO walks is the ordinary committed run — the dry run and the commit — and
// that is exactly where the stale skip lives. Three or more is a resume.
func TestAResumedRunKeepsItsRefusals(t *testing.T) {
	skipped := []migration.SkippedRow{{ExternalID: "a@x.test", Line: 2, Reason: "not-a-uuid"}}
	// Three rows read; a resume counted two of them twice, so the four
	// dispositions sum to five.
	report := migration.Report{
		Attempts: 3,
		Objects: []migration.ObjectReport{{
			Object: migration.ObjectLead, MirrorCount: 3, Created: 4, Skipped: skipped,
		}},
	}
	out := toContractImportReport(migration.Run{Status: migration.StatusComplete, Report: &report})
	if out.Disposition.Skipped != 1 {
		t.Errorf("skipped = %d, want the one refusal the run recorded — a resume's surplus is in "+
			"created, and taking it out of skipped drops a row somebody has to go fix",
			out.Disposition.Skipped)
	}
	if len(out.Issues) != 1 {
		t.Errorf("issues = %+v, want the refusal named", out.Issues)
	}

	// The ordinary committed run is unchanged: dry run plus commit is two walks,
	// and there the surplus IS a stale skip — the dry run refused a row the
	// commit then created.
	for _, walks := range []int{0, 1, 2} {
		ordinary := report
		ordinary.Attempts = walks
		out = toContractImportReport(migration.Run{Status: migration.StatusComplete, Report: &ordinary})
		if out.Disposition.Skipped != 0 {
			t.Errorf("skipped = %d on a %d-walk report, want the stale skip dropped",
				out.Disposition.Skipped, walks)
		}
	}
}

// The context tag rides on the stored MAPPING rather than on the create call,
// for the reason on_duplicate does: the commit happens on a later request, and
// a decision that lived only in the create body would be gone by the time
// anything wrote a row.
func TestMappingCarriesTheContextTagToTheCommit(t *testing.T) {
	t.Parallel()

	word := openapi_types.UUID(ids.NewV7())
	mapping, err := mappingFrom(migration.ObjectOrganization, crmcontracts.CreateImportRunRequest{
		Mapping:      map[string]string{"Name": "display_name"},
		ContextTagId: &word,
	})
	if err != nil {
		t.Fatalf("mappingFrom: %v", err)
	}
	if mapping.ContextTag != word.String() {
		t.Fatalf("the run's word did not reach the stored mapping: got %q, want %q",
			mapping.ContextTag, word.String())
	}
}

// A body naming the zero id names no word. Storing it would make every created
// record fail its apply at COMMIT time, long after the request that could have
// said so — so it is refused where the caller can still fix it.
func TestMappingRefusesTheZeroContextTag(t *testing.T) {
	t.Parallel()

	var zero openapi_types.UUID
	_, err := mappingFrom(migration.ObjectOrganization, crmcontracts.CreateImportRunRequest{
		Mapping:      map[string]string{"Name": "display_name"},
		ContextTagId: &zero,
	})
	if err == nil {
		t.Fatal("the zero uuid was accepted as a tag; every created record would fail its apply at commit")
	}
}

// A run that named no word files nothing, and must not be refused for it.
func TestMappingWithoutAContextTagFilesNothing(t *testing.T) {
	t.Parallel()

	mapping, err := mappingFrom(migration.ObjectOrganization, crmcontracts.CreateImportRunRequest{
		Mapping: map[string]string{"Name": "display_name"},
	})
	if err != nil {
		t.Fatalf("mappingFrom: %v", err)
	}
	if mapping.ContextTag != "" {
		t.Fatalf("a run naming no word carried one anyway: %q", mapping.ContextTag)
	}
}

// The writer files only the objects the tag surface carries. It is derived from
// the canonical record vocabulary rather than restated, so an object added to
// either side does not need this switch remembered — and the case that would
// otherwise rot silently is an importable object that is NOT taggable.
func TestTaggableObjectFollowsTheRecordVocabulary(t *testing.T) {
	t.Parallel()

	for _, object := range []string{
		migration.ObjectOrganization,
		migration.ObjectPerson,
		migration.ObjectLead,
	} {
		if _, ok := taggableObjectOf(object); !ok {
			t.Fatalf("%q is importable and in the record vocabulary, but files under no tag", object)
		}
	}
	if _, ok := taggableObjectOf("activity"); ok {
		t.Fatal("an object outside the record vocabulary was accepted as taggable")
	}
}

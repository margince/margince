// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// `setting` carries neither row-level security nor a row-scope clause. The
// migration that created it says so in its own comment, and the schema is the
// standing evidence: the table declares no workspace_id and no policy, so the
// platform/auth object gate at the reader or writer is the only control on it,
// and nothing backstops it.
//
// The WRITE half of that already has a rail — tableownership_test.go refuses a
// package writing a table it does not own, and `setting` is owned by
// internal/platform/settings. The READ half had none, and that gate's header
// ratifies skipping SELECTs on the grounds that a statement's own workspace
// predicate and the platform/auth row-scope clauses already govern them.
// Neither of those reaches this table, so the exemption does not cover it.
//
// So: every raw SQL read of `setting` outside the owning package carries a
// verdict here, or it is a finding. The verdicts are not a formality — the
// store offers a real seam for each honest case:
//
//   - Get/GetTx take the installation_settings object gate. Every read that
//     ANSWERS a value to a caller goes through one of them.
//   - ApplyTx is the ungated machinery read, and it is enforced rather than
//     asked politely: only an entry declared MachineryApplied at Define time
//     is readable through it.
//
// A raw statement bypasses both. Two shapes legitimately have to: a read that
// runs BEFORE a principal exists (so there is nothing to gate against), and a
// read embedded in a larger statement as a JOIN or a subquery, where a Go-level
// call would be a second round trip inside somebody else's transaction. Each
// site below says which it is, and what it costs.
//
// What this gate stops is the third shape — a raw read landing on a path where
// a principal DOES exist and the gate should have applied. Every such read is
// indistinguishable from one that was considered, which is the whole reason the
// verdict has to be written down.

import (
	"go/ast"
	"path"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// settingTable is this census's subject. The pattern below and the failure
// messages both read it, so a change of subject reaches both at once.
const settingTable = "setting"

// settingOwner is the package the table belongs to — the owner tableOwners
// declares for the write half, restated here rather than imported from it: the
// two gates ask different questions of the same package, and sharing the name
// would suggest a coupling that does not exist.
const settingOwner = "internal/platform/settings"

// settingReadLiteral matches a SQL literal reading the table by name.
//
// gatekit's pattern rather than one spelled here: it anchors on FROM/JOIN and
// requires a delimiter after the name, so `capture_settings` and
// `installation_settings` — both real tables in this schema — do not match, and
// its boundary cases are tested where it lives. A matcher that stops seeing
// this tree's SQL finds nothing to object to and reads exactly like a clean
// tree, which is what settingReadFloor is for.
var settingReadLiteral = gatekit.TableReadPattern(settingTable)

// rawSettingReads ratifies every raw read of the table outside its owner.
//
// Keyed `<path>:<function>:<statement>`, and the statement is in the key
// because the two shorter forms both let a read go unjudged. Keyed by path, one
// entry ratifies a whole file. Keyed by path and function — which is where this
// started — a SECOND read added inside an already-ratified function inherits the
// first one's verdict: gatekit attributes every matching literal in a body to
// that one function name, so the new read is admitted by a rationale written
// about a different statement, and neither the finding path nor the staleness
// sweep has anything to report. That is the same hole in "considered" the header
// says this gate exists to close, one level in.
//
// The cost is that editing a statement's first line invalidates its entry, and
// the gate then reports the waiver as matching nothing. That is the intended
// direction for this table: a raw read whose text changed is a read whose cost
// somebody should look at again.
var rawSettingReads = gatekit.Waive(map[string]string{
	"internal/modules/identity/service.go:Authenticate:SELECT s.id, u.id, u.email, u.display_name, u.seat_type, u.must_change_password, …":   "the session read, which runs BEFORE a principal exists — it is the query that builds one. Asking the installation_settings object gate here would be circular: auth.Require has no actor to judge, so the answer would be a refusal for every login rather than a control. The setting rides the session read as a subquery rather than a second statement for the reason the locale and zone beside it do: /me needs the label before the SPA draws anything, and a second round trip would paint the wrong one first. What it discloses is the installation label the sign-in page already shows an anonymous visitor",
	"internal/modules/identity/settingsentry.go:InstallationNameOf:SELECT coalesce((SELECT value #>> '{}' FROM setting WHERE key = $1), '')": "the installation's label, for the two surfaces that have no seat to gate: the login that has not built a principal yet, and the public preference page, which has no session at all. Coalesced to the empty string rather than erroring, because a display label missing is a misconfiguration and must not turn a working page into a 500. What it discloses is the installation's own name, not tenant data",
	"internal/modules/deals/basecurrencyfreeze.go:ratesPricedAgainstBase:SELECT value #>> '{}' FROM setting WHERE key = $1":                  "the freeze probe, which runs INSIDE the settings write's own transaction and before the new value is stored. The gated readers answer callers asking what the value IS; this is the write asking what it is about to replace, so Get/GetTx would take the object gate a second time on a caller the write already gated and read through a store whose transaction this is. What it reads is the outgoing value of the setting being written, and it is compared against nothing but the rate sheet",

	"internal/compose/vcardingest.go:asMailboxGrantor:SELECT cc.user_id, cc.provider …":                                                "the per-mailbox capture switch, as a subquery of the mailbox-grant SELECT. It is read from the row rather than through the store because that store gates on auth.Require and this read is what DECIDES whether to build a principal — it cannot use the one it gates. capture.SignatureEnrich is declared MachineryApplied for exactly this shape; what is not available is a Go-level call, because the value is the COALESCE fallback for a per-connection column in the same statement. What it discloses is whether a mailbox owner agreed to contact extraction",
	"internal/compose/weekly/weeklycompare.go:baseCurrency:SELECT (value #>> '{}')::text FROM setting …":                               "the installation's reporting currency, read inside a weekly-plan read that is already gated on its own object. Same ground leadSLAPolicy states: the currency is an INPUT to a figure the caller is separately authorised for, and a seat that may read its own week must not also need installation_settings:read to see the money in it. What reaches the caller is a converted total over their own rows, never the setting",
	"internal/compose/datasweep.go:orphanedSecretRefs:SELECT trim(both '\"' from value::text) AS ref FROM setting WHERE key = ANY($1)": "the vault refs the two DEPLOYMENT credentials are sealed under, as one arm of the reset's orphan census. The census asks vault_secret which sealed rows nothing references any more, and it has to ask in ONE statement: a ref read separately and compared in Go is a snapshot, and a snapshot is the exact defect this census replaced. Not through GetTx, and the reason is the direction it fails in — GetTx takes the entry's object gate, so an admin without license:read would fail the census, and a census that cannot complete must refuse rather than delete, which turns a missing grant into a reset that cannot finish. What it reads is a ref ADDRESS, never a setting's meaning, and it reads it only to keep the credential ALIVE: without this arm the licence is purged off a running installation",
	"internal/compose/analyticssharehandlers.go:snapshotFrame:SELECT s.period_start, s.period_end, s.base_currency, s.taken_at, …":     "the display timezone a frozen forecast is rendered in, LEFT JOINed into the snapshot read rather than fetched separately — the frame is one row, and a second statement for one label would be a round trip inside the share's transaction. The surface is a tokenized share link with no seat at all, which is the same ground the public preference page stands on: there is no principal to gate against. What it discloses is the installation's timezone, and it is coalesced to UTC when unset",
})

// settingReadFloor is set below the live count of raw reads so ordinary
// refactoring does not trip it. It catches the extractor going to zero — which
// reports PASS over a tree it can no longer see — and never a read being
// deleted.
const settingReadFloor = 5

// settingReaderScope collects the non-test, non-generated files under
// internal/ that read the table by name, the owning package included: the
// owner's own reads are what prove the extractor still works.
var settingReaderScope = gatekit.Scope{
	Roots:   []string{"internal"},
	Subject: readsSettingTable,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func readsSettingTable(filePath string, file *ast.File) bool {
	return gatekit.FileReadsTable(filePath, file, settingReadLiteral)
}

// unratifiedSettingReads returns one finding per raw read outside the owner
// that no waiver ratifies, and how many reads were seen in total.
//
// It RETURNS its findings and takes the waiver set rather than reaching for the
// package-level one, so the gate's decision can be exercised against a
// synthetic tree without failing the test that exercises it — and without
// marking a real entry matched, which would satisfy the staleness sweep for the
// entry the probe happened to name.
func unratifiedSettingReads(t testing.TB, waivers *gatekit.Waivers[string], files []gatekit.ParsedFile) (findings []string, seen int) {
	t.Helper()
	for _, parsed := range files {
		for _, read := range gatekit.TableReads(parsed.File, settingReadLiteral) {
			seen++
			if path.Dir(parsed.Path) == settingOwner || strings.HasPrefix(parsed.Path, settingOwner+"/") {
				continue
			}
			subject := settingReadSubject(parsed.Path, read)
			if waivers.Waived(t, subject) {
				continue
			}
			findings = append(findings, subject+" reads the "+settingTable+" table in raw SQL.\n"+
				"  This table has no row-level security and no row-scope clause, so the object gate at "+
				"the reader is the only control on it — and a raw statement asks none.\n"+
				"  Read it through settings.Get/GetTx, which take the gate, or through settings.ApplyTx "+
				"with the entry declared MachineryApplied, which is the enforced ungated seam.\n"+
				"  If neither fits — no principal exists yet, or the read is a JOIN inside a larger "+
				"statement — ratify THIS read in rawSettingReads[\""+subject+"\"] with what it costs.\n"+
				"  The read: "+gatekit.FirstLineOf(read.SQL))
		}
	}
	return findings, seen
}

// settingReadSubject names one read: where it is, whose body it sits in, and
// WHICH statement it is. The last part is what keeps two reads in one function
// from sharing a verdict; the SQL is collapsed to its first line so the key
// stays something a reader can match against the code by eye.
func settingReadSubject(filePath string, read gatekit.TableRead) string {
	return filePath + ":" + read.Function + ":" + gatekit.FirstLineOf(read.SQL)
}

func TestEveryRawReaderOfTheSettingTableCarriesAVerdict(t *testing.T) {
	t.Parallel()
	defer rawSettingReads.AssertAllMatched(t)

	findings, seen := unratifiedSettingReads(t, rawSettingReads, settingReaderScope.Files(t))
	for _, finding := range findings {
		t.Error(finding)
	}
	if seen < settingReadFloor {
		t.Fatalf("the census saw %d reads of the %s table, below the %d floor — the literal "+
			"extractor is reading a tree it no longer recognises, and that reports PASS exactly "+
			"like a tree with nothing to find", seen, settingTable, settingReadFloor)
	}
	t.Logf("%s reads: %d seen, %d ratified outside %s", settingTable, seen, len(rawSettingReads.Subjects()), settingOwner)
}

// The census refuses an unratified read, which is the half that has to keep
// working and the half a green tree can never demonstrate.
//
// The waiver set is a throwaway rather than rawSettingReads: querying the real
// one MARKS the entry matched for the whole package, which would quietly
// satisfy AssertAllMatched for whichever entry this case named — the one
// staleness a stale-waiver gate exists to report.
func TestAnUnratifiedSettingReadIsAFinding(t *testing.T) {
	t.Parallel()
	files := settingReaderScope.Files(t)
	empty := gatekit.Waive(map[string]string{})

	findings, seen := unratifiedSettingReads(t, empty, files)
	if seen == 0 {
		t.Fatal("the census saw no reads at all, so this case proves nothing about what it refuses")
	}
	if len(findings) == 0 {
		t.Fatal("every raw read outside the owner passed with an EMPTY waiver set — the gate " +
			"ratifies nothing and refuses nothing")
	}
	// Named, so the case proves WHICH read is refused rather than that
	// something somewhere was.
	const known = "internal/compose/weekly/weeklycompare.go:baseCurrency"
	if !slicesContainsPrefix(findings, known) {
		t.Errorf("the refusal set does not name %s; repoint this case at a live raw read", known)
	}
}

// The owner's own reads are never findings: the gate is about who reaches PAST
// the store, and the store reaching its own table is what it is for.
//
// The path is spelled out rather than taken from settingOwner, which is the
// value under test here. Derived from it, this case answers a question about
// itself: point settingOwner at a directory that does not exist and the
// exclusion stops excluding, every owner read becomes a finding, and a test
// looking for findings under the SAME wrong prefix still sees none and passes.
// Measured, not feared — that mutation survived until this literal replaced it.
const settingStorePath = "internal/platform/settings/store.go"

func TestTheSettingsPackagesOwnReadsAreNotFindings(t *testing.T) {
	t.Parallel()
	empty := gatekit.Waive(map[string]string{})
	files := settingReaderScope.Files(t)

	if !slicesContainsPrefix(pathsOf(files), settingStorePath) {
		t.Fatalf("%s is not in the census corpus, so this case proves nothing about what the "+
			"owner exclusion excludes", settingStorePath)
	}
	findings, _ := unratifiedSettingReads(t, empty, files)
	for _, finding := range findings {
		if strings.HasPrefix(finding, settingStorePath) {
			t.Errorf("the gate reported the owning store's own read: %s", finding)
		}
	}
}

func pathsOf(files []gatekit.ParsedFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func slicesContainsPrefix(findings []string, prefix string) bool {
	for _, f := range findings {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// A SECOND raw read inside an already-ratified function is still a finding.
//
// This is the case the key's shape exists for, and the one the shorter key
// could not have. gatekit attributes every matching literal in a body to that
// body's function name, so keyed `<path>:<function>` a new read lands on the
// entry the first one earned — admitted by a rationale written about a
// different statement, with nothing for the finding path or the staleness
// sweep to report.
//
// A fixture rather than the tree, because the tree has no such function: the
// defect this refuses is one somebody adds tomorrow, and a case that could only
// be written after the mistake was made is not a guard.
func TestASecondReadInARatifiedFunctionIsNotCoveredByTheFirst(t *testing.T) {
	t.Parallel()
	const twoReads = `package settingfixture

func loadTwo(ctx context.Context, tx pgx.Tx) error {
	if err := tx.QueryRow(ctx, ` + "`SELECT value FROM setting WHERE key = 'a'`" + `).Scan(&a); err != nil {
		return err
	}
	return tx.QueryRow(ctx, ` + "`SELECT value FROM setting WHERE key = 'b'`" + `).Scan(&b)
}
`
	const fixturePath = "internal/modules/fixture/two.go"
	files := []gatekit.ParsedFile{{Path: fixturePath, File: parseGateFixture(t, twoReads)}}

	// The shape this replaced, first, because it is the defect rather than the
	// fix: an entry keyed `<path>:<function>` covers BOTH reads, so a second
	// one is admitted by a rationale written about the first and the census
	// reports nothing at all.
	byFunction := gatekit.Waive(map[string]string{
		fixturePath + ":loadTwo": "the older key shape, which names no statement. It costs the second read's verdict",
	})
	blanket, seenBlanket := unratifiedSettingReads(t, byFunction, files)
	if seenBlanket != 2 {
		t.Fatalf("the census saw %d reads in the fixture, want 2 — it is not seeing the shape this case is about", seenBlanket)
	}
	if len(blanket) != 2 {
		t.Errorf("a verdict naming only the function ratified %d of the 2 reads — a key that does "+
			"not name the statement admits every later read in the same body", 2-len(blanket))
	}

	// Ratifying the FIRST read only, by the key the census mints for it.
	first := gatekit.Waive(map[string]string{
		fixturePath + ":loadTwo:SELECT value FROM setting WHERE key = 'a'": "the first read, ratified. It costs nothing: this is a fixture",
	})
	findings, _ := unratifiedSettingReads(t, first, files)
	if len(findings) != 1 {
		t.Fatalf("findings = %d, want exactly 1: the first read is ratified and the second is not.\n%v",
			len(findings), findings)
	}
	if !strings.Contains(findings[0], "key = 'b'") {
		t.Errorf("the finding names the wrong read: %s", findings[0])
	}
}

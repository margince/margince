// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// enumsync_test.go holds the CHECK on approval.target_entity_type equal to
// targettype.Type. That does not say whether a writer names a type the list
// leaves out. These gates hold every writer of an approval target, a notice
// target and a metric source inside its list. An unreadable site is waived by
// name. They also name the discriminator columns that stay free text.

import (
	"bufio"
	"go/ast"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	approvalTargetTypes  = "internal/shared/kernel/targettype.Type"
	noticeOwnTargetTypes = "internal/modules/notices.OwnTargetType"
	reportingSourceTypes = "internal/modules/reporting.SourceType"
	noticeTargetColumn   = "notice.target_type"
)

// Floors on writer sites, resolved or not. A scan that reads fewer has stopped
// reading the tree. Each sits just under the count measured when it was written.
const (
	stagerSiteFloor = 65
	noticeSiteFloor = 25
	factSiteFloor   = 24
)

var extensionTable = regexp.MustCompile(`^ext_[a-z0-9]+(_[a-z0-9]+)*$`)

// stagerScan reads the structs a staging is written through, which carry the
// target type under TargetType and CoTargetType.
var stagerScan = stringScan{
	literals:   map[string]bool{"approvals.StageInput": true, "agents.StageInfo": true, "automation.StageRequest": true},
	keys:       map[string]bool{"TargetType": true, "CoTargetType": true},
	stageCalls: true,
}

// noticeTargetScan is where a notice's target type is written, and the privacy
// review source whose kind becomes one.
var noticeTargetScan = stringScan{
	literals: map[string]bool{"notices.Target": true, "privacy.ErasureReviewSource": true},
	keys:     map[string]bool{"Type": true, "Kind": true},
}

// factSourceScan is where a metric names its source: the fact literal, and the
// readers every metric reads its rows through.
var factSourceScan = stringScan{
	literals: map[string]bool{"reporting.Fact": true},
	keys:     map[string]bool{"SourceType": true},
	calls: map[string]int{
		"readGroupedFacts": 4, "readGroupedFactsVia": 4, "readOutcomeFacts": 4, "scanOutcomeFacts": 3, "readColleagueWork": 4,
	},
}

func TestEveryApprovalStagerNamesAnApprovalTargetType(t *testing.T) {
	t.Parallel()
	_, typed := vocabularyCensusInputs(t)
	declared := typed[approvalTargetTypes]
	if len(declared) == 0 {
		t.Fatalf("no %s constants found: the derivation broke", approvalTargetTypes)
	}
	for _, source := range []string{"internal/shared/ports/datasource.EntityType", "internal/compose.agentRecordType"} {
		if len(typed[source]) == 0 {
			t.Fatalf("no %s constants found: the derivation broke", source)
		}
		for _, value := range typed[source] {
			if !slices.Contains(declared, value) {
				t.Errorf("%s holds %q, which targettype.Type does not. Staging it would fail the approval "+
					"CHECK. Add it to targettype.Type and to the CHECK together", source, value)
			}
		}
	}
	sites := scanSites(t, stagerScan)
	if len(sites) < stagerSiteFloor {
		t.Fatalf("the scan read %d staging sites, floor %d: it has stopped reading the tree", len(sites), stagerSiteFloor)
	}
	resolved, unresolved := scanValues(sites)
	t.Logf("%d staging sites, %d distinct types, %d sites hold a value read at run time", len(sites), len(resolved), len(unresolved))
	for value, file := range resolved {
		if !slices.Contains(declared, value) && !extensionTable.MatchString(value) {
			t.Errorf("%s stages target type %q, which targettype.Type does not hold", file, value)
		}
	}
}

// freeTextDiscriminators are the polymorphic type columns that keep no CHECK.
// Each is a log of what a thing was about when it happened. A closed list would
// refuse the row on the day a new kind of thing appears.
var freeTextDiscriminators = gatekit.Waive(map[string]string{
	"audit_log.entity_type":        "the audit log records every entity type the product has ever audited, including ones since removed, and a refused insert would drop the audit row",
	"ai_call.subject_type":         "the AI call log records what a model call was about; the row is kept after the subject and its kind are gone",
	"ai_task_run.subject_type":     "the AI run log records what a run was about; the row is kept after the subject and its kind are gone",
	"webhook_delivery.entity_type": "the webhook log records what a delivery was about, for every event type a subscriber was ever sent",
	"webhook_delivery.event_type":  "the webhook log records the event name as sent; a new event type must not make its delivery row fail",
})

var catalogTextColumnLine = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*\.[a-z_][a-z0-9_]*) text\b`)

func TestAFreeTextDiscriminatorHasNoVocabularyCheck(t *testing.T) {
	t.Parallel()
	raw := headCatalog(t)
	textColumns := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if m := catalogTextColumnLine.FindStringSubmatch(scanner.Text()); m != nil {
			textColumns[m[1]] = true
		}
	}
	vocabularies, err := catalogVocabularies()
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range freeTextDiscriminators.Subjects() {
		freeTextDiscriminators.Waived(t, column)
		if !textColumns[column] {
			t.Errorf("%s is not a text column in %s: remove its entry", column, headCatalogPath)
		}
		if _, hasCheck := vocabularies[column]; hasCheck {
			t.Errorf("%s now carries a vocabulary CHECK: remove its entry, and bind it in enumBindings", column)
		}
	}
	freeTextDiscriminators.AssertAllMatched(t)
}

// privacyReviewSQL is the SQL that names the kinds of retained content a privacy
// review flags. Each kind becomes a notice's target type.
const (
	privacyReviewFile  = "internal/modules/privacy/erasure_manualreview.go"
	privacyReviewConst = "manualMentionSourcesSQL"
)

var selectedLiteral = regexp.MustCompile(`SELECT '([a-z_]+)'`)

// sqlSelectedKinds reads the kinds a SQL constant selects as string literals.
func sqlSelectedKinds(t *testing.T, file, constName string) []string {
	t.Helper()
	parsed, err := gatekit.ParseFile(file, 0)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, text := range gatekit.StringConstantValues(t, []*ast.File{parsed})[constName] {
		for _, m := range selectedLiteral.FindAllStringSubmatch(text, -1) {
			kinds = append(kinds, m[1])
		}
	}
	return kinds
}

// noticeSiteWaivers are the notice target sites that hold a value the scan cannot
// read, with where the value comes from.
var noticeSiteWaivers = gatekit.Waive(map[string]string{
	"internal/compose/approvalnotify.go: staged.TargetEntityType":      "an approval's own target type, which the approval staging refuses unless targettype.Type holds it",
	"internal/compose/dealownernotify.go: request.TargetType":          "an approval's own target type, which the approval staging refuses unless targettype.Type holds it",
	"internal/compose/editaccess/effect.go: ask.RecordType":            "the record type of an edit-access request, one of the contract's five record types, all of them in targettype.Type",
	"internal/compose/extnotice.go: noticeTargetType(in.Record.Type)":  "a record type an extension sends a notice about, or its declared kind's object; the test below reads extension.NoticeRecordTypes, and notices.requireKnownTarget refuses anything else",
	"internal/compose/noticesseam.go: string(target.Type)":             "a datasource.EntityType, every value of which TestEveryApprovalStagerNamesAnApprovalTargetType holds inside targettype.Type",
	"internal/compose/notificationdigestrender.go: targetType":         "a key of a lookup built from a stored notice's own target type; it writes no notice",
	"internal/compose/ownershiptransfernotice.go: transfer.RecordType": "company or contact, the two record types an ownership transfer moves",
	"internal/compose/privacyreview/review.go: source.Kind":            "the kind of a privacy.ErasureReviewSource, read by the scan of that literal and by the SQL kinds this test reads",
	"internal/compose/receiptalarm.go: missed.SubjectType":             "contact or lead, the subject types a missed opt-out receipt names",
	"internal/modules/notices/centre.go: *targetType":                  "reads a stored notice target; it writes no notice",
	"internal/modules/notices/digestclaim.go: *targetType":             "reads a stored notice target; it writes no notice",
	"internal/modules/notices/emailclaim.go: *targetType":              "reads a stored notice target; it writes no notice",
	"internal/modules/notices/replay.go: *storedType":                  "reads a stored notice target; it writes no notice",
	"internal/modules/notices/store.go: *targetType":                   "reads a stored notice target; the insert above it is refused by requireKnownTarget",
})

// factSiteWaivers are the metric source sites that hold a value the scan cannot read.
var factSiteWaivers = gatekit.Waive(map[string]string{
	"internal/compose/metriccolleaguework.go: source": "a parameter of readColleagueWork; each caller's argument is a site of this scan",
	"internal/compose/metricevents.go: source":        "a parameter of the outcome readers; each caller's argument is a site of this scan",
	"internal/compose/metricleads.go: source":         "a parameter of the grouped readers; each caller's argument is a site of this scan",
})

// A notice points at anything an approval can name, or at one of the things only
// a notice has. The CHECK is held to that union, so neither list is typed a second
// time and a value dropped from either fails here. Every site that writes a target
// type is read; one the scan cannot read is waived with where its value comes from.
func TestNoticeTargetTypeIsTheApprovalListAndTheNoticeOwn(t *testing.T) {
	t.Parallel()
	vocabularies, typed := vocabularyCensusInputs(t)
	if len(typed[approvalTargetTypes]) == 0 || len(typed[noticeOwnTargetTypes]) == 0 {
		t.Fatalf("a source list is empty (%d approval, %d notice own): the derivation broke",
			len(typed[approvalTargetTypes]), len(typed[noticeOwnTargetTypes]))
	}
	want := asSet(append(slices.Clone(typed[approvalTargetTypes]), typed[noticeOwnTargetTypes]...))
	got, ok := vocabularies[noticeTargetColumn]
	if !ok {
		t.Fatalf("no CHECK vocabulary found for %s", noticeTargetColumn)
	}
	if setKey(asSet(got)) != setKey(want) {
		t.Errorf("%s: the CHECK admits %v but targettype.Type plus notices.OwnTargetType hold %v. Change both together",
			noticeTargetColumn, got, want)
	}

	sites := scanSites(t, noticeTargetScan)
	if len(sites) < noticeSiteFloor {
		t.Fatalf("the scan read %d notice target sites, floor %d: it has stopped reading the tree", len(sites), noticeSiteFloor)
	}
	resolved, unresolved := scanValues(sites)
	for _, site := range unresolved {
		if !noticeSiteWaivers.Waived(t, site.key()) {
			t.Errorf("%s holds a notice target type the scan cannot read. Resolve it to a constant, or waive it "+
				"in noticeSiteWaivers with where its value comes from", site.key())
		}
	}
	noticeSiteWaivers.AssertAllMatched(t)

	// Values no Go literal holds: the SQL that names a privacy review's kinds, and
	// the record types an extension may send a notice about.
	for _, kind := range sqlSelectedKinds(t, privacyReviewFile, privacyReviewConst) {
		resolved[kind] = privacyReviewFile
	}
	if len(sqlSelectedKinds(t, privacyReviewFile, privacyReviewConst)) == 0 {
		t.Fatalf("no kinds read from %s in %s: the SQL moved", privacyReviewConst, privacyReviewFile)
	}
	for _, recordType := range extension.NoticeRecordTypes() {
		resolved[recordType] = "pkg/extension (NoticeRecordTypes)"
	}
	for value, file := range resolved {
		if !slices.Contains(want, value) && !extensionTable.MatchString(value) {
			t.Errorf("%s writes notice target type %q, which the CHECK refuses", file, value)
		}
	}
}

func TestEveryMetricSourceNamesAReportingSourceType(t *testing.T) {
	t.Parallel()
	_, typed := vocabularyCensusInputs(t)
	declared := typed[reportingSourceTypes]
	if len(declared) == 0 {
		t.Fatalf("no %s constants found: the derivation broke", reportingSourceTypes)
	}
	sites := scanSites(t, factSourceScan)
	if len(sites) < factSiteFloor {
		t.Fatalf("the scan read %d metric source sites, floor %d: it has stopped reading the tree", len(sites), factSiteFloor)
	}
	resolved, unresolved := scanValues(sites)
	for _, site := range unresolved {
		if !factSiteWaivers.Waived(t, site.key()) {
			t.Errorf("%s holds a metric source the scan cannot read. Resolve it to a constant, or waive it "+
				"in factSiteWaivers with where its value comes from", site.key())
		}
	}
	factSiteWaivers.AssertAllMatched(t)
	for value, file := range resolved {
		if !slices.Contains(declared, value) {
			t.Errorf("%s names metric source %q, which reporting.SourceType does not hold", file, value)
		}
	}
}

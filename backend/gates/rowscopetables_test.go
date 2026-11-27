// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// Which table a row-scope call bounds, and which column names a reference to
// one.
//
// The obligation used to be table-agnostic: a function that projected an
// `company_id` satisfied it by reaching a probe over `deal`. That is not a
// weaker check, it is a different one — #1876's read named a company and
// scoped a deal, and a gate counting probes rather than matching them would
// have gone green over it and then certified it.
//
// And the columns it looked for were named after their table. An FK named for
// its ROLE — `partner_company_id` was one of the three references #1876 fixed —
// escaped entirely: the extractor read the column's own words as a table name,
// found no such table, and dropped the site with no census entry and no waiver.

import (
	"fmt"
	"go/ast"
	"go/token"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// anyTable is the scope a spelling applies when it cannot be attributed to one
// table. It satisfies a reference to any of them, which is the honest reading:
// ScopeClause and OwnerPredicate render over whatever alias the caller gives
// them, so the table is the caller's and this gate cannot see it.
const anyTable = "*"

// scopeSpellingTable says which table each row-scope spelling bounds.
//
// Three shapes, and the difference is in platform/auth's own signatures:
//
//   - a table NAMED BY ARGUMENT — EnsureVisible(ctx, tx, table, id) — where the
//     value is the string literal at that index, and a non-literal means a
//     caller allowlisting at its own seam, which this gate reads as any table;
//   - a table IMPLIED BY THE NAME — EnsureActivityVisible bounds activities and
//     nothing else, so the name is the answer;
//   - no table at all, which is anyTable above.
//
// TestEveryRowScopeSpellingSaysWhichTableItBounds holds this map against
// rowScopeSpellings in both directions, so a spelling added to one and not the
// other fails rather than defaulting to a silent wildcard.
var scopeSpellingTable = map[string]scopeSpelling{
	"ScopeClause":      {table: anyTable},
	"ScopeClauseFor":   {argument: 1},
	"OwnerPredicate":   {table: anyTable},
	"VisiblePredicate": {argument: 1},

	"EnsureVisible":                 {argument: 2},
	"EnsureVisibleLive":             {argument: 2},
	"EnsureVisibleExisting":         {argument: 2},
	"EnsureVisibleForSubjectRights": {argument: 2},
	// The write-authority family takes the same (ctx, tx, table, id) shape as
	// the visible family it opens with, so the table is in the same position.
	"EnsureWritable":     {argument: 2},
	"EnsureWritableLive": {argument: 2},
	"HoldWritableLive":   {argument: 2},
	"EnsureReadable":     {argument: 2},
	"EnsureChangeable":   {argument: 2},
	"EnsureLinkTarget":   {argument: 2},
	// The attach direction's probe opens with EnsureLinkTarget and adds write
	// authority, so it bounds a reference at least as tightly.
	"EnsureAttachTarget": {argument: 2},
	"VisibleTo":          {argument: 2},
	"VisibleSubset":      {argument: 2},
	"PassingSubset":      {argument: 2},
	// The link target's table is the caller's to name in its own allowlist;
	// this clause renders over the alias it is given.
	"LinkTargetVisibleClause":  {table: anyTable},
	"LinkTargetReadableClause": {table: anyTable},

	"ActivityDiscoverClause":           {table: "activity"},
	"ActivityContentClause":            {table: "activity"},
	"ActivityMessageContentClause":     {table: "activity"},
	"EnsureActivityVisible":            {table: "activity"},
	"EnsureActivityVisibleLive":        {table: "activity"},
	"EnsureActivityContentVisible":     {table: "activity"},
	"EnsureActivityContentVisibleLive": {table: "activity"},
	"ActivityContentSubset":            {table: "activity"},

	"SignalScopeClause":       {table: "signal"},
	"EnsureSignalVisible":     {table: "signal"},
	"EnsureSignalVisibleLive": {table: "signal"},
	"SignalVisibleLiveSubset": {table: "signal"},

	// The edge conjunction bounds the relationship by its ENDPOINTS: each
	// endpoint column is tested with VisiblePredicate over its own table, so a
	// contact or company id projected from an edge that passed it is
	// bounded as surely as one from a direct scope clause. Reading it as
	// "relationship only" would report those projections unscoped and send the
	// next author to add a second clause over a column the conjunction covers.
	"RelationshipEndpointScope": {endpoints: true},
	"EnsureRelationshipVisible": {endpoints: true},
	"EdgeReadScope":             {endpoints: true},
	"RelationshipVisibleSubset": {endpoints: true},
	// The project-company builders bound the edge and the company it names.
	// Only the company is a reference a read hands back.
	"ProjectCompanyPredicates": {table: "company"},
	"ProjectCustomerSQL":       {table: "company"},
	"ProjectCompanyEdgesSQL":   {table: "company"},
}

// scopeSpelling is one spelling's table: fixed, read off an argument, or the
// edge conjunction's — which bounds the relationship AND every endpoint it
// names, so its tables are read out of platform/auth's own endpoint list.
type scopeSpelling struct {
	table string
	// argument is the 0-based index of the parameter naming the table, used
	// when table is empty and endpoints is false.
	argument int
	// endpoints marks the spellings whose clause is the endpoint conjunction:
	// every endpoint column is bounded by VisiblePredicate over its own table,
	// so a contact id projected from an edge that passed it IS bounded.
	endpoints bool
}

func TestEveryRowScopeSpellingSaysWhichTableItBounds(t *testing.T) {
	t.Parallel()
	for spelling := range rowScopeSpellings {
		if _, classified := scopeSpellingTable[spelling]; !classified {
			t.Errorf("%s applies a row scope and this gate cannot say which table it bounds, so every "+
				"reference would count it — classify it in scopeSpellingTable, with anyTable only when "+
				"platform/auth's signature genuinely names no table", spelling)
		}
	}
	for spelling := range scopeSpellingTable {
		if !rowScopeSpellings[spelling] {
			t.Errorf("scopeSpellingTable classifies %s, which is not a row-scope spelling any more — "+
				"a classification outliving its subject is a standing answer to a question nobody asks", spelling)
		}
	}
}

// referenceColumn is a column name that holds a reference to a row-scoped
// record, mapped to the table it points at.
//
// DERIVED FROM THE SCHEMA's own FOREIGN KEY declarations rather than from the
// column's spelling, which is what makes a role-named FK visible: the catalog
// says `partner_company_id` references `company`, where the name says
// `partner_company` and no such table exists.
func referenceColumns(t *testing.T, tables map[string]bool) map[string]string {
	t.Helper()
	catalog := headCatalog(t)
	columns := map[string]string{}
	for _, at := range foreignKeyDeclaration.FindAllStringSubmatch(catalog, -1) {
		column, table := at[1], at[2]
		// Composite keys carry a comma and reference a pair; no reference this
		// census is about is one, and half of a composite is not a reference.
		if strings.Contains(column, ",") || !tables[table] {
			continue
		}
		columns[column] = table
	}
	return columns
}

// foreignKeyDeclaration reads one single-column FK out of the catalog line that
// declares it.
var foreignKeyDeclaration = regexp.MustCompile(`FOREIGN KEY \(([a-z_, ]+)\) REFERENCES ([a-z_]+)\(`)

// The extractor's floor, in the ticket's own words: a regex that silently
// matches nothing reads exactly like a tree with nothing to find.
func TestTheReferenceColumnMapIsDerivedAndReachesTheAliasedNames(t *testing.T) {
	t.Parallel()
	tables := rowScopedTables(t)
	columns := referenceColumns(t, tables)
	if len(columns) < wantMinimumReferenceColumns {
		t.Fatalf("the schema yielded %d reference columns, want at least %d — the catalog's shape moved "+
			"and this census is now reading almost none of it", len(columns), wantMinimumReferenceColumns)
	}
	// The half the old extractor could not see: a column whose own name is not
	// its table's. Without one of these the map is only an expensive spelling
	// of the `<table>_id` regex it replaces.
	aliased := 0
	for column, table := range columns {
		if column != table+"_id" {
			aliased++
		}
	}
	if aliased == 0 {
		t.Error("every reference column is named for its own table, so nothing here covers the case " +
			"this map exists for — an FK named for its ROLE, which the name-derived regex drops silently")
	}
}

// wantMinimumReferenceColumns is far below the real count: it catches a parser
// that has stopped reading the catalog, not a schema that has changed.
const wantMinimumReferenceColumns = 20

// scopedTable resolves which table one row-scope call bounds, from the spelling
// and the arguments it was given.
//
// A table argument that is not a string literal reads as anyTable, and that is
// deliberate: those callers forward a name they allowlist at their own seam
// (link entity types, grant record types, search anchors), and platform/auth
// refuses an unknown one itself. Refusing them here would fail correct code for
// being written the only way it can be.
func scopedTable(spelling string, args []ast.Expr) string {
	classified, known := scopeSpellingTable[spelling]
	if !known {
		// Unclassified is anyTable rather than nothing, so a spelling added to
		// rowScopeSpellings alone weakens this gate instead of breaking the
		// tree — TestEveryRowScopeSpellingSaysWhichTableItBounds is what makes
		// that state fail, and it names the omission where a reader can fix it.
		return anyTable
	}
	if classified.table != "" {
		return classified.table
	}
	if classified.endpoints {
		// One name for the whole conjunction; the caller expands it, because
		// the tables it covers are read from source rather than written here.
		return edgeEndpoints
	}
	if classified.argument >= len(args) {
		return anyTable
	}
	literal, isLiteral := args[classified.argument].(*ast.BasicLit)
	if !isLiteral {
		return anyTable
	}
	table, unquoted := stringConst(literal)
	if !unquoted {
		return anyTable
	}
	return table
}

// edgeEndpoints stands for "the relationship and every endpoint table it
// names". scopedTable returns it as one token and the census expands it, so the
// set stays derived from platform/auth rather than restated here.
const edgeEndpoints = "@edge-endpoints"

// relationshipEndpointTables reads the endpoint tables out of platform/auth's
// own relationshipEndpointColumns, so an endpoint added there widens this
// census without anyone remembering to.
func relationshipEndpointTables(t *testing.T) map[string]bool {
	t.Helper()
	consts := map[string]string{}
	var endpoints []string
	for _, src := range tierFiles(t, rowScopeVocabularyPkg) {
		for _, decl := range src.File.Decls {
			gen, isGen := decl.(*ast.GenDecl)
			if !isGen {
				continue
			}
			for _, spec := range gen.Specs {
				value, isValue := spec.(*ast.ValueSpec)
				if !isValue || len(value.Names) != 1 || len(value.Values) != 1 {
					continue
				}
				if gen.Tok == token.CONST {
					if text, isString := stringConst(value.Values[0]); isString {
						consts[value.Names[0].Name] = text
					}
					continue
				}
				if value.Names[0].Name == "relationshipEndpointColumns" {
					endpoints = endpointTableNames(value.Values[0])
				}
			}
		}
	}
	if len(endpoints) == 0 {
		t.Fatalf("%s declares no relationshipEndpointColumns — teach this gate where the edge's endpoint "+
			"list moved, because reading it as the empty set would report every edge projection unscoped",
			rowScopeVocabularyPkg)
	}
	tables := map[string]bool{"relationship": true}
	for _, name := range endpoints {
		if text, isConst := consts[name]; isConst {
			tables[text] = true
			continue
		}
		tables[name] = true
	}
	return tables
}

// endpointTableNames reads the table of each {column, table} pair as WRITTEN —
// a const by its identifier, a literal by its text.
func endpointTableNames(expr ast.Expr) []string {
	lit, isLit := expr.(*ast.CompositeLit)
	if !isLit {
		return nil
	}
	var names []string
	for _, element := range lit.Elts {
		pair, isPair := element.(*ast.CompositeLit)
		if !isPair || len(pair.Elts) != 2 {
			continue
		}
		switch table := pair.Elts[1].(type) {
		case *ast.Ident:
			names = append(names, table.Name)
		case *ast.BasicLit:
			if text, isString := stringConst(table); isString {
				names = append(names, text)
			}
		}
	}
	return names
}

// The modules tier, read whole. Each function holding an unscoped reference is
// ratified by name, so a new one fails and a fixed one leaves a stale waiver.
func TestTheModulesTierIsMeasuredForTheWideningItIsOwed(t *testing.T) {
	t.Parallel()
	defer modulesTierUnscopedReads.AssertAllMatched(t)
	tables := rowScopedTables(t)
	vocab := referenceVocabulary{
		tables:  tables,
		columns: referenceColumns(t, tables),
		edge:    relationshipEndpointTables(t),
	}
	pkgs, sites := referenceSitesIn(t, modulesTier, vocab)
	if len(sites) < wantMinimumModuleSites {
		t.Fatalf("only %d record-reference reads found in %s, want at least %d — the extractor lost its "+
			"source, and a census that reads a smaller tree reports PASS with nothing to notice",
			len(sites), modulesTier, wantMinimumModuleSites)
	}
	unscoped := 0
	var unscopedSites []string
	for _, site := range sites {
		if reachesRowScope(pkgs[site.dir].visibleTo(site.recv), site.fn, site.table, map[string]bool{}) {
			continue
		}
		unscoped++
		unscopedSites = append(unscopedSites, fmt.Sprintf("%s:%d:%s -> %s", site.dir, site.line, site.fn, site.table))
		// A censused module's reads are ratified once, in the compose census's
		// own list, so the two gates cannot give one read two reasons.
		key := site.dir + ":" + site.fn
		if unscopedReferenceReads.Waived(t, key) || modulesTierUnscopedReads.Waived(t, key) {
			continue
		}
		t.Errorf("%s:%d: %s hands back a %s reference and reaches no row-scope clause for that table. Apply "+
			"auth.ScopeClauseFor or EnsureVisible for %q, or ratify %s in modulesTierUnscopedReads (or "+
			"unscopedReferenceReads, for a censusedModules package) with what bounds the reference instead",
			site.dir, site.line, site.fn, site.table, site.table, key)
	}
	if unscoped > modulesTierUnscopedCeiling {
		t.Errorf("%d of %d record references in %s reach no row scope for the table they name, above the "+
			"recorded %d. A new unscoped statement hands back a reference it never bounded, and in a function "+
			"already waived the waiver does not answer for it: bound it, or say in that waiver why it shares "+
			"the reason and raise the ceiling",
			unscoped, len(sites), modulesTier, modulesTierUnscopedCeiling)
		t.Logf("Unscoped references:\n%s", strings.Join(unscopedSites, "\n"))
	}
}

const (
	modulesTier = "internal/modules"
	// The floor that catches an extractor which has stopped reading, far below
	// the real count.
	wantMinimumModuleSites = 60
	// The waivers name functions, and one function can hold several unscoped
	// statements. This counts statements, so a new one inside a waived function
	// still fails. It may only fall.
	modulesTierUnscopedCeiling = 174
)

// modulesTierUnscopedReads names, per function, what bounds a module read that
// hands back a record reference without a row scope for its table.
//
// Most are passes under a system principal, which holds no seat for a scope to
// narrow to. Narrowing such a pass to one seat would leave every other seat's
// records unrepaired while the pass reported success.
var modulesTierUnscopedReads = gatekit.Waive(map[string]string{
	"internal/modules/activities:DeliveryReportsTx":          "the delivery-report repair pages imported mail under auth.RequireSystem over the whole workspace; each lead id goes only to contacts.ReDeriveLeadFromRemainingTouchesTx, which probes writability",
	"internal/modules/activities:HeldMeetingCounterparties":  "a derived-table body Deal Scout composes under its system principal; the company goes to deals.RecordSuggestionTx, and a rep sees a suggestion only through deals' visibility clause with the company row scope",
	"internal/modules/activities:StampProvenSenderTx":        "the proven-sender stamp runs under auth.RequireSystem and writes the sender onto the stored activity under its row lock; no contact id is returned to a caller",
	"internal/modules/activities:askLeadsToQualifyTx":        "files the qualify task on each lead a held meeting is filed on, inside the write that recorded it held; the lead ids feed that task writer and are returned to no caller",
	"internal/modules/activities:attachAttendeeResponses":    "reads each meeting answer, then keeps a contact's answer only when the contact is in shownContacts, the links the reader already sees on the meeting",
	"internal/modules/activities:backfillParticipants":       "the participant backfill is one INSERT ... SELECT over the workspace's stored mail; contact ids move between tables inside the statement and only a row count is returned",
	"internal/modules/activities:carryMissingParticipantsTx": "copies a folded meeting copy's guests onto the meeting that stays, under a system job or a calendar sync; the contacts move between two rows of one meeting and none is returned",
	"internal/modules/activities:copyEchoLinks":              "an echo's links are copied onto the human send that survives it, inside the fold's own transaction; the company, deal, lead and project ids move between link rows and none is returned",
	"internal/modules/activities:lastTouchCandidateQuery":    "the quiet-account candidate scan, behind auth.RequireReadForReaders; the account arm groups by company to fold its last touch, and the reminder writer applies each record's scope before a card shows",
	"internal/modules/activities:linkedLeads":                "the follow-up auto-resolve consumer asks which leads a captured activity touches, as the event worker; the lead ids select tasks to close and never reach a reader",
	"internal/modules/activities:lockMeetingFacts":           "reads a meeting's participant contact ids only to hash them into the preparation's guest-list fingerprint; changeReadiness has already taken EnsureActivityContentVisibleLive on the meeting, and no id is returned",
	"internal/modules/activities:meetingAttendeesSQL":        "a CTE fragment that resolves each follow-up candidate's recipients; meetingFollowUpsPage wraps it in the follow-up gates and the addressable-contact clause before any contact is named",
	"internal/modules/activities:reachableOnConversation":    "the reply goes to this conversation's counterparty: the links were visibility-checked when the anchor was read under EnsureActivityContentVisible, and each is re-checked when the reply's own links are inserted",
	"internal/modules/activities:readPreparations":           "reads attendee contact ids only to hash them into the preparation fingerprint, for meetings the caller already read under the content clause; no id is returned",
	"internal/modules/activities:refoldTargetsTx":            "the clock refold reads the contact, deal, company and project an activity whose time has come is filed under, behind auth.RequireSystem; the ids go straight into the refold and none reaches a reader",

	"internal/modules/ai:DueDeferredBuilds":   "the voice-build retry worker lists due builds across profiles, with no seat behind it; each profile id goes to the build pipeline, which is the profile owner's own work",
	"internal/modules/ai:ListDeltas":          "a voice profile's history, behind auth.Require for voice_profile and visibleProfile, which admits only the profile's own owner; a voice profile has no row scope beyond its owner",
	"internal/modules/ai:eraseSamplesTx":      "an erasure must reach every owner's voice profile holding a sample from erased mail or naming an erased contact; each profile id is only locked, and the call returns nothing but an error",
	"internal/modules/ai:lockJudgeableSignal": "finds the learning signal by draft reference and the caller's own seat (p.owner_id = $2), so the profile it names is always the caller's own",

	"internal/modules/assignments:parentOfAssignment": "an unlocked lookup of which record an assignment hangs on; lockAssignment then takes ensureParentWritable on that parent before anything is read for the caller",

	"internal/modules/capture:ParkedEnrichments":         "the operator recovery's list of companies an enrichment outage exhausted, with no seat to narrow to; the ids reopen each company's own backoff cursor and none is returned to a reader",
	"internal/modules/capture:channelReplyContact":       "the capture sink resolves a messaging counterparty to the contact on file, by the provider's own identity key; the id files the inbound reply and is never served",
	"internal/modules/capture:insertFurtherParticipants": "the capture write matches each party's address to a contact inside the INSERT that stores the participant rows; only a row count leaves it",
	"internal/modules/capture:mailContacts":              "the deal rule reads every contact on a mail; a seat that sees only some would file on a guess, so the ids stay in the rule and its link passes EnsureLinkTarget",
	"internal/modules/capture:mailReplyContact":          "the capture sink resolves a mail counterparty to the contact on file by address; the id files the inbound reply and is never served",
	"internal/modules/capture:privateLeadLinksTx":        "answers which lead links sit on messages private to one seat, for the privacy and personal-sender passes that remove them; the ids are unlinked, never served",
	"internal/modules/capture:readDealProvenance":        "the deal-link repair reads which deal a captured mail is filed under and how; the id decides only whether the link stays and is not served",

	"internal/modules/chatlinks:AdoptProject": "runs only under the chat engine's system principal (auth.RequireSystem); the project id is stored on the deal's channel link and never returned to a caller",

	"internal/modules/collections:compareSnapshot": "writes a Live List's entered and left events under the system checker, which must see every record for the history to be whole; every read of those events applies the reader's row scope",

	"internal/modules/consent:RecordHardBounceTx":                "runs under the connector principal that carried the delivery report, with no seat; the caller chooses no id, and the one contact id it reads only decides which record the audit row hangs from",
	"internal/modules/consent:bindWithdrawalSubject":             "the send path mints a withdrawal link under the system principal that composes mail; the caller hands over the address the mail goes to, and two matching records bind no subject at all",
	"internal/modules/consent:deliveryLinks":                     "the transmit gate reads the deal ids the delivery's activity is filed under, for the live-deal arm; the ids decide whether a send proceeds and never leave the gate",
	"internal/modules/consent:legacyPreferenceTokenAsWithdrawal": "the anonymous one-click unsubscribe (RFC 8058) has no session to scope; possession of the mailed token is the authority, and the ref carries only an address and the withdrawal scope",
	"internal/modules/consent:liveAddressHolderTx":               "mints a preference link for the address a mail already goes to; the caller names an address, not an id, and the contact id only keys the link",
	"internal/modules/consent:resolvePreferenceTokenTx":          "the public preference centre has no session to scope; possession of the mailed token is the authority, as ungatedEntryPoints records for ResolvePreferenceToken",
	"internal/modules/consent:resolveWithdrawalTokenTx":          "the anonymous withdrawal resolve projects contact_id and lead_id so a one-click unsubscribe can act; the mailed credential is the authority and carries only an address and a scope",
	"internal/modules/consent:revokeOverrideAdmittedTx":          "a vouch chain spans merged records, so no single row scope bounds it; EnsureRetractable is taken on the named contact and its survivor first, and only decided_by_level is returned",
	"internal/modules/consent:subjectOfConfirmTokenTx":           "the public confirmation submit names whose link this is before it locks anything; the token is the authority, and the spend that redeems it runs under the subject lock",
	"internal/modules/consent:survivingSubject":                  "follows contact.merged_into_id one hop for a contact every caller has already probed in the same transaction (EnsureWritable, EnsureRetractable, EnsureReadable or the subject lock); it returns the id named or its survivor",

	"internal/modules/contacts:CommitmentsTheirsForProjects":        "the projects are the ones the caller named, read from a record page it already opened; the contact each claim names passes ScopeClauseFor in the same statement",
	"internal/modules/contacts:CompaniesCarryingLiveProjects":       "work the reader cannot see must still block a merge, so the answer is unscoped by project; it says only whether a company holds one, never which",
	"internal/modules/contacts:CompaniesWhoseEmploymentEndedTx":     "reads the companies whose employments ended, as the system principal (auth.RequireSystem); the clock refold job takes them and returns nothing to a caller",
	"internal/modules/contacts:ContactsOwedACohortRepair":           "the link-reconcile sweep lists contacts whose captured mail is not all on their record, as the system; each id goes to the repair, never to a reader",
	"internal/modules/contacts:CountExistingLeadContactPairs":       "the LinkedIn identity data task's bounded estimate of live lead-contact pairs; it returns a count, never an id",
	"internal/modules/contacts:DomainsOwedTheirContacts":            "the link-reconcile sweep runs as the system, the only actor that may attach every seat's contacts to a company; each company id goes to that attach and none is served",
	"internal/modules/contacts:EnrichmentFence":                     "reads whether the subject is archived or merged as two flags (merged_into_id IS NOT NULL), inside the enrichment hold; it returns a yes or no with a reason, never an id",
	"internal/modules/contacts:NormalizeContactLinkedInTx":          "a data task page under auth.RequireSystem that rewrites stored LinkedIn profiles; the contact ids advance the cursor and none leaves the server",
	"internal/modules/contacts:OpenDuplicateCompanyPairsTx":         "system-only (auth.RequireSystem): hands Deal Scout the open company pairs to withdraw a suggestion raised twice; a rep sees a suggestion only through deals' visibility clause",
	"internal/modules/contacts:ReleaseSignatureWorkTx":              "a signature-work release selector under auth.RequireSystem; the contact keys move between import runs inside maintenance and none enters a response",
	"internal/modules/contacts:ReopenSignatureTitleHistoryTx":       "a data task page under auth.RequireSystem that reopens signature watermarks; the contact ids page the work and no field changes",
	"internal/modules/contacts:ResolveOrCreateChannelIdentity":      "the channel ingest's insert-then-adopt: the incumbent contact bound to a provider identity is the answer by construction, and adopting it widens no visibility",
	"internal/modules/contacts:RetractMisattributedSignatureFields": "a repair under the enrichment job's system principal; narrowing it to one caller would leave other contacts with another sender's title. Its authority is auth.Require(contact, update), confined in writesWithoutARowProbe",
	"internal/modules/contacts:SetChannelIdentityBlocked":           "applies a provider's block or unblock to the identity it names, under the ingest actor; the contact id goes to the audit row, never to a reader",
	"internal/modules/contacts:SignatureFieldSourcesTx":             "pages signature fields for the repair data task as the system actor; the ids go only to RetractSignatureFieldsByIDTx, ratified in writesWithoutARowProbe",
	"internal/modules/contacts:SubjectsWithProviderFills":           "names the contacts one provider's purchases wrote to, so the revert can take them one transaction at a time; the ids go to that revert and none is served",
	"internal/modules/contacts:addressVariantContacts":              "the dedupe ladder searches the whole workspace by design; the id becomes a review row, or a possible-duplicate answer that asks the caller's visibility before naming the contact",
	"internal/modules/contacts:adoptEmailRoutedIncumbent":           "binds a messaging account to the contact the address already found; adopting leaves the contact's visibility as it was and returns the id only to the ensure",
	"internal/modules/contacts:companyByDomain":                     "the provider fill resolves an employer by its unique domain; the company id becomes an employment edge inside the fill and is not returned to a reader",
	"internal/modules/contacts:companyIsLive":                       "locks a company and reads merged_into_id IS NULL as a flag, to learn whether it is still live before an attach; it answers a yes or no, never an id",
	"internal/modules/contacts:declinedTriageBind":                  "re-reads the company a triage row already names after a guarded bind declined; the id only tells a replay from a lost race and is never served",
	"internal/modules/contacts:dropProfileFieldSupportTx":           "names the companies and contacts whose signature rows lose a source; the ids stay inside the move's SQL and reach no caller",
	"internal/modules/contacts:dropShortlistMemberships":            "reads back the Shortlists an archive took the record off, from the statement that removed the memberships; the ids go into the archive's own audit evidence, never to a reader",
	"internal/modules/contacts:employmentFingerprint":               "digests one contact's employment edges into a hash, after ApplyEmploymentImport took HoldWritableLive on the contact; no company id is returned",
	"internal/modules/contacts:exactCompanyByDomain":                "the dedupe ladder scores every company in the installation; the id it finds routes capture or becomes a review row, never a response",
	"internal/modules/contacts:exactContactByChannelIdentity":       "the dedupe ladder's first lane matches a provider identity against every live contact; the id routes capture or becomes a review row, never a response",
	"internal/modules/contacts:exactContactByEmail":                 "the dedupe ladder matches an address against every live contact; the id routes capture or becomes a review row, never a response",
	"internal/modules/contacts:exactContactByPhone":                 "the dedupe ladder matches a number against every live contact; the id routes capture or becomes a review row, never a response",
	"internal/modules/contacts:fileLinkedInTwins":                   "the LinkedIn lane searches the whole workspace by design, as the address-variant lane does; the ids become review rows inside the write and none is returned",
	"internal/modules/contacts:holdCohortActivities":                "locks the activities a cohort repair may write before it locks the contact; the ids bound that repair's writes inside its transaction and none is returned",
	"internal/modules/contacts:inferredEmploymentPage":              "the employment repair pages inferred edges under auth.RequireSystem; the company ids decide which edges retire and none is served",
	"internal/modules/contacts:leadLinkedCompany":                   "the lead writers must see a lead's company whether or not the caller may open it; every read of a lead withholds it through withholdUnreadableCompanies",
	"internal/modules/contacts:leadResponseTouches":                 "the lead SLA consumers ask which leads an activity touches, as the event worker; the ids drive the SLA ladder and never reach a reader",
	"internal/modules/contacts:legacyEvidenceMoves":                 "names the companies and contacts whose legacy evidence rows move into field_provenance; the ids stay inside the move's SQL and reach no caller",
	"internal/modules/contacts:linkActivityToContact":               "settles a captured contact against a merge just before the link insert; the id is the one the ensure already chose, and it is written, not served",
	"internal/modules/contacts:linkedLeads":                         "the lead-score recompute consumer asks which leads an activity touches, as the event worker; the ids are rescored and never reach a reader",
	"internal/modules/contacts:lockContactForEmployment":            "locks the contact behind one employment edge so two primary-flag writes serialize; only the lock is taken and no id is returned",
	"internal/modules/contacts:lockKeptPromise":                     "the kept-promise reconcile locks one claim under auth.RequireSystem; the contact id settles that claim and is returned to no caller",
	"internal/modules/contacts:moveLegacyMeetingEvidence":           "the subject query of the legacy evidence move, under auth.RequireSystem; the company ids stay inside the move's SQL and reach no caller",
	"internal/modules/contacts:nameDomainClaimant":                  "the dedupe ladder's rival for a company named after a domain; the ladder scores every company, and the id becomes a review row, never a response",
	"internal/modules/contacts:pendingSignatureCohort":              "the signature retry selector behind PendingSignatureWork's auth.RequireSystem; its system-only caller consumes the keys inside maintenance and none enters a response",
	"internal/modules/contacts:primaryEmployersOf":                  "lists every current employee of the company a merge retires, past the reader's scope, because the merge moves every edge; the ids go only to the twin re-check",
	"internal/modules/contacts:promoteCohortTx":                     "settles the cohort's contact against a merge before the repair writes; the id is the one the pass was handed, and it is written, not served",
	"internal/modules/contacts:promotedContactOf":                   "an unlocked hint naming the contact a lead was promoted into; the demote locks it, then re-reads under both locks and EnsureWritable before acting",
	"internal/modules/contacts:queryOpenPairs":                      "the nightly duplicate re-check reads both ends of every open pair under the system principal; the ids decide only whether a pair stays open",
	"internal/modules/contacts:readCurrentEmployer":                 "a contact's current primary employer for the duplicate filing and re-check; the company id is compared inside the pass and none leaves the server",
	"internal/modules/contacts:readDedupeCandidate":                 "the by-id read of one review pair; GetDedupeCandidate then requires both sides to pass EnsureVisible, or EnsureVisibleLive while open, before the row is served",
	"internal/modules/contacts:readDispositionTx":                   "locks a domain's triage verdict; the company it names is the one the verdict resolved to, used inside the ensure or the triage write, never served",
	"internal/modules/contacts:readEmployerChange":                  "the employer-change signal reads the current and target companies; employerChangeFor then asks ownerSeesCompanies before anything is proposed",
	"internal/modules/contacts:readStoredLeadPartner":               "a decision about a lead's partner link must see what is stored, which a lead read blanks for a partner the caller cannot open; the id decides the link and is not served",
	"internal/modules/contacts:refreshChannelUsername":              "keeps a provider identity's handle current on each inbound message; the contact id goes to the audit row of a handle change and is never served",
	"internal/modules/contacts:refuseWhenBothCarryProjects":         "work the caller cannot see must still block a company merge; the refusal names only projects the caller may see and counts the rest",
	"internal/modules/contacts:retractUnsupportedPhones":            "reads contact ids to lock and audit unsupported signature numbers; its caller requires the system actor and the repair returns a count",
	"internal/modules/contacts:subdomainClaimants":                  "the dedupe ladder's subdomain rival, unscoped like nameDomainClaimant; the ladder scores every company and the id becomes a review row",
	"internal/modules/contacts:switchboardNumbers":                  "counts the companies stating each number, to tell a switchboard from a shared line; it returns a flag per number, never a company id",
	"internal/modules/contacts:takeBackSignatureFieldsTx":           "the misattribution repair's ordered locks on the contacts its rows name; the ids are locked, never served, under RetractSignatureFieldsByIDTx's terms",
	"internal/modules/contacts:workedFromContact":                   "lead writers that keep one live lead per contact must see the link whether or not the caller may open the contact; every lead read withholds it through withholdUnreadableSourceContacts",

	"internal/modules/deals:DealOfContract":                "the stage-evidence trigger reads the deal a contract is bound to, as the event worker; the deal id settles that deal's criteria and is never served",
	"internal/modules/deals:ReadActivityAuthorship":        "the stage-evidence readers ask which deal an activity reaches, under ActivityContentClause on the activity; the deal id scores evidence and is not returned to a reader",
	"internal/modules/deals:SilentClosesTx":                "the backfill job lists silent closes under auth.RequireSystem; the deal ids go to the offer-back writer and none is served",
	"internal/modules/deals:copyOfferIntoRevision":         "clones a sent offer into its next revision after RegenerateOffer read it through visibleOfferLocked; the deal id is copied row to row and not returned on its own",
	"internal/modules/deals:correctionReceiptSelect":       "the SELECT list every change report shares, with no WHERE of its own; each caller (RecentCorrectionsOwnedBy, DealChanges, TeamChanges) adds ScopeClauseFor or EnsureVisibleExisting",
	"internal/modules/deals:lockCorrection":                "locks the correction row FOR UPDATE so a repeated Undo serializes; the caller takes EnsureWritable on the deal it names, in the same transaction, before acting",
	"internal/modules/deals:lockReview":                    "locks a legal review and reads its deal; WithdrawLegalReview and DecideLegalReview take EnsureWritableLive on that deal before acting",
	"internal/modules/deals:nextMembers":                   "the close-date pass walks the membership it froze at its start, under the system principal; narrowing to one rep would leave the rest unassessed while the run reported coverage",
	"internal/modules/deals:queryActivityParticipants":     "reads an activity's parties for the authorship rule; it answers roles and whether a contact is present, never a contact id",
	"internal/modules/deals:readOfferTerms":                "reads each offer's buyer company into the legal review terms; the ids feed a SHA-256 the stage gate compares and never leave the server",
	"internal/modules/deals:readQuietSide":                 "reads one deal's last word each way under the deal owner's context; compose names the counterparty through ContactNamesTx in that context, so an unseen contact goes unnamed",
	"internal/modules/deals:readReviewTerms":               "reads the deal's company into the legal review terms; the ids feed a SHA-256 the stage gate compares and never leave the server",
	"internal/modules/deals:reconcileEvidenceAuthorshipTx": "pages activity evidence for the own-domain repair as the system actor; its deal ids go only to correctEvidenceAuthor, which locks and audits the row",
	"internal/modules/deals:reconcileMeetingProof":         "re-judges machine event_held claims as the system actor; its deal ids go only to withdrawMeetingClaim, which locks and audits the row",
	"internal/modules/deals:resolveBuyerCompany":           "an explicit buyer company passes EnsureLinkTarget in admitOfferCreation; without one the offer inherits the company of the deal the caller is writing",
	"internal/modules/deals:wonStints":                     "the won-stint population behind the stage pace; callers aggregate it into per-stage durations, and no deal id is returned",

	"internal/modules/finance:findInvoice": "the finance sync's before image of one mirrored invoice, held FOR UPDATE for its own rewrite; the company id goes into the audit image, never to a reader",
	"internal/modules/finance:findPayment": "the finance sync's before image of one mirrored payment, held FOR UPDATE for its own rewrite; the company id goes into the audit image, never to a reader",
	"internal/modules/finance:readLinks":   "the finance sync reads which company a human linked each source customer to; a schedule holds no seat, and the ids stamp mirrored rows",

	"internal/modules/integrations:applyStoredPurchases": "the provider backfill sweep hands stored purchases to the domain, as the integration worker; the contact ids go to that apply and none is served",
	"internal/modules/integrations:leaseForPoll":         "the provider run worker leases a run to poll; the contact id travels with the run to its settlement, and ReadRunForContactTx probes EnsureVisible before a reader sees a run",
	"internal/modules/integrations:leaseForSubmit":       "the provider run worker leases a queued run to submit; the contact id travels with the run, and ReadRunForContactTx probes EnsureVisible before a reader sees a run",
	"internal/modules/integrations:readRun":              "loads one provider run for the queue and skip writers; the reader path, ReadRunForContactTx, takes EnsureVisible on the contact first",

	"internal/modules/introductions:AwaitingMyAnswer": "an ask is between two colleagues and is bound to the acting introducer; the contact it names is the one the requester asked this caller about",

	"internal/modules/privacy:ListRestrictedActivities":     "the restriction register, gated on the retention-policy read that the seeded roles hold admin-only; it lists held records so an operator can release them",
	"internal/modules/privacy:contactsMatchingSuppression":  "hashes every live address and channel identity against the erasure-suppression list, which has no tenant key; the ids go straight into EraseContact",
	"internal/modules/privacy:leadTwinOf":                   "follows a rights-case subject through merges to the leads they were; an operator's seat must not narrow a subject's rights case",
	"internal/modules/privacy:legacyBuyerAuditQuery":        "finds the erasure of the seat that wrote a deleted record, for the rights case; the record itself is gone and the operator's seat must not narrow the case",
	"internal/modules/privacy:readLeadNotice":               "names the kinds of data a notice link's own website lead holds, for the privacy page; it reads presence flags for the lead the link resolved and returns no reference",
	"internal/modules/privacy:redactSubjectLegalReviewText": "erasure redacts the subject's name from legal reviews on every deal they hold a stake in; it writes tombstones and returns no deal to a reader",
	"internal/modules/privacy:sarLegalReviewSection":        "a subject-access export of legal reviews on the subject's deals; Art. 15 owes everything held, and AssembleSAR admits only an unbounded human holding contact.delete",
	"internal/modules/privacy:sarProvenanceSections":        "a subject-access export of where held data came from; Art. 15 owes everything held, and AssembleSAR admits only an unbounded human holding contact.delete",
	"internal/modules/privacy:sarRecordSections":            "a subject-access export of the records the subject appears in; Art. 15 owes everything held, and AssembleSAR admits only an unbounded human holding contact.delete",
	"internal/modules/privacy:subjectQuestionSources":       "follows a rights-case subject to the deals and projects they hold a stake in, so access and erasure reach every kept answer; an operator's seat must not narrow the case",
	"internal/modules/privacy:tombstoneRestrictions":        "a restriction writes one tombstone per held record; the deal ids go into the audit row and event the authority is shown, never to a reader",

	"internal/modules/search:RebuildEdges":            "the nightly rebuild replaces the graph projection wholesale from the base tables; contact ids move between tables inside the statement and none is returned",
	"internal/modules/search:ReleaseGraphHistoryTx":   "an interaction release selector under auth.RequireSystem; the contact keys move between import runs inside maintenance and none enters a response",
	"internal/modules/search:affectedContactPairs":    "resolves which contact edges an activity change can affect, for the projection refold; the pairs are refolded inside the consumer and never served",
	"internal/modules/search:affectedPairs":           "resolves which seat-contact edges an activity change can affect, for the projection refold; the pairs are refolded inside the consumer and never served",
	"internal/modules/search:foldPairsSQL":            "the upsert that refolds named pairs of the graph projection; contact ids move between tables inside the statement and none is returned",
	"internal/modules/search:lockInteractionPairs":    "the interaction lock selector of the projection refold; the contact keys are locked inside maintenance and none enters a response",
	"internal/modules/search:pendingInteractionPairs": "the interaction retry selector behind DrainPendingGraphTx's auth.RequireSystem; the contact keys are refolded inside maintenance and none enters a response",
	"internal/modules/search:queueInteractionPairs":   "the interaction queue selector of the projection refold; the contact keys are queued inside maintenance and none enters a response",
	"internal/modules/search:rebuildContactEdges":     "the contact half of the nightly projection rebuild; contact ids move between tables inside the statement and none is returned",
})

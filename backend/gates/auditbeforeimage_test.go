// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// An audited update says what it changed FROM.
//
// audit_log carries before and after images. A row whose action is `update` and
// whose before is absent records that something changed and cannot say what it
// changed from — so field history renders half a change, and nothing can restore
// the value. There is no other place that value is written down: an audit row
// cannot be written after the fact.
//
// Two kinds of write reach the same column, and until AuditEvent existed they
// were spelled identically. One has no prior field state at all — a secret
// rotated, a delivery replayed, a tag applied — and its absent before-image is
// the honest answer. The other simply did not look. This gate is what keeps them
// apart: the first says so by the door it calls, and says why in a waiver here.
//
// It is the census half of a pair. storekit's refusal is the chokepoint, and it
// is the authority — an H2 walk cannot resolve a call reached through an
// interface, a stored field, or a closure, nor an action argument built at
// runtime (privacy/retentionactions.go passes a verb read from a database row).
// The gate censuses the direct writers of audit_log too, because the chokepoint
// binds only what goes through it: the approvals module sends its own INSERT for
// its own row. Between them, the gate says who writes the table and the
// chokepoint says what a door will accept.
//
// Only `update` is judged. `archive` sites also pass an absent before-image and
// are deliberately left alone: un-archiving is not a before-image replay but
// `SET archived_at = NULL` plus whatever that record's archive took down with
// it, which is a per-type product decision and not this gate's to force.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The doors into audit_log, and which of them carries a before-image.
var auditDoorsWithBeforeImage = map[string]bool{
	"Audit":                  true,
	"AuditWithEvidence":      true,
	"AuditWithTrail":         true,
	"AuditEvent":             false,
	"AuditEventWithEvidence": false,
}

// Doors whose verb is fixed by construction rather than passed as an argument.
// AuditWithTrail takes a verb the caller cannot spell freely — Trail.Resolve
// admits update and restore and nothing else — so every call through it is
// update-shaped and the rule binds it without this walk having to reduce
// anything.
//
// This is why threading a runtime verb through the record types costs no
// ratification: the one door that carries a verb the walk cannot see is also
// the one door whose verb set is closed.
var auditDoorsAlwaysUpdateShaped = map[string]bool{
	"AuditWithTrail": true,
}

// Argument positions shared by every door: (ctx, tx, verb, entityType,
// entityID, …). Only the images differ after that, which is the point of the
// split. AuditWithTrail carries a Trail where the others carry an action
// string; the positions are the same, and its verb is answered by
// auditDoorsAlwaysUpdateShaped rather than read from the argument.
const (
	auditActionArg     = 2
	auditEntityTypeArg = 3
	auditBeforeArg     = 5
)

// eventShapedUpdates: writes that record an occurrence on a record rather than a
// change to its fields, keyed by `path:function`. Each entry states what the
// write has INSTEAD of a prior state — a reason that only restates the subject
// is refused by gatekit.
//
// The FUNCTION is the unit of ratification, not the call. A function that audits
// twice — people/linkedinmatchapply.go does — is judged once, so ratifying it
// says every audited update it makes is occurrence-shaped. Splitting the two
// would need a line number in the key, and a key that moves whenever the file
// above it moves is a waiver that goes stale for no reason.
//
// Empty is a result, not a default: it would mean every audited update in the
// tree records what it changed from.
var eventShapedUpdates = gatekit.Waive(map[string]string{
	"internal/modules/notices/store.go:MarkRead": "the recipient settling their notice. An OCCURRENCE and not a replacement: read_at moves from its one legal prior state (NULL — the statement's own guard makes a second settle a no-op that audits nothing), so the before-image is fully implied by the action itself, and the evidence records the one fact that changed (read: true)",

	"internal/modules/people/organization_vat_check.go:RequestVatCheck": "a person asking for the register to be consulted again. An OCCURRENCE and not a replacement: nothing about the company changed — the question has been asked and not yet answered — so no field holds a prior value for an image to name. It is recorded at all because it is recordable nowhere else: the worker that answers runs under a confined system principal, so the receipt it later writes says system:vatcheck whatever prompted it, and without this row nobody can tell that a person spent a consultation on this company. The verb is `update` on `organization` because that is the row the caller was authorized against and the row whose standing the answer will change",

	"internal/modules/people/organization_vat_check.go:RecordVatCheck": "the FIRST consultation of a company's VAT ID. An OCCURRENCE and not a replacement: nothing about the company changed, an answer that did not exist before was recorded, and there is no prior verdict, receipt or register name for an image to name. A RE-check takes the other branch and carries both — which is the case that matters, because a number that was valid and now is not is the finding this lane exists for. The verb is `update` on `organization` because that is the row the caller was authorized against and the row whose standing this changes",

	"internal/modules/comms/bounce.go:RecordBounce": "the receiving mail system refusing a message is an OCCURRENCE on the sent activity, not a field replacement: the bounce columns were NULL on every row this can match (the CAS refuses a second mark), so the prior state is the absence the mark ends, and there is nothing for a before-image to name",

	"internal/modules/people/channelidentity.go:auditChannelIdentityChange": "binding an account to a person who had none replaces nothing: the question is which account reaches this human, and before the bind none did. A rebind takes the other branch and names the binding it moved",

	"internal/modules/consent/qualifyingevent.go:RecordQualifyingEvent": "a human writing down an exchange that happened away from every system — a card handed over at a trade fair. It is an OCCURRENCE and not a replacement: nothing about the person changed state, a row saying what happened was added, and there is no prior version of a meeting for the image to name. The verb is `update` on `person` because that is the row the caller was authorized against and the row whose sendability this changes; the alternative, a create on the qualifying-event row, would put a rule in the trail that authorized nothing here",

	"internal/modules/consent/confirmsubmit.go:stageSubmission": "a data subject proposing a correction to their own record through the confirm link, or asking to be removed. An OCCURRENCE and not a replacement: nothing about the person changed state, and the field a correction names still holds its old value — deliberately, because a bearer-token caller may not write the CRM, so there is no prior version for an image to record. The verb is `update` on `person` because that is the row a later reader asks about. The proposed value stays OFF the audit payload, where a second copy of the subject's own data would outlive the erasure that clears the first",

	// Three writes whose FIRST occurrence has no prior state and whose later
	// ones do. Each routes on that, so the branch that replaced something says
	// what, and the branch that replaced nothing says so rather than inventing
	// an image. The runtime refusal found all three; no static reading could,
	// because each reaches the door through a verb it does not spell.
	"internal/modules/capture/owndomainstore.go:Add":                "registering a domain nobody had seen adds an entry to the workspace list, so there is no prior source or verification for the row to name; confirming a candidate a mailbox already saw takes the other branch and records what moved",
	"internal/modules/overlay/flipstate.go:auditFreeze":             "a first seal freezes a mirror that was not frozen, so no field held a value to record; a reseal or a release passes the state it moved and takes the image door",
	"internal/modules/people/domainadmission.go:SetDomainAdmission": "a first decision replaces no admission, no reason and nobody answerable for one; every later decision moved all three and records what they were",

	"internal/modules/people/linkedinmatchapply.go:auditLinkedInMatch": "the confirmed handle lands in person_social and no column of the person moves, so what the contact gained is the whole of what this write has to record",

	"internal/modules/capture/senderoverride.go:Set":     "the settings row has no column for a sender decision; the write records one seat's answer about one address, and the image names the decision and the kind it overruled — the prior state a reader wants is what the MACHINE had said, which the image carries",
	"internal/modules/capture/senderoverride.go:Remove":  "the same decision withdrawn: what it had been is in the row this deletes, and the image names that it was withdrawn",
	"internal/modules/capture/exclusionstore.go:Add":     "the settings row has no column for a rule; the write inserts one exclusion into a list, and the image names the rule that now applies",
	"internal/modules/capture/owneridentitystore.go:Add": "the settings row has no column for a seat's own addresses; the write inserts one claim into a per-seat list. The image names the claim's id and kind and deliberately NOT its value — an owner identity is one person's private address, and the audit log is where nothing erases it and every admin reads it",

	"internal/modules/people/providerclaims.go:WriteProviderClaims": "the bought values land in person_provider_claim and the person row is untouched, and quoting them would put a second copy of the subject's data in a table the erasure treats as evidence rather than as subject data",
	"internal/modules/people/researchclaim.go:SaveResearchClaims":   "the claims land in person_profile_field and the person row is untouched; what arrived rides the evidence column, where field history will not read it as a field of the record",

	"internal/modules/people/organizationtechnical.go:auditTechnicalEnrichment": "what a company publicly runs lands in organization_fact and no column of the organization moves, so no field has a prior value for an image to name. What was replaced is still recorded, and more precisely than an image would: the evidence carries the rows written AND the rows the reconciliation removed, which is what makes a mail provider moving to Microsoft 365 readable as a move rather than as an arrival",

	"internal/modules/collections/tags.go:ApplyTag": "the tag row is untouched; the write inserts a taggable link, " +
		"and the after image names the record it now points at.",
	"internal/modules/collections/tags.go:RemoveTag": "the tag row is untouched; the write deletes a taggable link, " +
		"and the after image names the record it stopped pointing at.",
	"internal/modules/collections/members.go:AddMember": "the list row is untouched; the write inserts a membership, " +
		"and the after image names the record that joined.",
	"internal/modules/webhooks/store.go:RotateSecret": "the new signing secret replaces a value that must never be " +
		"copied into audit_log, so the after image carries the fact of the rotation and neither secret.",
	"internal/modules/webhooks/deliverystore.go:requireReplay": "the subscription is unchanged; the write records that " +
		"a delivery was re-attempted, and the after image names which one.",
	"internal/modules/activities/extractionclaim.go:FinishExtractionRead": "the claim proves the reading was running, " +
		"so a prior state exists — it is a job's own progress rather than a field a person set, and a document is " +
		"re-read by starting a new attempt, never by putting a finished one back to running.",
	"internal/modules/activities/transcriptread.go:FinishTranscriptRead": "the compare-and-set proves the reading was " +
		"running, so this is a run record's outcome, not a record whose fields a user edits: nothing would ever be " +
		"restored to a reading's earlier progress.",
	"internal/modules/ai/feedback.go:Record": "the upsert as readily creates the ledger row as replaces one, so the " +
		"write records that a human decided rather than a field moving; a verdict it superseded was written through " +
		"this same door and stays readable as that write's own audit row.",
})

// unresolvableAuditActions: call sites whose action argument this walk cannot
// reduce to a constant, keyed by `path:function`. Each entry states that
// wrapper's own guarantee, because the gate cannot judge the verb and the
// chokepoint is what actually holds these.
//
// Ratifying them is the only honest third answer. Failing them would make the
// gate impossible to green over wrappers that are correct; merely counting them
// would let a real defect through in silence.
var unresolvableAuditActions = gatekit.Waive(map[string]string{
	// The seam that routes an extension's own change, choosing the door from
	// what the change carries rather than from a verb it cannot read.
	"internal/compose/extledger.go:recordExtensionChange": "the verb is the unit's own, and the seam picks the door from whether the change declared a before-image, so an update either carries one or is recorded as the occurrence it says it is",

	// Wrappers whose images are computed by the caller and handed in whole.
	"internal/modules/capture/lifecycleaudit.go:auditLifecycle":                       "both images are parameters, so the verb and what it changed arrive together from a caller that has the row",
	"internal/modules/overlay/writeaudit.go:auditWriteBack":                           "both images are parameters, minimized for the entity before they land; a write-back that changed a field arrives holding what it replaced",
	"internal/modules/overlay/usermapadmin.go:auditUserMapChange":                     "both images are parameters built from the mapping row on either side of the change",
	"internal/modules/overlay/activation.go:activateConnection":                       "the images come from the connection row read in this transaction, whichever activation verb the caller resolved",
	"internal/modules/identity/teams.go:recordTeamChange":                             "the verb spans create, update, archive and restore, and every one of them is handed the team row's own images",
	"internal/modules/identity/onboarding.go:auditOnboardingState":                    "create and update share one writer, and both pass the onboarding row as it stood before the step",
	"internal/modules/people/coldstartprofile.go:applyColdStartTx":                    "a created record diffs against an explicitly empty image and an existing one against its own columns, so both verbs carry a before-image by construction",
	"internal/modules/people/company.go:SaveCompany":                                  "the anchor's columns are read before the save and narrowed against what it wrote, so the update branch records what each one held",
	"internal/modules/people/companysiteread.go:recordSiteReadConfirmation":           "the confirmation shares the anchor's column images, so its update branch records what the record held before the read was applied",
	"internal/modules/people/relationshipimage.go:emitRelationshipChangeWithEvidence": "create and archive carry the edge whole and update carries the columns that moved, narrowed against the row read under the lock",
	"internal/modules/ai/voice_source_store.go:recordSourceIngest":                    "a first ingest has no prior row and records none; a re-ingest records the corpus source it replaced",
	"internal/modules/ai/voice_versions.go:recordVoiceVersionTransition":              "the image is the version's own status on either side, and the verb names which transition it was",
	"internal/modules/ai/ratewrite.go:writeModelRate":                                 "an upsert resolves to create or update, and the replaced rate is read and passed in either way",
	"internal/modules/deals/fxrate_store.go:writeFxRate":                              "the same upsert shape as the model rate, carrying the rate row it superseded",
	"internal/modules/capture/freemaildomain.go:Add":                                  "a fresh carve-out and an amended one take different verbs, and the amended one carries the rule it replaced",
	"internal/modules/commissions/decide.go:decideTx":                                 "the verb names the decision and the images are the patch's own, so what the decision moved is recorded with it",
	"internal/modules/commissions/decide.go:voidOne":                                  "a void is spelled as its own verb and carries the patch images for the row it retired",
	"internal/modules/consent/store.go:recordAdmittedTx":                              "a grant and a withdrawal are separate verbs, and both record the consent state they moved from",
	"internal/modules/dealrooms/lifecycle.go:moveRoom":                                "each room transition names its own verb and carries the patch images the move built",
	"internal/modules/privacy/retentionpolicystore.go:Delete":                         "the verb is an archive of the policy row, and the image is the policy as it stood",
	"internal/platform/settings/store.go:SetRawTx":                                    "each setting declares its own verb, and the value on either side is rendered by the same declaration",

	// The one site no static reading could ever judge, and the reason the
	// chokepoint is not optional.
	"internal/modules/privacy/retentionactions.go:apply": "the verb is a column of the policy row being applied, read at run time; retention_policy_action_check admits only archive, anonymize and erase, so this site cannot reach update at all",
})

// directAuditLogWriters: files that send their own INSERT into audit_log rather
// than calling a door. Every rule this gate and the chokepoint hold is built on
// the doors, so a direct writer is outside both — and one nobody ratified is a
// second spelling of the write nobody noticed.
//
// storekit itself is absent on purpose: it IS the doors, and its own statement
// is what they reach.
var directAuditLogWriters = gatekit.Waive(map[string]string{
	"internal/modules/approvals/service.go":       "the approval row's own lifecycle, which is not a record whose fields move: the statement writes no before or after column at all, so there is no image for the door's rule to be about",
	"internal/compose/integration/harnessseed.go": "the integration harness's scrub-tombstone fixture, and the only writer here that is not product code. Its rows are erase tombstones — no before or after column, so there is no image for the door's rule to be about — and there is no door to reach: every product path that stamps one also archives its subject, and a tombstone on a LIVE record is the one seed that isolates an erasure boundary from that cascade",
})

func TestEveryAuditedUpdateRecordsWhatItChangedFrom(t *testing.T) {
	t.Parallel()
	defer eventShapedUpdates.AssertAllMatched(t)
	defer unresolvableAuditActions.AssertAllMatched(t)
	defer directAuditLogWriters.AssertAllMatched(t)

	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileReachesAnAuditDoor,
		// The extension tier is absent on purpose. extensions/ holds separate Go
		// modules that cannot import internal/platform/database/storekit at all —
		// the dependency DAG forbids it — so an extension reaches audit_log only
		// through compose/extledger.go, which is inside this root and ratified
		// below like any other wrapper.
	}.Files(t)

	constantsByPackage := packageConstants(files)

	var withBefore, eventShaped, unresolvable, direct int
	for _, parsed := range files {
		if writesAuditLogDirectly(parsed.File) && !strings.Contains(parsed.Path, "/storekit/") {
			direct++
			if !directAuditLogWriters.Waived(t, filepath.Dir(parsed.Path)+"/"+filepath.Base(parsed.Path)) {
				t.Errorf("%s writes audit_log with its own statement, so neither this gate's rule nor "+
					"storekit's refusal reaches it.\n"+
					"\tCall a door, or ratify the file in directAuditLogWriters with what its rows are.",
					parsed.Path)
			}
		}
		consts := constantsByPackage[filepath.Dir(parsed.Path)]
		for _, site := range auditSitesIn(parsed) {
			switch action, known := site.action(consts); {
			case !known:
				unresolvable++
				if !unresolvableAuditActions.Waived(t, site.key) {
					t.Errorf("%s: %s builds its audit verb at runtime, so this gate cannot judge it.\n"+
						"\tRatify it in unresolvableAuditActions with the guarantee that holds it, "+
						"or spell the verb as a literal.", site.key, site.door)
				}
			case action != "update":
				// archive, create, erase and the rest: out of this gate's scope.
			case auditDoorsWithBeforeImage[site.door]:
				withBefore++
				if site.beforeIsAbsent {
					t.Errorf("%s: audits an update to %s with no before-image.\n"+
						"\tRecord what the fields held (the row is readable in this transaction), "+
						"or call storekit.AuditEvent and say here what this write has instead.",
						site.key, site.entityType(consts))
				}
			default:
				eventShaped++
				if !eventShapedUpdates.Waived(t, site.key) {
					t.Errorf("%s: records an update to %s as an occurrence.\n"+
						"\tRatify it in eventShapedUpdates with what this write has instead of a "+
						"prior state, or record the before-image and call storekit.Audit.",
						site.key, site.entityType(consts))
				}
			}
		}
	}

	// Counted last, and reported rather than fatal: a Fatalf here would abort
	// before the findings above print, and those findings are the worklist.
	//
	// Three counts, because the walk has three ways to fail short and one total
	// would hide which. Each floor sits BELOW the real number, so a scan that
	// stopped working fails instead of quietly finding nothing.
	if withBefore < 80 || eventShaped+unresolvable < 15 || direct < 1 {
		t.Errorf("resolved %d image-carrying, %d occurrence-shaped and %d runtime-verb audit site(s), "+
			"and %d direct writer(s) of the table — one of the ways of finding a write has stopped working",
			withBefore, eventShaped, unresolvable, direct)
	}
}

// auditSite is one call into audit_log, located and pre-read.
type auditSite struct {
	key            string // path:function — what a waiver is keyed by
	door           string // which of the four doors
	args           []ast.Expr
	beforeIsAbsent bool
}

// action resolves the site's verb: a literal, or a package-level constant this
// file's package declares. Anything else — a parameter, a struct field, a value
// read at runtime — is unknown, and saying so is the point.
func (s auditSite) action(consts map[string]string) (string, bool) {
	if auditDoorsAlwaysUpdateShaped[s.door] {
		return "update", true
	}
	return resolveString(s.argAt(auditActionArg), consts)
}

// entityType is for the failure message only, so an unresolvable one degrades to
// the expression's own spelling rather than costing a finding.
func (s auditSite) entityType(consts map[string]string) string {
	if value, known := resolveString(s.argAt(auditEntityTypeArg), consts); known {
		return value
	}
	return "an entity named at runtime"
}

func (s auditSite) argAt(i int) ast.Expr {
	if i >= len(s.args) {
		return nil
	}
	return s.args[i]
}

func resolveString(expr ast.Expr, consts map[string]string) (string, bool) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		if node.Kind != token.STRING {
			return "", false
		}
		value, err := strconv.Unquote(node.Value)
		return value, err == nil
	case *ast.Ident:
		value, declared := consts[node.Name]
		return value, declared
	default:
		return "", false
	}
}

// storekitPath is the package whose doors this gate judges. The subject
// predicate, the qualifier resolution and the direct-INSERT sweep all read it,
// so they cannot come to mean different packages.
const storekitPath = "github.com/margince/margince/backend/internal/platform/database/storekit"

// fileReachesAnAuditDoor is the Scope subject, asked identically inside the
// roots and outside them: a file that calls a door, or that writes audit_log
// with its own statement.
func fileReachesAnAuditDoor(path string, file *ast.File) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	if writesAuditLogDirectly(file) {
		return true
	}
	qualifier := auditDoorQualifier(file)
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if _, isDoor := auditDoorAt(n, qualifier); isDoor {
			found = true
		}
		return !found
	})
	return found
}

// auditDoorQualifier is the name THIS file reaches storekit by. A gate that
// assumed the canonical spelling would stop seeing a file that aliases the
// import, and would report nothing rather than fail — the way a census fails
// short. Empty means the file cannot reach the doors at all.
func auditDoorQualifier(file *ast.File) string {
	if file.Name != nil && file.Name.Name == "storekit" {
		return ""
	}
	qualifier, dotImported := gatekit.ImportedAs(file, storekitPath)
	if dotImported {
		// A dot-import leaves the door bare. No file in this tree does it, and
		// the gate would silently stop judging one that did.
		return "."
	}
	return qualifier
}

// writesAuditLogDirectly reports whether a file sends its own INSERT into
// audit_log. The doors are a chokepoint only for callers that use them, and a
// module writing the table itself is outside every check built on them.
func writesAuditLogDirectly(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if found {
			return false
		}
		if text, ok := gatekit.LiteralText(asExpr(n)); ok &&
			strings.Contains(strings.ToUpper(text), "INSERT INTO AUDIT_LOG") {
			found = true
		}
		return !found
	})
	return found
}

func asExpr(n ast.Node) ast.Expr {
	expr, _ := n.(ast.Expr)
	return expr
}

func auditSitesIn(parsed gatekit.ParsedFile) []auditSite {
	qualifier := auditDoorQualifier(parsed.File)
	var sites []auditSite
	for _, decl := range parsed.File.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return true
			}
			door, isDoor := auditDoorAt(call.Fun, qualifier)
			if !isDoor {
				return true
			}
			sites = append(sites, auditSite{
				key:            parsed.Path + ":" + fn.Name.Name,
				door:           door,
				args:           call.Args,
				beforeIsAbsent: auditDoorsWithBeforeImage[door] && isAbsentImageExpr(call),
			})
			return true
		})
	}
	return sites
}

// auditDoorAt names the door a node calls, if it calls one.
func auditDoorAt(n ast.Node, qualifier string) (string, bool) {
	selector, isSelector := n.(*ast.SelectorExpr)
	if !isSelector {
		return "", false
	}
	pkg, isIdent := selector.X.(*ast.Ident)
	if !isIdent || qualifier == "" || pkg.Name != qualifier {
		return "", false
	}
	if _, isDoor := auditDoorsWithBeforeImage[selector.Sel.Name]; !isDoor {
		return "", false
	}
	return selector.Sel.Name, true
}

// isAbsentImageExpr reads the before argument the one way source can be read
// here: the bare `nil` identifier, written at the call.
//
// Everything else is invisible to it — a variable assigned nil, a parameter, a
// field, and any helper that builds an image conditionally, such as
// `imageOrNil(ch.Before)`. Each of those reaches audit_log as SQL NULL exactly
// as a bare nil does, and only storekit's refusal catches them. That is the
// division of labour this gate's doc comment describes, and the reason the
// refusal is not optional.
func isAbsentImageExpr(call *ast.CallExpr) bool {
	if auditBeforeArg >= len(call.Args) {
		return false
	}
	ident, isIdent := call.Args[auditBeforeArg].(*ast.Ident)
	return isIdent && ident.Name == "nil"
}

// packageConstants collects every package-level string constant in the tree,
// grouped by the directory that declares it. Go puts a package in a directory,
// so the directory is the resolution scope.
//
// By PACKAGE and not by file, because that is where the verbs live that are not
// written as literals. `actionUpdate` is a package-level constant used from
// many files in that package; resolving only a file's own declarations would call
// those sites unresolvable, and an unresolvable site gets RATIFIED. That would
// turn "we could not read this verb" into a standing waiver over sites whose
// verb is plainly `update` — the census failing short into the one bucket that
// forgives it.
func packageConstants(files []gatekit.ParsedFile) map[string]map[string]string {
	byPackage := map[string]map[string]string{}
	for _, parsed := range files {
		dir := filepath.Dir(parsed.Path)
		if byPackage[dir] == nil {
			byPackage[dir] = map[string]string{}
		}
		collectStringConstants(parsed.File, byPackage[dir])
	}
	return byPackage
}

func collectStringConstants(file *ast.File, into map[string]string) {
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				if literal, known := resolveString(value.Values[i], nil); known {
					into[name.Name] = literal
				}
			}
		}
	}
}

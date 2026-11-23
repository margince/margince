// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A path that hangs a child row onto a record (an activity, a note, a task, a
// file) must ask auth.EnsureAttachTarget. That probe needs write authority over
// the record. auth.EnsureLinkTarget and auth.EnsureReferenceTarget ask only whether
// the caller can SEE it. So every function that calls one of the two read
// probes says here why seeing is enough, and a new caller fails until it does.
//
// The reasons fall in four groups, and each entry starts with its group:
//
//   - reference: the shared record is a value in a row the caller writes and
//     is gated on (a company named on a new deal). Nothing on it changes.
//   - capture: automatic filing of mail and meetings the mailbox owner took
//     part in. The record gains the correspondence whoever owns it.
//   - carried: a link the continued activity already holds, so the record
//     already shows that conversation.
//   - read: the function writes nothing on the record; it reads, locks, or
//     pairs the probe with a write probe of its own.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// readProbeCallers ratifies each caller of a read-only record probe, keyed
// package.function, with the group and the reason.
var readProbeCallers = gatekit.Waive(map[string]string{
	"activities.mayCarry": "carried: the one spelling of a link the continued activity holds; insertActivityLinks takes it only for those links",

	"capture.linkActivityToProject":   "capture: files captured mail under a project; gated on activity:update",
	"capture.mayFileOnDeal":           "capture: files captured mail under a deal the mailbox owner can see",
	"capture.mayFileUnder":            "capture: files a captured meeting under its participants' records",
	"capture.Sink.linkActivity":       "capture: files captured mail under the records its addresses resolve to",
	"capture.stampTheProjectOnTheRow": "capture: re-checks the project a captured message is filed under before the stamp",

	"collections.admitMemberChange": "reference: a list entry is the list owner's bookmark and changes nothing on the record; list authority gates the write",
	"collections.holdTagTarget":     "read: holds the record FOR SHARE, then takes auth.EnsureWritableLive itself",

	"commissions.accrueTx": "reference: the commission entry names the deal and partner company; neither changes",

	"consent.refuseUnreadableEvidence": "reference: a consent record cites a deal as evidence; the deal does not change",
	"consent.writeExchangeTx":          "reference: a qualifying exchange names the deal it was about; the deal does not change",

	"contacts.admitAndDedupeCompanyCreate":    "reference: a new company names its parent company",
	"contacts.admitLeadCreate":                "reference: a new lead names its company or partner",
	"contacts.admitLeadReferences":            "reference: a lead update re-points its own company or partner",
	"contacts.appendHandoffEvent":             "reference: an SDR handoff event names the deal it led to",
	"contacts.AttachCompanyToProjectTx":       "reference: the project is the row written, behind its own write probe; the company is its value",
	"contacts.currentEmployerID":              "read: reads which employer of a contact the caller may see",
	"contacts.employmentImportItem":           "read: an import preview names the company a row would match",
	"contacts.ensureHandoffTargetsVisible":    "reference: a handoff names the deal and records it hands over",
	"contacts.ensureLeadPartner":              "reference: a lead names its partner company",
	"contacts.ensureLinkableCompany":          "reference: a lead names its company; the lead is the row written",
	"contacts.ensureRelationshipEndpoints":    "reference: the far endpoints of an edge; the anchor takes ensureRelationshipAnchorWritable",
	"contacts.FillParticipantNamesTx":         "capture: names a meeting attendee from a captured invitation, only on a contact the mailbox owner sees",
	"contacts.LockCompanyRosterTx":            "read: locks the contacts of a company roster read",
	"contacts.matchNamedEmployer":             "reference: the contact's employment names its company; the contact write is gated",
	"contacts.previewMergeHandoffs":           "read: a merge preview names the handoff deals; it commits nothing",
	"contacts.requireEmploymentRead":          "read: an employment read asks whether the contact is visible",
	"contacts.setCompanyCheckedFields":        "reference: a company update re-points its own parent company",
	"contacts.setCompanyRoleTx":               "reference: sets a company's role on a project the caller writes",
	"contacts.Store.attachEmployerTx":         "reference: the contact's employment names its company; the contact write is gated",
	"contacts.Store.ApplyEmployerChangeTx":    "reference: an accepted employer change names the new company; the contact write is held",
	"contacts.Store.DecideHandoff":            "reference: a handoff decision names the deal it produced",
	"contacts.Store.EnsureDealVisible":        "read: a relationship read asks whether the deal is visible",
	"contacts.Store.EnsureProjectVisible":     "read: a relationship read asks whether the project is visible",
	"contacts.Store.matchEmploymentCompany":   "reference: an employment import names the matched company; the contact write is gated",
	"contacts.Store.resolveEmploymentCompany": "reference: an employment import names the chosen company; the contact write is gated",

	"contracts.CompanyContinuityTx":          "read: reads the contracts of a company the caller sees",
	"contracts.createContractTx":             "reference: a new contract names its company and deal",
	"contracts.ensureLinksVisible":           "reference: a contract names its deal, project and company",
	"contracts.Store.ListCompanyContracts":   "read: lists the contracts of a visible company",
	"contracts.Store.readDealContracts":      "read: lists the contracts of a visible deal",
	"contracts.Store.ListProjectContractsTx": "read: lists the contracts of a visible project",

	"deals.admitOfferCreation":           "reference: a new offer names its deal and buyer company",
	"deals.applyCompanyLinkPatch":        "reference: a deal update re-points its own company",
	"deals.applyPartnerAttributionPatch": "reference: a deal update names its partner company",
	"deals.ensureBirthLinksVisible":      "reference: a new deal names its company, contacts and lead",
	"deals.ensureClearedLinksVisible":    "reference: a deal update clears or re-points its own links",
	"deals.offerHeaderPatch":             "reference: an offer update re-points its own buyer company",
	"deals.resolveBuyerCompany":          "reference: an offer names its buyer company",

	"identity.admitOwnerCarry":       "read: pairs the probe with the owner change's own write gate",
	"identity.createRecordGrantInTx": "read: visibility first; auth.EnsureCanGrant then asks for write authority",

	"projects.admitProjectCreation": "reference: a new project names its company",
	"projects.CompanyDeliveryTx":    "read: reads the delivery state of a visible company",

	"signals.CompanySignalsCompleteTx": "read: reads the signals of a visible company",
	"signals.consentedContact":         "read: a resolver reads which contact a signal may name",
	"signals.LockCompanySignalsTx":     "read: locks a visible company while its signals are read",
	"signals.RecordExtensionTx":        "reference: an extension's signal names the record it is about",
	"signals.Store.CreateSignal":       "reference: a signal names the record it is about; signals are their own rows with their own gate",
	"signals.Store.IntroPath":          "read: reads the introduction path to a visible company",
	"signals.visibleCandidates":        "read: a resolver reads which companies a signal may name",
})

// readProbes are the platform/auth functions that ask about sight alone.
var readProbes = map[string]bool{"EnsureLinkTarget": true, "EnsureReferenceTarget": true}

// readProbeGroups are the prefixes an entry's reason starts with.
var readProbeGroups = []string{"reference: ", "capture: ", "carried: ", "read: "}

// wantMinimumReadProbeCallers fails the gate when the walk stops seeing the
// calls. Fifty-nine stand today.
const wantMinimumReadProbeCallers = 40

func TestEveryReadOnlyRecordProbeCallerSaysWhySeeingIsEnough(t *testing.T) {
	t.Parallel()
	defer readProbeCallers.AssertAllMatched(t)
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return len(readProbeCallersIn(file)) > 0 },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	found := map[string]bool{}
	for _, src := range scope.Files(t) {
		for _, caller := range readProbeCallersIn(src.File) {
			if found[caller] {
				continue
			}
			found[caller] = true
			if !readProbeCallers.Waived(t, caller) {
				t.Errorf("%s: %s asks a read-only record probe. A path that hangs a row onto a record asks "+
					"auth.EnsureAttachTarget; anything else is added to readProbeCallers with its group and reason",
					src.Path, caller)
			}
		}
	}
	if len(found) < wantMinimumReadProbeCallers {
		t.Errorf("found %d callers of the read-only record probes, want at least %d: the walk has stopped seeing them",
			len(found), wantMinimumReadProbeCallers)
	}
}

func TestEveryReadProbeWaiverNamesItsGroup(t *testing.T) {
	t.Parallel()
	for caller, reason := range readProbeCallers.Reasons() {
		grouped := false
		for _, group := range readProbeGroups {
			grouped = grouped || strings.HasPrefix(reason, group)
		}
		if !grouped {
			t.Errorf("%s: the reason %q starts with none of %v", caller, reason, readProbeGroups)
		}
	}
}

// readProbeCallersIn names each function in the file that names a read probe
// through the auth package, as package.function or package.Receiver.function.
// platform/auth's own bare calls are the probes composing each other.
func readProbeCallersIn(file *ast.File) []string {
	var callers []string
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc {
			continue
		}
		ast.Inspect(fn, func(node ast.Node) bool {
			selector, isSelector := node.(*ast.SelectorExpr)
			if !isSelector || !readProbes[selector.Sel.Name] {
				return true
			}
			if pkg, isIdent := selector.X.(*ast.Ident); !isIdent || pkg.Name != "auth" {
				return true
			}
			callers = append(callers, file.Name.Name+"."+qualifiedFuncName(fn))
			return false
		})
	}
	return callers
}

// carrySetters ratifies each function that marks an activity as continuing
// another, keyed package.function. A carried link skips the write check
// (activities.mayCarry). So each setter says why the new row carries on from
// that activity, and why no request can choose it.
var carrySetters = gatekit.Waive(map[string]string{
	"activities.anchorThreading":                 "a reply threads onto a mail with a message identity; a note, meeting or task anchor and a new topic carry nothing",
	"activities.outboundMessage.activity":        "the sent row takes the continuation anchorThreading derived, never the caller's anchor",
	"activities.emailRequestTaskInput":           "a reminder to answer a request is filed where the request message is; the message id is the source the store read",
	"activities.Store.LandMeetingNotesTx":        "a capture transcript is filed where its meeting is; the meeting comes from the calendar sync, not a request",
	"activities.LogActivityInput.ContinueFiling": "the one exported setter; its callers are listed here too",
	"compose.writeCommitmentTask":                "a commitment task is filed where the message it was read from is; the message id is the extractor's",
})

// wantMinimumCarrySetters fails the gate when the walk stops seeing setters.
// Six stand today.
const wantMinimumCarrySetters = 4

func TestEveryCarrySetterSaysWhyItsActivityIsCarriedOn(t *testing.T) {
	t.Parallel()
	defer carrySetters.AssertAllMatched(t)
	scope := gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(_ string, file *ast.File) bool {
			return len(carrySettersIn(file)) > 0 || declaresLogActivityInput(file) != nil
		},
		Exempt: gatekit.Waive(map[string]string{}),
	}
	files := scope.Files(t)
	var requestFields map[string]bool
	for _, src := range files {
		if fields := declaresLogActivityInput(src.File); fields != nil {
			requestFields = fields
		}
	}
	if len(requestFields) == 0 {
		t.Fatal("found no exported field of activities.LogActivityInput, so no setter can be checked for a request value")
	}
	found := map[string]bool{}
	for _, src := range files {
		for _, setter := range carrySettersIn(src.File) {
			found[setter.name] = true
			if !carrySetters.Waived(t, setter.name) {
				t.Errorf("%s: %s marks an activity as continuing another, so its links skip the write check. "+
					"Add it to carrySetters with the reason no request can choose that activity", src.Path, setter.name)
			}
			if field := requestValueIn(setter.value, requestFields); field != "" {
				t.Errorf("%s: %s continues an activity named by %s, a field a request fills", src.Path, setter.name, field)
			}
		}
	}
	if len(found) < wantMinimumCarrySetters {
		t.Errorf("found %d carry setters, want at least %d: the walk has stopped seeing them", len(found), wantMinimumCarrySetters)
	}
}

type carrySetter struct {
	name  string
	value ast.Expr
}

// carrySettersIn finds each write of a `continues` field inside package
// activities, the only package that can write it. It also finds each
// ContinueFiling call anywhere, with the value written.
func carrySettersIn(file *ast.File) []carrySetter {
	var setters []carrySetter
	for _, decl := range file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		name := file.Name.Name + "." + qualifiedFuncName(fn)
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.KeyValueExpr:
				if key, isIdent := n.Key.(*ast.Ident); isIdent && key.Name == "continues" && file.Name.Name == "activities" {
					setters = append(setters, carrySetter{name, n.Value})
				}
			case *ast.AssignStmt:
				for i, lhs := range n.Lhs {
					if sel, isSel := lhs.(*ast.SelectorExpr); isSel && sel.Sel.Name == "continues" &&
						file.Name.Name == "activities" && i < len(n.Rhs) {
						setters = append(setters, carrySetter{name, n.Rhs[i]})
					}
				}
			case *ast.CallExpr:
				if sel, isSel := n.Fun.(*ast.SelectorExpr); isSel && sel.Sel.Name == "ContinueFiling" && len(n.Args) == 1 {
					setters = append(setters, carrySetter{name, n.Args[0]})
				}
			}
			return true
		})
	}
	return setters
}

// declaresLogActivityInput answers the exported fields of
// activities.LogActivityInput when the file declares it: the fields a request
// fills.
func declaresLogActivityInput(file *ast.File) map[string]bool {
	if file.Name.Name != "activities" {
		return nil
	}
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, isType := spec.(*ast.TypeSpec)
			if !isType || typeSpec.Name.Name != "LogActivityInput" {
				continue
			}
			structType, isStruct := typeSpec.Type.(*ast.StructType)
			if !isStruct {
				return nil
			}
			fields := map[string]bool{}
			for _, field := range structType.Fields.List {
				for _, name := range field.Names {
					if name.IsExported() {
						fields[name.Name] = true
					}
				}
			}
			return fields
		}
	}
	return nil
}

// requestValueIn names the request field or request value a setter's value
// reads, empty when it reads none.
func requestValueIn(value ast.Expr, requestFields map[string]bool) string {
	found := ""
	ast.Inspect(value, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			if requestFields[n.Sel.Name] {
				found = n.Sel.Name
			}
		case *ast.Ident:
			if n.Name == "req" || n.Name == "request" {
				found = n.Name
			}
		}
		return found == ""
	})
	return found
}

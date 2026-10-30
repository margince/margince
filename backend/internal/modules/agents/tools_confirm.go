// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The consequential-write tool family. Each still implements StageInfo, which
// pins a staged call to the target's CURRENT version — an approval is a
// judgment about the record as the human saw it, never about whatever it
// became since — and that machinery is what a workspace tier floor uses when
// an installation asks for these to be confirmed.
//
// They no longer stage by DEFAULT. A passport carries the granting human's own
// seat, grants and row scope, so a verb this family holds is one its holder
// could perform unaided in the web app, and requiring a second confirmation
// from the same contact made the agent surface weaker than the contact behind
// it rather than safer. This is ADR-0055's argument — already accepted for
// DECIDING an approval — applied to doing the thing itself.
//
// What still bounds a call is what bounds the human: RBAC, row scope, the seat
// ceiling, expiry, and the passport scope its holder chose to lend. An
// installation that wants a verb confirmed sets a tier floor for it — which is
// only true because every tool in this family answers recordTypedTool
// (tierfloor.go); a verb that cannot say which record type a call names takes no
// floor at all, and the default would be the only setting there is.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// --- archive_record (write: runs at once unless a tier floor stages it; hard to undo) ---

// archivableRecordTypes is what SystemOfRecordProvider.Archive actually routes
// (compose/provider.go). It is narrower than datasource.EntityTypes(), which
// includes `lead`, a type Archive does not serve.
//
// Where a tier floor stages this tool, it archives only on the approved retry.
// A type admitted at staging and refused at execution then spends a human's
// approval on a call that could never run. The list is stated rather than
// derived, so a new Archive branch is edited in both.
var archivableRecordTypes = []string{
	string(datasource.EntityContact), string(datasource.EntityCompany),
	string(datasource.EntityDeal), string(datasource.EntityProject),
	string(datasource.EntityRelationship), string(datasource.EntityActivity),
	string(datasource.EntityProduct), string(datasource.EntityOfferTemplate), string(datasource.EntityOffer),
}

// archivableHere answers what the ROUTED executor archives, falling back to the
// native list above when the provider cannot say.
//
// The list above is what the NATIVE provider archives, and a fork's adapter may
// archive fewer types than that. A stage-time check reading the native list
// against such a provider admits an archive the executor was always going to
// refuse — the one failure this tool's confirm-first shape exists to prevent,
// and the failure the comment on archivableRecordTypes describes happening to
// `activity` once already.
//
// The fallback is not a shrug: a provider that does not answer
// RecordArchiverV2 is a fork's own adapter, and the native set is the only
// honest guess about it. What it must NOT do is pretend to be an answer — so
// it is the same list the caller would have used anyway, and nothing
// downstream reads it as a fact about the executor.
func archivableHere(ctx context.Context, p datasource.SystemOfRecordProvider) ([]string, error) {
	archiver, ok := p.(datasource.RecordArchiverV2)
	if !ok {
		return archivableRecordTypes, nil
	}
	types, err := archiver.ArchivableTypes(ctx)
	if err != nil {
		// Surfaced, never absorbed. The answer turns on which mode this
		// installation runs in, and a failed read of that is not a reason to
		// fall back to the WIDER set: doing so would admit the three types
		// this check was added to refuse, and leave the safety resting on a
		// second read of the same value succeeding later.
		return nil, err
	}
	out := make([]string, 0, len(types))
	for _, t := range types {
		out = append(out, string(t))
	}
	return out, nil
}

type archiveArgs struct {
	RecordType string   `json:"record_type"`
	ID         ids.UUID `json:"id"`
}

type archiveRecord struct {
	p        datasource.SystemOfRecordProvider
	language baselanguage.Resolver
}

func (t archiveRecord) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "archive_record", Title: "Archive a record", Version: toolVersionV1,
		AskUserFirst:  true,
		Description:   archiveRecordCopy.render(),
		Instead:       archiveRecordCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "archiveContact/archiveCompany/archiveDeal/archiveProject/archiveRelationship/archiveActivity/archiveProduct/archiveOfferTemplate/archiveOffer",
		InputSchema: schema(`{"type":"object","required":["record_type","id"],"properties":{
			"record_type":{"type":"string","enum":["contact","company","deal","project","relationship","activity","product","offer_template","offer"]},
			"id":{"type":"string","format":"uuid"},
			"approval_id":{"type":"string","format":"uuid","description":"Set on approved retry"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ArchiveResult](),
	}
}

// StageInfo decodes this door's arguments into the archive command and
// delegates: the refusals and the staged subject live in the resolver
// (command.go), where the REST door reaches the same ones for the same
// operation.
//
// This door's wire shape IS the command's field set — the arguments differ
// only in carrying JSON tags — so it converts rather than restating the fields,
// and a command that grows one fails to compile here instead of quietly
// leaving it unset.
//
// ONE check runs HERE, before the command is built, for exactly the reason
// createRecord.StageInfo's does (tools.go): a record type this verb's OWN
// write path cannot express. Handle archives exclusively through
// datasource.SystemOfRecordProvider.Archive — and the surface does not enforce
// the InputSchema enum at this layer, so a raw tool call can put any string
// here. Without it, `record_type:"tag"` stages an approval a human releases
// onto a retry that dies at the provider, and an arbitrary string stages one
// with a target type the approvals surface has no visibility rule for at all:
// a zombie authority object, minted at the caller's choosing.
//
// It asks archivableRecordTypes rather than the seam's whole vocabulary,
// because those two are not the same question: `lead` is a seam entity that
// Archive does not route, so the broader check let it stage and fail after
// approval — the trap `activity` fell into until Archive learned to serve it.
//
// That is a fact about THIS door's executor rather than about the operation,
// which is why it cannot live in the resolver: the same command reaching it
// from REST names an archive whose OWN module's handler performs it fine, and
// archiveResolver.Guards stands down for exactly that reason.
func (t archiveRecord) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	var args archiveArgs
	if err := decodeArgs(in, &args); err != nil {
		return StageInfo{}, err
	}
	if err := refuseUnarchivableType(ctx, t.p, args.RecordType); err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewArchiveCall(t.p, t.language, ArchiveCommand(args)))
}

// refuseUnarchivableType holds the verb to the types the seam actually routes,
// naming the whole set rather than saying the value is invalid.
//
// Both doors call it. It used to sit in StageInfo alone, which was enough while
// the verb always staged — nothing reached Handle without an approval, and the
// approval could not be minted for a type that would fail. Executing directly
// removes that shelter: the refusal has to be here, or `activity` is admitted
// and fails deep in the seam with a message about the system of record. That is
// the defect this refusal was written for in the first place.
func refuseUnarchivableType(ctx context.Context, p datasource.SystemOfRecordProvider, recordType string) error {
	archivable, err := archivableHere(ctx, p)
	if err != nil {
		return err
	}
	if !slices.Contains(archivable, recordType) {
		return &BadArgsError{Cause: fmt.Errorf(
			"this verb does not archive %q records; it archives %s",
			recordType, strings.Join(archivable, ", "))}
	}
	return nil
}

func (t archiveRecord) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args archiveArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if err := refuseUnarchivableType(ctx, t.p, args.RecordType); err != nil {
		return nil, err
	}
	ref, err := archiveAt(ctx, t.p, datasource.EntityRef{Type: datasource.EntityType(args.RecordType), ID: args.ID})
	if err != nil {
		return nil, err
	}
	noteEvidence(ctx, ref.Type, ref.ID)
	return json.Marshal(ArchiveResult{Archived: true, RecordType: ref.Type, ID: ref.ID})
}

// --- promote_lead (🟡 write — graduates a lead into the clean core) ---

// LeadPromoter is the provider extension promotion rides (the sor seam
// has no promotion verb yet — fable feedback/17).
//
// ifVersion carries the version the write must be conditioned on, for the
// reason LeadDisqualifier's does: this verb STAGES a target version at
// approval time, and a promotion that applied none would mint a contact from
// lead fields a concurrent edit may have changed since the human approved.
type LeadPromoter interface {
	PromoteLead(ctx context.Context, id ids.UUID, trigger string, evidenceNote *string, ifVersion *int64) (datasource.EntityRef, bool, error)
}

type promoteArgs struct {
	LeadID       ids.UUID `json:"lead_id"`
	Trigger      string   `json:"trigger"`
	EvidenceNote *string  `json:"evidence_note"`
}

type promoteLead struct {
	p        datasource.SystemOfRecordProvider
	promoter LeadPromoter
	language baselanguage.Resolver
}

func (t promoteLead) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "promote_lead", Title: "Promote a lead to a contact", Version: toolVersionV1,
		AskUserFirst:  true,
		Description:   promoteLeadCopy.render(),
		Instead:       promoteLeadCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp: "promoteLead",
		InputSchema: schema(`{"type":"object","required":["lead_id","trigger"],"properties":{
			"lead_id":{"type":"string","format":"uuid"},
			"trigger":{"type":"string","enum":["inbound_reply","meeting_booked","meeting_held","human_qualify"],
				"description":"The genuine engagement justifying promotion; cold outreach with no reply never promotes"},
			"evidence_note":{"type":"string"},
			"approval_id":{"type":"string","format":"uuid","description":"Set on approved retry"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[PromoteLeadResult](),
	}
}

// StageInfo decodes this door's arguments into the promotion command and
// delegates: the refusals and the staged subject live in the resolver
// (commandlifecycle.go), where the REST door reaches the same ones for the
// same operation.
func (t promoteLead) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	var args promoteArgs
	if err := decodeArgs(in, &args); err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewPromoteLeadCall(t.p, t.language, PromoteLeadCommand{
		LeadID:  args.LeadID,
		Trigger: args.Trigger,
	}))
}

// validTriggers mirrors the contract enum — checked BEFORE staging so a
// forbidden trigger (cold outbound) can never even reach the inbox.
var validTriggers = map[string]bool{
	"inbound_reply": true, "meeting_booked": true, "meeting_held": true, "human_qualify": true,
}

func (t promoteLead) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args promoteArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if err := requireGenuineTrigger(args.Trigger); err != nil {
		return nil, err
	}
	// nil, because this tool takes no if_version of its own: the pin it
	// applies is the one the approval was released against, as every other
	// staged write's is.
	pin, err := pinForWrite(ctx, nil)
	if err != nil {
		return nil, err
	}
	ref, merged, err := t.promoter.PromoteLead(ctx, args.LeadID, args.Trigger, args.EvidenceNote, pin)
	if err != nil {
		return nil, err
	}
	rec, err := t.p.Read(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("crmagents: promotion landed but read-back failed: %w", err)
	}
	noteEvidence(ctx, datasource.EntityLead, args.LeadID)
	return json.Marshal(PromoteLeadResult{Merged: merged, Contact: newWireRecord(ctx, rec)})
}

// --- merge_records (🟡 write — collapses two records into one) ---

type mergeArgs struct {
	RecordType string   `json:"record_type"`
	SourceID   ids.UUID `json:"source_id"`
	TargetID   ids.UUID `json:"target_id"`
}

// mergeableTypes: only contact and company have a merge verb (deals and
// leads leave through their own lifecycle).
var mergeableTypes = map[string]bool{importObjectContact: true, importObjectCompany: true}

// mergeableTypeNames renders the vocabulary above for a refusal, sorted so the
// message is byte-stable across processes rather than following map order.
func mergeableTypeNames() []string {
	names := make([]string, 0, len(mergeableTypes))
	for name := range mergeableTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type mergeRecords struct {
	p        datasource.SystemOfRecordProvider
	language baselanguage.Resolver
	cases    ContactCaseMerger
}

// caseArgs are the arguments of a contact case decision. A call naming
// member_ids decides the case the review screen shows, through its engine.
type caseArgs struct {
	ContactID  ids.UUID   `json:"contact_id"`
	Revision   string     `json:"revision"`
	MemberIDs  []ids.UUID `json:"member_ids"`
	SurvivorID ids.UUID   `json:"survivor_id"`
	OwnerID    ids.UUID   `json:"owner_id"`
	Preview    bool       `json:"preview"`
}

type mergeCallArgs struct {
	mergeArgs
	caseArgs
}

var errMergeNamesOneWay = &BadArgsError{Cause: errors.New(
	"name source_id and target_id to merge two records, or contact_id, member_ids, survivor_id, owner_id " +
		"and revision to merge a contact case, not both")}

// decodeMerge reads one call as a pair merge or a case decision, never both.
func decodeMerge(in json.RawMessage) (mergeArgs, *ContactCaseCommand, error) {
	var args mergeCallArgs
	if err := decodeArgs(in, &args); err != nil {
		return mergeArgs{}, nil, err
	}
	if len(args.MemberIDs) == 0 && args.ContactID.IsZero() {
		return args.mergeArgs, nil, nil
	}
	if !args.SourceID.IsZero() || !args.TargetID.IsZero() {
		return mergeArgs{}, nil, errMergeNamesOneWay
	}
	if args.RecordType != importObjectContact {
		return mergeArgs{}, nil, &BadArgsError{Cause: errors.New("a case merge is record_type contact")}
	}
	cmd := ContactCaseCommand(args.caseArgs)
	return args.mergeArgs, &cmd, nil
}

func (t mergeRecords) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "merge_records", Title: "Merge two records", Version: toolVersionV1,
		AskUserFirst:  true,
		Description:   mergeRecordsCopy.render(),
		Instead:       mergeRecordsCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp: "mergeContact/mergeCompany/mergeContactCase/previewContactCaseMerge",
		InputSchema: schema(`{"type":"object","required":["record_type"],"properties":{
			"record_type":{"type":"string","enum":["contact","company"]},
			"source_id":{"type":"string","format":"uuid","description":"The record merged away (archived, redirected to the survivor)"},
			"target_id":{"type":"string","format":"uuid","description":"The surviving record everything relinks to"},
			"contact_id":{"type":"string","format":"uuid","description":"Any member of a contact case, to merge the case instead of a pair"},
			"member_ids":{"type":"array","items":{"type":"string","format":"uuid"},"description":"The case members to merge into one contact, survivor included"},
			"survivor_id":{"type":"string","format":"uuid","description":"The case member that remains"},
			"owner_id":{"type":"string","format":"uuid","description":"Who owns the remaining contact: the owner of one selected member"},
			"revision":{"type":"string","description":"The case revision you read"},
			"preview":{"type":"boolean","description":"Answer what the case merge would do and write nothing"},
			"approval_id":{"type":"string","format":"uuid","description":"Set on approved retry"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[MergeRecordsResult](),
	}
}

// StageInfo decodes this door's arguments into the merge command and
// delegates: the refusals and the staged subject live in the resolver
// (commandrecord.go), where the REST door reaches the same ones for the same
// operation.
func (t mergeRecords) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	args, decided, err := decodeMerge(in)
	if err != nil {
		return StageInfo{}, err
	}
	if decided != nil {
		return StageSubject(ctx, NewContactCaseCall(t.p, t.language, *decided))
	}
	return StageSubject(ctx, NewMergeCall(t.p, t.language, MergeCommand(args)))
}

func (t mergeRecords) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	args, decided, err := decodeMerge(in)
	if err != nil {
		return nil, err
	}
	if decided != nil {
		return t.decideCase(ctx, *decided)
	}
	// The resolver's own refusal (commandrecord.go), predicate and sentence
	// both: the approved retry re-enters here without passing Guards, so a type
	// the staging refused must be refused again, and read the same way when it
	// is.
	if err := requireMergeableType(args.RecordType); err != nil {
		return nil, err
	}
	ref, err := t.p.Merge(ctx, datasource.MergeInput{
		Type: datasource.EntityType(args.RecordType), SourceID: args.SourceID, TargetID: args.TargetID,
	})
	if err != nil {
		return nil, err
	}
	// BOTH records, not only the one that survived: the source is what was
	// folded in, and an evidence list naming only the survivor describes half of
	// what happened.
	noteEvidence(ctx, ref.Type, ref.ID)
	noteEvidence(ctx, datasource.EntityType(args.RecordType), args.SourceID)
	return json.Marshal(MergeRecordsResult{Merged: true, RecordType: ref.Type, SurvivorID: ref.ID})
}

// decideCase runs the case decision the review screen runs.
func (t mergeRecords) decideCase(ctx context.Context, cmd ContactCaseCommand) (json.RawMessage, error) {
	if err := requireCaseSelection(cmd); err != nil {
		return nil, err
	}
	if t.cases == nil {
		return nil, &BadArgsError{Cause: errors.New("contact cases are not served here")}
	}
	decide := t.cases.MergeContactCase
	if cmd.Preview {
		decide = t.cases.PreviewContactCaseMerge
	}
	outcome, err := decide(ctx, cmd)
	if err != nil {
		return nil, err
	}
	noteEvidence(ctx, datasource.EntityContact, ids.UUID(outcome.SurvivorId))
	for _, merged := range outcome.MergedIds {
		noteEvidence(ctx, datasource.EntityContact, ids.UUID(merged))
	}
	return json.Marshal(MergeRecordsResult{
		Merged: outcome.Committed, RecordType: datasource.EntityContact,
		SurvivorID: ids.UUID(outcome.SurvivorId), Case: &outcome,
	})
}

// recordLabel pulls a human-readable name out of a record's fields for
// inbox summaries; falls back to the id.
func recordLabel(rec datasource.Record) string {
	var f struct {
		FullName    string `json:"full_name"`
		DisplayName string `json:"display_name"`
		Name        string `json:"name"`
		Email       string `json:"email"`
		Kind        string `json:"kind"`
		Subject     string `json:"subject"`
	}
	//craft:ignore swallowed-errors label extraction is best-effort by design — unparseable fields fall through to the id below
	_ = json.Unmarshal(rec.Fields, &f)
	// An edge has no name of any sort: its identity is its kind plus two
	// endpoints, so "Archive relationship 0195c3…" tells the approving human
	// nothing about what disappears, while "employment" at least names the class.
	//
	// Scoped to that ONE type rather than added to the ladder below. `kind` is a
	// field an activity also carries, and there the id is the better answer: a
	// staged overwrite reading `Update activity "note"` would name a class where a
	// human needs to know WHICH note, and would suppress the id that told them.
	if rec.Ref.Type == datasource.EntityRelationship && f.Kind != "" {
		return fmt.Sprintf("%q", f.Kind)
	}
	// An activity's subject is the WHICH the kind cannot supply — "Archive
	// activity 0195c3…" names nothing a human can weigh, while `Archive activity
	// "Kickoff Migration Shopsystem"` is the decision they are being asked for.
	// Also scoped to the one type, and to the field that identifies rather than
	// classifies: an activity carrying no subject still falls through to the id.
	if rec.Ref.Type == datasource.EntityActivity && f.Subject != "" {
		return fmt.Sprintf("%q", f.Subject)
	}
	for _, s := range []string{f.FullName, f.DisplayName, f.Name, f.Email} {
		if s != "" {
			return fmt.Sprintf("%q", s)
		}
	}
	return rec.Ref.ID.String()
}

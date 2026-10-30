// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The canonical V1 CRUD tool set (interfaces.md §2.1), composed over the
// SystemOfRecordProvider seam so the same tools serve whichever provider
// answers for the records. Record-type-generic by design:
// one read_record with a record_type argument, mapping onto the per-type
// contract operations. Writes stamp source="manual" (see ToolSource below);
// captured_by is derived from the authenticated Principal by the store — an
// agent cannot forge provenance any more than a browser can.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents/apps"

	"github.com/margince/margince/backend/internal/shared/kernel/filedpairs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// toolVersionV1 is the version every shipped tool spec carries. Named because
// it appears on every tool in the package: a bare literal repeated two dozen
// times is a value nobody can grep for when the surface finally versions.
const toolVersionV1 = "1.0.0"

// ToolSource is the origin every MCP write carries.
//
// Exported because compose writes one activity on the tool surface's behalf
// (the slipping-deal follow-up) and must stamp the same word; a second literal
// there is how the three spellings this replaced got started.
//
// It is `manual` — the same word the web app writes — because `source` names
// where a row came from, and a tool call is a contact asking for it through an
// assistant rather than through a form. Which door it came through, and who
// walked through it, are recorded in captured_by, where retrieval ranking and
// the record history both read them.
const ToolSource = provenance.RecordSourceManual

// StageResolver supplies the advance_deal tier resolver's input: the
// target stage's configured semantic (won/lost is a property of pipeline
// config, not of the request arguments).
type StageResolver interface {
	StageSemantic(ctx context.Context, stageID ids.UUID) (semantic string, pipelineID ids.UUID, err error)
}

// WithDealTaskSettler injects the seam that answers a closing deal's open
// tasks. A registry composed without one reports none and settles none.
func WithDealTaskSettler(settler DealTaskSettler) RegistryOption {
	return func(r *Registry) { r.closeTasks = settler }
}

// DealTaskSettler answers, and settles, the open tasks a closing deal leaves
// behind. Closing a deal never settles them silently (#7475), so the surface
// that closes it has to be able to ask.
//
// Narrow rather than on the provider port. The read is an activities concern,
// and three fakes assert that port without embedding it. Widening it would put
// stubs for this in modules that never close a deal.
type DealTaskSettler interface {
	// OpenTasksOnDeal lists the deal's open tasks, newest first.
	OpenTasksOnDeal(ctx context.Context, dealID ids.UUID) ([]crmcontracts.DealOpenTask, error)
	// CompleteOpenTasksOnDeal ticks them done through the write the bulk
	// change applies, so the audit rows match the web's "Complete all".
	CompleteOpenTasksOnDeal(ctx context.Context, dealID ids.UUID) ([]ids.UUID, error)
}

// RegisterCoreTools wires the §2.1 CRUD set over one provider: the 🟢
// tools, `advance_deal` (🟢→🟡 dynamic), and the 🟡 confirm-first tools the
// approval loop carries — `archive_record`, `promote_lead` and
// `merge_records`. The two write-shaped §2.2 intents that compose over the
// SAME provider + stage seams — `qualify_lead` and `progress_deal` —
// register here too; the read/draft intents and the lifecycle transitions
// have their own seams (RegisterIntentTools, RegisterSlippingTools,
// RegisterLifecycleTools).
//
// Every verb the contract declares with `x-mcp-tool` is registered by one of
// those functions. A declared verb with no tool is not a gap to describe here:
// TestEveryDeclaredToolVerbIsRegistered fails the build for it.
func RegisterCoreTools(r *Registry, p datasource.SystemOfRecordProvider, stages StageResolver, promoter LeadPromoter, consumerMail ConsumerMail, duplicates OpenDuplicatesFor, tagOffer TagOfferFor) {
	r.Register(readRecord{p: p, name: r.seats})
	r.Register(createRecord{p: p, duplicates: duplicates, tagOffer: tagOffer, language: r.language, words: r.changeWords})
	r.Register(updateRecord{p: p, language: r.language, words: r.changeWords})
	r.Register(logActivity{p: p})
	r.Register(createTask{p: p})
	r.Register(advanceDeal{p: p, stages: stages, language: r.language})
	r.Register(progressDeal{p: p, stages: stages, language: r.language, closeTasks: r.closeTasks})
	r.Register(qualifyLead{p: p, consumerMail: consumerMail})
	r.Register(archiveRecord{p: p, language: r.language})
	r.Register(promoteLead{p: p, promoter: promoter, language: r.language})
	r.Register(mergeRecords{p: p, language: r.language, cases: r.contactCases})
}

// schema turns a hand-written JSON schema into the wire form, COMPACTED.
//
// The compaction is not tidiness. Every spec's InputSchema is rendered
// verbatim into the tool listing that rides every Surface-B prompt
// (runner.ToolListing), so the tabs and newlines these literals are indented
// with are paid for on every run — ~210 tokens across the core surface, out of
// a listing already held to a two-thirds budget of the window.
//
// Doing it here rather than by unindenting forty literals keeps the source
// readable: a schema stays legible where it is written, and costs nothing
// where it is read. A literal that is not valid JSON is left exactly as it
// was, so a malformed schema still fails the test that reads it rather than
// being hidden by this.
func schema(s string) json.RawMessage {
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(s)); err != nil {
		return json.RawMessage(s)
	}
	return json.RawMessage(compact.String())
}

type wireRecord struct {
	RecordType string          `json:"record_type"`
	ID         ids.UUID        `json:"id"`
	Fields     json.RawMessage `json:"fields"`
	Version    int64           `json:"version,omitempty"`
	// TrustTier says whether this installation holds the record or copies it
	// from a system that does. It carries a value on every record, because an
	// absent label means UNKNOWN and a native read is not unknown.
	//
	// It is not the envelope's trust tier and does not answer the same
	// question. A connector-captured row stored here is authoritative and
	// still folds an automated writer's tier onto the envelope. The words come
	// from datasource, which the REST surface reads too.
	TrustTier string `json:"trust_tier,omitempty"`
}

// newWireRecord is the ONE place a datasource.Record becomes MCP tool
// output — every read/search/read-back rides it, so the external trust
// taint is stamped uniformly and can never be silently dropped by one
// call site again.
//
// It takes the context for the same reason it stamps that taint. The result
// envelope has to say what the answer rests on and how fresh it is, and this is
// the one point where the record, its ref and its freshness are all in hand — so
// a tool cannot serve a record without sourcing it, and a tool written next year
// inherits both properties by calling the function it was going to call anyway.
func newWireRecord(ctx context.Context, rec datasource.Record) wireRecord {
	noteRecord(ctx, rec)
	return wireRecord{
		RecordType: string(rec.Ref.Type), ID: rec.Ref.ID, Fields: rec.Fields,
		Version: rec.Version, TrustTier: rec.Freshness.TrustTier(),
	}
}

func listResult(ctx context.Context, name SeatNamer, res datasource.SearchResult) ListRecordsResult {
	records := make([]wireRecord, 0, len(res.Records))
	for _, r := range res.Records {
		records = append(records, newWireRecord(ctx, r))
	}
	return ListRecordsResult{Records: withOwners(ctx, name, records), NextCursor: res.NextCursor}
}

// --- read_record (🟢 read) ---

type readRecord struct {
	p    datasource.SystemOfRecordProvider
	name SeatNamer
}

func (t readRecord) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_record", Title: "Read a record", Version: toolVersionV1,
		Description:   readRecordCopy.render(),
		Instead:       readRecordCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "getContact/getCompany/getDeal/getLead/getActivity/getProject/getPartner/getProduct/getOfferTemplate/getOffer/getCommissionEntry/getDealRoom",
		InputSchema: schema(`{"type":"object","required":["record_type","id"],"properties":{
			"record_type":{"type":"string","enum":["contact","company","deal","lead","activity","project","partner","product","offer_template","offer","commission","deal_room"],"description":"partner is addressed by its COMPANY's id: the row is that company's partner terms, not a separate record."},
			"id":{"type":"string","format":"uuid"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[recordWithOwner](),
	}
}

func (t readRecord) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		RecordType string   `json:"record_type"`
		ID         ids.UUID `json:"id"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	rec, err := t.p.Read(ctx, datasource.EntityRef{Type: datasource.EntityType(args.RecordType), ID: args.ID})
	if err != nil {
		return nil, err
	}
	return json.Marshal(withOwners(ctx, t.name, []wireRecord{newWireRecord(ctx, rec)})[0])
}

// --- create_record (🟢 write, reversible) ---

type createRecord struct {
	p datasource.SystemOfRecordProvider
	// duplicates reports what the create filed for review. Nil where no reader
	// is bound — an installation whose system of record is an overlay mirror has
	// no queue of ours to read — and a nil reader reports nothing rather than
	// failing: silence is what this surface did before, so it is the safe
	// degradation.
	duplicates OpenDuplicatesFor
	// tagOffer says what accepting a proposed tag word would take. Nil where no
	// vocabulary is bound, and a nil seam offers nothing.
	tagOffer TagOfferFor
	language baselanguage.Resolver
	words    ChangeWords
}

func (t createRecord) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "create_record", Title: "Create a record", Version: toolVersionV1,
		Description:   createRecordCopy.render(),
		Instead:       createRecordCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "createContact/createCompany/createDeal/createLead/createProject/createRelationship/createProduct/createOfferTemplate/createOffer/createDealRoom/openDealRoomThread/replyDealRoomThread",
		InputSchema: schema(`{"type":"object","required":["record_type","fields"],"properties":{
			"record_type":{"type":"string","enum":["contact","company","deal","lead","activity","project","relationship","product","offer_template","offer","deal_room","deal_room_thread","deal_room_comment"]},
			"fields":{"type":"object","description":` + jsonString(recordFieldsDescription) + `},
			"offer_tag":{"type":"string","maxLength":64,"description":"A tag word the user said fits this record, such as the event they met at. It offers the tag to the user and applies nothing."},
			"approval_id":{"type":"string","format":"uuid","description":"Set on approved retry"}},
			"additionalProperties":false}`),
		UnkeyedArguments: recordFieldsUnkeyed(),
		OutputSchema:     schemaFor[createdRecord](),
		// The card's choice is a decision about the pair this create filed, so
		// the card is the second renderer of an answer the text already gives.
		// Model-only: a view acts through the tools apps/actions.json names,
		// and creating a record is not one of them.
		UI: &mcp.ToolUI{ResourceURI: apps.CreateFollowupsURI, Visibility: []string{mcp.VisibilityModel}},
	}
}

func (t createRecord) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		RecordType string          `json:"record_type"`
		Fields     json.RawMessage `json:"fields"`
		OfferTag   string          `json:"offer_tag"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if err := rejectUnknownFields(createWriteShapes, args.RecordType, args.Fields); err != nil {
		return nil, err
	}
	ctx, filed := filedpairs.Collect(ctx)
	ref, err := t.p.Create(ctx, datasource.CreateInput{
		EntityType: datasource.EntityType(args.RecordType),
		Fields:     args.Fields,
		Source:     ToolSource,
	})
	if err != nil {
		return nil, err
	}
	rec, err := readBackRecord(ctx, t.p, ref)
	if err != nil {
		return nil, err
	}
	return marshalResult(createdRecord{
		wireRecord:          rec,
		Changes:             t.words.describeCreate(ctx, summaryIn(ctx, t.language).lang, args.Fields, rec.Fields),
		DuplicateCandidates: t.reportCreateDuplicates(ctx, args.RecordType, args.Fields, ref.ID, filed.IDs()),
		TagOffer:            t.offerTag(ctx, args.RecordType, args.OfferTag),
	}, nil)
}

// StageInfo puts a create the contract tightened to confirm-first in the inbox
// instead of dead-ending it (#982).
//
// It decodes this door's arguments into the create command and delegates: the
// refusals and the staged subject live in the resolver (command.go), where the
// REST door reaches the same ones for the same operation. WHAT IT STAGES IS A
// CREATE, and the resolver's shape says so: the record type with no id and no
// version pin, because the record does not exist yet and there is no row an
// approval could bind to. That is the shape the REST door stages for the same
// operation, whose route carries no `{id}` — one operation, one staged shape,
// whichever door the agent came through.
//
// ONE check runs HERE, before the command is even built, rather than inside
// the shared resolver: a record type this verb's OWN write path cannot make
// at all. create_record's Handle writes exclusively through
// datasource.SystemOfRecordProvider.Create, which has no way to express a
// type outside createShapes, so staging one here would mint an approval whose
// approved retry dies at the provider with the authority already spent. That
// is a fact about THIS door's executor — the surface does not enforce the
// InputSchema enum at this layer, so a raw tool call can still name a type
// outside it — and it has no REST equivalent: a REST create for the same
// out-of-schema type reaches its own module's handler, which performs it
// fine, so the identical command asked of the resolver by that door must NOT
// be refused here. createResolver.Guards (command.go) says why it stays
// silent on this question rather than repeating it.
//
// Every other check (unknown fields, the staged shape) runs HERE as well as
// in Handle, and that is the point: the approved retry re-enters through
// Handle, so a rule enforced only there is one a human's yes is spent
// discovering.
// takeParent lifts a parent id out of a create's fields: the route names it in
// its path, a create_record call among its fields.
func takeParent(fields json.RawMessage, name string) (ids.UUID, json.RawMessage, error) {
	var named map[string]json.RawMessage
	if err := json.Unmarshal(fields, &named); err != nil {
		return ids.UUID{}, nil, &BadArgsError{Field: fieldsArg, Cause: fmt.Errorf("fields must be a JSON object: %w", err)}
	}
	var id ids.UUID
	if err := json.Unmarshal(named[name], &id); err != nil || id.IsZero() {
		return ids.UUID{}, nil, &BadArgsError{Field: name, Cause: fmt.Errorf("%s names the record this one is created on", name)}
	}
	delete(named, name)
	rest, err := json.Marshal(named)
	return id, rest, err
}

func (t createRecord) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	var args struct {
		RecordType string          `json:"record_type"`
		Fields     json.RawMessage `json:"fields"`
		OfferTag   string          `json:"offer_tag"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return StageInfo{}, err
	}
	if !t.ServesRecordType(args.RecordType) {
		return StageInfo{}, &BadArgsError{Cause: fmt.Errorf(
			"this verb does not create %q records, so no approval of it could ever be carried out",
			args.RecordType)}
	}
	// An offer is made ON a deal, as POST /deals/{id}/offers makes it, so both
	// doors stage the one command that names the deal.
	if args.RecordType == string(datasource.EntityOffer) {
		dealID, fields, err := takeParent(args.Fields, "deal_id")
		if err != nil {
			return StageInfo{}, err
		}
		return StageSubject(ctx, NewCreateOfferCall(t.p, t.language, CreateOfferCommand{DealID: dealID, Fields: fields}))
	}
	// This door's wire shape IS the command's field set (same reasoning as
	// archiveRecord.StageInfo, command.go), so it converts rather than
	// restating the fields: a field CreateCommand grows fails to compile here
	// instead of quietly leaving it unset. offer_tag is accepted here and left
	// out of the command, yet the staging path still stores the full arguments
	// and hashes every member, so an approval binds the offer too.
	return StageSubject(ctx, NewCreateCall(t.language, CreateCommand(struct {
		RecordType string          `json:"record_type"`
		Fields     json.RawMessage `json:"fields"`
	}{args.RecordType, args.Fields})))
}

// --- log_activity (🟢 write) ---

type logActivity struct {
	p datasource.SystemOfRecordProvider
}

func (t logActivity) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "log_activity", Title: "Log an activity", Version: toolVersionV1,
		Description:   logActivityCopy.render(),
		Instead:       logActivityCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		Unlisted:  true,
		OpenAPIOp: "logActivity",
		// The two vocabularies are SPLICED from the contract, never spelled
		// here: this tool's body IS crm.yaml's CreateActivityRequest, and both
		// hand-written copies had already drifted from it in opposite
		// directions — `kind` was missing the messaging members, so a message
		// the server stores could not be logged; `entity_type` offered a
		// `project` link the contract does not accept, so an agent doing what
		// the schema said was refused.
		//
		// channel_provider is NOT spliceable the same way: it is a deployment
		// fact (ADR-0107/A158), so no generated enum can carry it and the schema
		// describes the rule instead. A schema is documentation, not validation
		// — the refusal that actually binds is the server's.
		InputSchema: schema(`{"type":"object","required":["kind"],"properties":{
			"kind":{"type":"string","enum":` + activityKindEnum + `},
			"channel_provider":{"type":"string","description":"Required when kind is \"message\", else refused; a provider the workspace's channel provider list names."},
			"subject":{"type":"string","description":` + proseLanguageNote + `},
			"body":{"type":"string","description":` + proseLanguageSeeSubject + `},
			"occurred_at":{"type":"string","format":"date-time"` + timestampNote + `},
			"occurred_precision":{"enum":["instant","day"]},
			"direction":{"type":"string","enum":["inbound","outbound"]},
			"due_at":{"type":"string","format":"date-time"` + timestampNote + `},
			"links":{"type":"array","items":{"type":"object","required":["entity_type","entity_id"],"properties":{
				"entity_type":{"type":"string","enum":` + activityLinkEntityTypeEnum + `},
				"entity_id":{"type":"string","format":"uuid"}},"additionalProperties":false},
				"description":"Every record this was about, ALL OF THEM in this call — EXCEPT a project, which this verb REFUSES because filing there marks commercial correspondence. A meeting or a call is with a CONTACT and reaches their company through them — linking one to a company is REFUSED, so name the contact who was there and the company follows from where they work. A meeting linked to the deal alone sits on no attendee's timeline and the company sees nothing. Adding a link AFTERWARDS is a second write — and a later link onto a project stages an approval that waits for the user's yes before it takes effect."},
			"attendee_user_ids":{"type":"array","maxItems":25,"items":{"type":"string","format":"uuid"},
				"description":"Meeting or call only: the workspace members who attended besides you, ids from the colleague roster. The meeting then counts toward each one's relationship with every linked contact. An id that is not an active member is refused."},
			"source_activity_id":{"type":"string","format":"uuid","description":` + followUpSourceDescription + `},
			"follow_up_task":` + string(argumentSchemaFor[crmcontracts.ActivityFollowUpTask](timestampRule)) + `,
			"source_system":{"type":"string"},"source_id":{"type":"string"}},
			"patternProperties":{"^cf_[a-z0-9_]+$":{"description":"An activity custom field, by its cf_ key from describe_vocabulary. A key no active activity field names is refused."}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[wireRecord](),
	}
}

func (t logActivity) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	// The args ARE the contract's create-activity body (minus provenance,
	// which this surface stamps); the provider re-validates strictly.
	ref, err := t.p.Create(ctx, datasource.CreateInput{
		EntityType: datasource.EntityActivity,
		Fields:     in,
		Source:     ToolSource,
	})
	if err != nil {
		return nil, err
	}
	return readBack(ctx, t.p, ref)
}

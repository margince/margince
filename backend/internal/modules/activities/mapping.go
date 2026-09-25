// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Contract request → store input mappings, in ONE place: the HTTP
// handlers and the SoR provider (the MCP surface's door) both decode the
// same crm.yaml shapes, and a defaulting rule that lived in only one of
// them would make the two surfaces silently disagree.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// codeRequired is the machine code for "this value was absent". A constant
// rather than a literal at each of the thirteen refusals in this module that
// answer it: a client branches on the code, and one spelled differently is a
// refusal nothing recognises and nothing reports.
const codeRequired = "required"

// RequiredFieldError maps to 422 on both surfaces.
type RequiredFieldError struct{ Field string }

func (e *RequiredFieldError) Error() string { return e.Field + " is required" }

// FieldFault names the missing required field, on every surface.
func (e *RequiredFieldError) FieldFault() (field, code, message string) {
	return e.Field, codeRequired, e.Error()
}

// ReservedMailIdentityError refuses a client write into the mail identity.
//
// A mail activity's natural key is (connector.EmailSourceSystem, Message-ID),
// and the store replays that key rather than inserting a second row. A caller
// who could spell it would therefore pre-plant a row under a Message-ID and
// have a real capture of that message hand the planted row back as already
// existing — the message's own content never landing, and its timeline links
// resolving to whatever the planter wrote. Only a connector that authenticated
// as one, or this system's own send, may claim a mail identity.
//
// It is refused for EVERY kind, not just `email`: the unique index spans
// kinds, so a planted `note` under a Message-ID suppresses the mail just as
// well.
type ReservedMailIdentityError struct{}

func (e *ReservedMailIdentityError) Error() string {
	return "source_system " + connector.EmailSourceSystem +
		" is reserved for captured and sent mail; omit it, or use a value that names your own system"
}

// FieldFault states the refusal as caller-fixable, reaching the HTTP mapper and
// the tool surface the same way every other field refusal here does.
func (e *ReservedMailIdentityError) FieldFault() (field, code, message string) {
	return "source_system", "reserved_source_system", e.Error()
}

// fieldBody names the activity body field in a FieldFault, the one spelling
// every 422 that refuses something about it shares (mirrors activity.go's
// fieldKind).
const fieldBody = "body"

// transcriptSourceSystem is the ADR-0058 marker: a `logActivity` call
// carrying it identifies its body as pasted/uploaded transcript text rather
// than ordinary meeting notes, which is what routes it through
// normalizeTranscript and what the activity/transcript retention selector
// (privacy/retentionselectors.go) keys its sweep on.
const transcriptSourceSystem = "transcript"

// faultInvalid is the RFC 7807 code every field refusal in this module carries:
// the value is well-formed enough to have reached us and is still not one this
// field accepts.
const faultInvalid = "invalid"

// TranscriptKindError maps to 422: a transcript is a recording of a
// conversation, so it only makes sense on the two activity kinds that ARE
// one. The message never echoes the caller's kind — this fires before the
// kind ever reaches the DB CHECK that would otherwise be the only
// validation of it, so an unbounded value has no other floor here.
type TranscriptKindError struct{ Kind string }

func (e *TranscriptKindError) Error() string {
	return "only a call or meeting activity may carry a transcript"
}

// FieldFault names the offending field; the caller's value is left to the
// wire's own field pointer, not interpolated into the message.
func (e *TranscriptKindError) FieldFault() (field, code, message string) {
	return "kind", faultInvalid, e.Error()
}

// MessageProviderError maps to 422: the two axes must agree. A message names
// the transport that carried it, and nothing else names one (ADR-0107/A158).
//
// It fires BEFORE the database CHECK that enforces the same rule, so a caller
// gets a field fault naming what to fix rather than a 500 from a constraint
// they cannot see. The two are deliberately the same rule stated twice — the
// CHECK is the floor no writer can go under, this is the message a caller can
// act on — and the fitness test holds them against each other.
type MessageProviderError struct{ Kind string }

func (e *MessageProviderError) Error() string {
	if e.Kind == KindMessage {
		return "a message must name the transport that carried it in channel_provider"
	}
	return "only a message may name a channel_provider; this kind did not travel on a transport"
}

// faultNotValidForKind is the code the contract PROMISES for a field that is
// wrong for the kind it was sent with (crm.yaml, Activity and
// CreateActivityRequest). Contract-first: the promise came first, so the code
// emits what it says rather than the module's generic `invalid`.
const faultNotValidForKind = "field_not_valid_for_kind"

// FieldFault names channel_provider either way: it is the field that is wrong
// in both directions, whether it is missing or present when it should not be.
func (e *MessageProviderError) FieldFault() (field, code, message string) {
	return "channel_provider", faultNotValidForKind, e.Error()
}

// MeetingStatusKindError maps to 422: only a meeting has a meeting_status.
// The database CHECK constrains the value's vocabulary but not its pairing
// with the kind, so without this a note carrying `held` would store silently
// and read back as a meeting-shaped fact about something that was not one.
type MeetingStatusKindError struct{ Kind string }

func (e *MeetingStatusKindError) Error() string {
	// The caller's kind is unbounded input; like TranscriptKindError above,
	// the message never echoes it — the field pointer names what to fix.
	return "only a meeting may carry a meeting_status"
}

// FieldFault names meeting_status: it is the field the caller has to drop.
func (e *MeetingStatusKindError) FieldFault() (field, code, message string) {
	return "meeting_status", faultNotValidForKind, e.Error()
}

// refuseKindProviderMismatch holds the kind and the transport against each
// other, in both directions.
func refuseKindProviderMismatch(kind, provider string) error {
	if (kind == KindMessage) == (provider != "") {
		return nil
	}
	return &MessageProviderError{Kind: kind}
}

// pathID asserts a contract path id as entity K's id — the widening
// point between the wire and the typed store surface (the route already
// names the entity, so the assertion lives here, not in the store).
func pathID[K ids.EntityKind](id crmcontracts.Id) ids.ID[K] {
	return ids.From[K](ids.UUID(id))
}

// idArg asserts an optional wire UUID (body field or query parameter)
// as entity K's id; nil stays nil.
func idArg[K ids.EntityKind](u *openapi_types.UUID) *ids.ID[K] {
	if u == nil {
		return nil
	}
	v := ids.From[K](ids.UUID(*u))
	return &v
}

// activityUpdateInput maps the contract patch onto the store's input. Both
// transports go through it — the HTTP handler and the datasource seam's
// Update — so an activity patch cannot mean one thing over REST and another
// over the tool surface, which is the whole point of the seam.
func activityUpdateInput(req crmcontracts.UpdateActivityRequest, ifVersion *int64) UpdateActivityInput {
	return UpdateActivityInput{
		Subject:    req.Subject,
		Body:       req.Body,
		OccurredAt: req.OccurredAt,
		DueAt:      req.DueAt,
		RemindAt:   req.RemindAt,
		AssigneeID: idArg[ids.UserKind](req.AssigneeId),
		IsDone:     req.IsDone,
		// Carried, not checked here: the kind this must pair with belongs to the
		// stored row, and only the store has read it. UpdateActivity refuses the
		// mismatch with the same MeetingStatusKindError create raises.
		MeetingStatus: meetingStatusArg(req.MeetingStatus),
		IfVersion:     ifVersion,
	}
}

// meetingStatusArg unwraps the contract's enum into the store's plain string.
func meetingStatusArg(status *crmcontracts.UpdateActivityRequestMeetingStatus) *string {
	if status == nil {
		return nil
	}
	s := string(*status)
	return &s
}

// LogActivityInputFrom maps the contract's create request onto the store's
// input. It is exported for the composition layer, which hands an extension's
// core write to LogActivityTx and must map it the way the HTTP handler does —
// a second mapping written beside this one would be a second set of rules
// about the reserved import namespace, and the two would drift.
func LogActivityInputFrom(req crmcontracts.CreateActivityRequest) (LogActivityInput, error) {
	return logActivityInput(req, provenanceAdmission{})
}

// provenanceAdmission says which reserved identities THIS writer may spell.
//
// Two writers, two disjoint sets, and neither is "the reserved names": the
// engine stamps its quiet-account reminders, the importer stamps its own
// namespace, and each must stay refused the other's. A single boolean would
// have opened both sets to whichever writer asked.
//
// The zero value admits nothing, so a door that says nothing is the client
// door — the safe default, and what every existing caller gets.
type provenanceAdmission struct{ engineReminder, importer bool }

// admits answers whether this writer may spell one reserved source system.
func (a provenanceAdmission) admits(sourceSystem string) bool {
	if a.engineReminder && provenance.EngineReminderSource(sourceSystem) {
		return true
	}
	return a.importer && provenance.ImporterNamespace(sourceSystem)
}

// LogActivityInputFromImporter is LogActivityInputFrom for a declared
// importer: a human holding import_run:create, decided by
// auth.DeclaredImporter at the HANDLER and nowhere else.
//
// Handler-only is the whole boundary. provider.go, handlers_task.go and
// compose/extcore.go keep the client door, so an agent or an extension holding
// its human's grants cannot reach this one —
// activities/provider_reminderidentity_test.go pins that at the provider seam.
func LogActivityInputFromImporter(req crmcontracts.CreateActivityRequest) (LogActivityInput, error) {
	return logActivityInput(req, provenanceAdmission{importer: true})
}

// logActivityInputAllowingReminderIdentity is LogActivityInputFrom for the
// automation engine's own reminder write, which must stamp an identity no
// client may spell. provider.go's logInputForPrincipal decides who reaches it,
// on the principal rather than on anything in the request.
//
// One body behind both, because the guard is the only difference: a second
// mapping beside this one would be a second set of rules about the reserved
// namespace, and the two would drift.
func logActivityInputAllowingReminderIdentity(req crmcontracts.CreateActivityRequest) (LogActivityInput, error) {
	return logActivityInput(req, provenanceAdmission{engineReminder: true})
}

// refuseReservedProvenance guards the two provenance fields on a create wire.
//
// The importer's namespace is not a client's to write: this store keys its
// idempotent replay on (source_system, source_id), so a caller who could spell
// the reserved prefix could pre-plant a row under an incumbent record id and
// have a later import hand it back as already existing
// (provenance.ReservedSourceSystemPrefix).
//
// The admission is narrow on BOTH sides: the engine may spell its reminder
// identities and never the importer's namespace, the importer its namespace
// and never a reminder identity.
func refuseReservedProvenance(req crmcontracts.CreateActivityRequest, adm provenanceAdmission) error {
	if req.SourceSystem != nil {
		if !adm.admits(*req.SourceSystem) {
			if err := provenance.Refuse("source_system", *req.SourceSystem); err != nil {
				return err
			}
		}
		// Outside the admission deliberately: `email` is the one identity every
		// mail transport shares, and no writer — the importer included — may
		// claim it on this wire.
		if *req.SourceSystem == connector.EmailSourceSystem {
			return &ReservedMailIdentityError{}
		}
	}
	// `source` is nobody's to forge, and the admission says nothing about it.
	return provenance.Refuse("source", req.Source)
}

func logActivityInput(req crmcontracts.CreateActivityRequest, adm provenanceAdmission) (LogActivityInput, error) {
	if req.Kind == "" {
		return LogActivityInput{}, &RequiredFieldError{Field: "kind"}
	}
	if err := refuseReservedProvenance(req, adm); err != nil {
		return LogActivityInput{}, err
	}
	in := LogActivityInput{
		Kind:         string(req.Kind),
		Subject:      req.Subject,
		Body:         req.Body,
		OccurredAt:   req.OccurredAt,
		DueAt:        req.DueAt,
		RemindAt:     req.RemindAt,
		SourceSystem: req.SourceSystem,
		SourceID:     req.SourceId,
		Source:       req.Source,
		AssigneeID:   idArg[ids.UserKind](req.AssigneeId),
	}
	if err := optionalFieldsFrom(req, &in); err != nil {
		return LogActivityInput{}, err
	}
	if err := refuseKindProviderMismatch(in.Kind, in.ChannelProvider); err != nil {
		return LogActivityInput{}, err
	}
	if in.SourceSystem != nil && *in.SourceSystem == transcriptSourceSystem {
		if in.Kind != string(crmcontracts.ActivityKindCall) && in.Kind != string(crmcontracts.ActivityKindMeeting) {
			return LogActivityInput{}, &TranscriptKindError{Kind: in.Kind}
		}
		var body string
		if in.Body != nil {
			body = *in.Body
		}
		normalized, err := normalizeTranscript(body)
		if err != nil {
			return LogActivityInput{}, err
		}
		in.Body = &normalized
	}
	if err := importedProvenanceFrom(req, &in); err != nil {
		return LogActivityInput{}, err
	}
	return in, nil
}

// optionalFieldsFrom carries the create wire's optional fields onto the input,
// with the two kind rules that only apply when their field is present: a
// request acceptance belongs to a task, a meeting status to a meeting.
//
// Apart from logActivityInput because they are one group — every branch here is
// "the caller said nothing, so leave it alone" — and holding them beside the
// required fields put more of one function in a reader's head than the mapping
// itself needed.
func optionalFieldsFrom(req crmcontracts.CreateActivityRequest, in *LogActivityInput) error {
	if req.RequestActivityId != nil {
		if string(req.Kind) != string(crmcontracts.ActivityKindTask) {
			return &RequestAcceptanceFieldError{Field: "request_activity_id", Message: "Only a task can accept a request."}
		}
		id := ids.UUID(*req.RequestActivityId)
		in.RequestActivityID = &id
	}
	// The caller states the transport; nothing infers it. The predecessor of this
	// read the provider back out of the kind, which was only ever a translation of
	// an input shape that could not say what it meant — and since ADR-0107/A158 the
	// kind does not name a transport at all.
	if req.ChannelProvider != nil {
		in.ChannelProvider = *req.ChannelProvider
	}
	if req.Direction != nil {
		d := string(*req.Direction)
		in.Direction = &d
	}
	if req.MeetingStatus != nil {
		if in.Kind != string(crmcontracts.ActivityKindMeeting) {
			return &MeetingStatusKindError{Kind: in.Kind}
		}
		m := string(*req.MeetingStatus)
		in.MeetingStatus = &m
	}
	// Only a meeting has a host to name. A caller who sends one on a mail or a
	// task is refused rather than having it dropped: the field they meant to set
	// is the wrong one for what they are logging, and silently ignoring it leaves
	// them believing an attribution that was never written.
	//
	// Saying nothing means "I held it": the store fills in the acting human for
	// a meeting with no host named. There is deliberately no way to say "a
	// meeting nobody here hosted" on the create wire — that state exists on the
	// row for imports whose calendar named no owner, and a human logging a
	// meeting they were at is the case this path serves.
	if req.HostUserId != nil {
		if in.Kind != string(crmcontracts.ActivityKindMeeting) {
			return &KindFieldError{Field: "host_user_id", Only: "a meeting"}
		}
		in.HostUserID = idArg[ids.UserKind](req.HostUserId)
	}
	if req.Links != nil {
		for _, link := range *req.Links {
			in.Links = append(in.Links, ActivityLinkInput{
				EntityType: string(link.EntityType),
				EntityID:   ids.UUID(link.EntityId),
			})
		}
	}
	return nil
}

// importedProvenanceFrom takes what an importer keeps with a record it carried
// across: the source system's own copy, how long it took, and the message's own
// identity and addresses.
//
// All of it was accepted on the wire and dropped before the store, which is the
// defect this path closes.
func importedProvenanceFrom(req crmcontracts.CreateActivityRequest, in *LogActivityInput) error {
	in.Raw = req.Raw
	if req.DurationSeconds != nil {
		// The contract promises field_not_valid_for_kind for this, and only the
		// two kinds that occupy a span of time have a duration to state.
		if in.Kind != KindMeeting && in.Kind != string(crmcontracts.ActivityKindCall) {
			return &KindFieldError{Field: "duration_seconds", Only: "a meeting or call"}
		}
		in.DurationSeconds = req.DurationSeconds
	}
	return mailIdentityFrom(req, in)
}

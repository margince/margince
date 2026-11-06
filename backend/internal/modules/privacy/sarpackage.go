// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// SARPackage is the assembled export. Sections hold raw row maps —
// the package is a data handover, not an API shape.
type SARPackage struct {
	ReportingContributions []map[string]any `json:"reporting_contributions"`
	// ReportingQuestionAnswers are the kept question answer rows a record
	// about the subject was counted in.
	ReportingQuestionAnswers []map[string]any `json:"reporting_question_answers"`
	// ReportingQuestionBindings are kept report card filters naming the subject.
	ReportingQuestionBindings []map[string]any `json:"reporting_question_bindings"`
	Subject                   map[string]any   `json:"subject"`
	Emails                    []map[string]any `json:"emails"`
	Phones                    []map[string]any `json:"phones"`
	// The messaging-channel accounts bound to the subject: which provider
	// identity writes as them, the handle it carries, whether they have blocked
	// this installation's bot, and whether the binding is still live.
	ChannelIdentities []map[string]any `json:"channel_identities"`
	// Which conversations the subject was recorded as being IN, and in what
	// role (ACT-DDL-3). Distinct from Activities, which is what was said: this
	// is the record that they were a party to it at all, and it is held about
	// them whether or not they were ever a contact.
	InteractionParticipation []map[string]any `json:"interaction_participation"`
	// Where the subject appears in a colleague's imported LinkedIn network.
	// They never consented to that import and would have no way to know of it.
	LinkedInConnections []map[string]any `json:"linkedin_connections"`
	Relationships       []map[string]any `json:"relationships"`
	Deals               []map[string]any `json:"deals"`
	Leads               []map[string]any `json:"leads"`
	Activities          []map[string]any `json:"activities"`
	Attachments         []map[string]any `json:"attachments"`
	Consent             []map[string]any `json:"consent"`
	ConsentEvents       []map[string]any `json:"consent_events"`
	// The deterministic thing on the record that made business correspondence
	// lawful, which the consent state alone does not say.
	ConsentQualifyingEvents []map[string]any `json:"consent_qualifying_events"`
	// Why this contact exists: the answer to Art. 15(1)(g).
	AcquisitionEvidence []map[string]any `json:"acquisition_evidence"`
	NoticeCases         []map[string]any `json:"notice_cases"`
	// Every confirm link the workspace mailed this subject, and what became of
	// it: issued, opened, answered, or left to expire. The mail carrying the
	// link is already in Activities; this says what the link itself did, which
	// is the half that decides whether a grant on file is one the subject
	// actually completed.
	//
	// Neither the token nor the address it went to appears here — see
	// sarConsentLinkSections for why each is withheld.
	ConsentLinks []map[string]any `json:"consent_links"`
	// What the subject themselves sent through their confirm link — a
	// correction they typed, or a request to be removed. The one part of this
	// package the subject authored rather than the workspace, which is exactly
	// why an export that omitted it would be answering the wrong question.
	ConfirmSubmissions []map[string]any `json:"confirm_submissions"`
	// The rights requests this subject made, and what happened to them.
	//
	// Art. 15 asks what is held about somebody, and a controller holding a
	// record of their erasure request holds something about them. Omitting it
	// meant a subject who asked to be erased, and was refused, could not see
	// that the refusal existed — the one thing they would need to appeal it.
	RightsRequests []map[string]any `json:"rights_requests"`
	RequestProofs  []map[string]any `json:"request_proofs"`
	// The unsubscribe links minted for this subject, which are capabilities
	// over their own record and live two years.
	//
	// The TOKEN is never here: the row holds a hash and the plaintext exists
	// only in the mail that carried it, which is the whole point of hashing it.
	// What the subject learns is that links exist, what each can stop, and
	// whether any has been used — the questions somebody asks after finding an
	// old message and wondering whether the link in it still works.
	WithdrawalCredentials []map[string]any `json:"withdrawal_credentials"`
	// Why each message to this subject was permitted, what non-consent basis
	// stood behind it, and every objection or restriction they recorded.
	// Art. 15(1)(a)-(c): the purposes of the processing and its lawful ground,
	// answered per message rather than in the abstract.
	CommunicationDecisions   []map[string]any `json:"communication_decisions"`
	CommunicationBases       []map[string]any `json:"communication_bases"`
	CommunicationSuppression []map[string]any `json:"communication_suppression"`
	// The standing vouches a rep recorded that a machine-level refusal for one
	// category may be overruled for this subject — not consent, not a lawful
	// basis, but a human decision on the record. Owed for the same reason the
	// suppression above is: a subject asking what is held about them is owed
	// the record that a human decided to write to them anyway, and why.
	CommunicationOverrides []map[string]any `json:"communication_overrides"`
	// The times a named contact decided a message to this subject went out
	// DESPITE a refusal. Art. 15 owes what is held, and a subject asking why
	// they received something the installation had refused is owed the override
	// as much as the refusal — an export showing only the second describes a
	// message that never went.
	//
	// It carries the decision, never the director's own sentence about them:
	// the explanation is erased with the subject, so the export reads it back
	// as the tombstone it becomes.
	CommunicationExceptions []map[string]any `json:"communication_exceptions"`
	RawCapture              []map[string]any `json:"raw_capture"`
	CapturePartySources     []map[string]any `json:"capture_party_sources"`
	FieldOrigins            []map[string]any `json:"field_origins"`
	// EnrichedFields is what the system read about the subject from a public
	// page or a mail signature, each with the verbatim text it came from.
	// Art. 15(1)(g) makes the source itself disclosable, and the snippet IS
	// the source.
	EnrichedFields []map[string]any `json:"enriched_fields"`
	// Corrections is what a human recorded over what the system inferred
	// about this subject. It is theirs twice over: the value was typed by a
	// contact about them, and the suppressions are the record of which claims
	// this installation has agreed to stop making.
	Corrections []map[string]any `json:"corrections"`
	// ReplyJudgements is what this installation concluded the subject's own
	// messages MEANT — whether each reply read as positive, negative or
	// neutral — and every human correction of that conclusion. A verdict is a
	// claim about the subject derived from their words, so Art. 15 owes it for
	// the same reason Corrections above is owed: the subject is entitled to
	// know not only what they wrote but what we decided it meant, and which
	// classifier decided it.
	ReplyJudgements []map[string]any `json:"reply_judgements"`
	// RequestSettlements is what this installation concluded our own replies
	// DID about what the subject asked of us: whether the answer settled their
	// request, and where it did not, the sentence naming what we still owe
	// them. Owed for ReplyJudgements' reason and one of its own — the subject
	// is entitled to know what we decided their message meant, and here that
	// decision can be an outstanding obligation TO them, which is a thing held
	// about somebody that they would have no other way to discover.
	RequestSettlements   []map[string]any `json:"request_settlements"`
	CommitmentAdmissions []map[string]any `json:"commitment_admissions"`
	// StageEvidence is what a model or a record concluded about a deal from the
	// subject's messages. It carries the quoted words, whether they agreed or
	// proposed, and how sure the writer was. Owed on ReplyJudgements' ground, because it
	// is a conclusion drawn from what they wrote.
	StageEvidence []map[string]any `json:"stage_evidence"`
	// Handoffs is each time this contact was passed from one seat to another as a
	// prospect, what was decided, and why. Art. 15 owes it twice over: the note
	// is what one colleague wrote ABOUT them, and the decision is one colleagues
	// made about whether they were worth working at all.
	//
	// That this section exists at all is held by:
	// TestErasureAndSARReachEveryPIITable (backend/gates/piicoverage_test.go),
	// which fails when a table registered sarRead has no section here.
	Handoffs []map[string]any `json:"handoffs"`
	// HandoffHistory is how each of those handoffs reached where it stands. The
	// current status conceals a prospect sent back once before somebody took
	// them, and that round is as much a decision about the subject as the last
	// one is.
	HandoffHistory []map[string]any `json:"handoff_history"`
	// ListMemberships is each Shortlist the subject is on, by name, with who
	// chose them, when and the note on why. ListMembershipHistory is every
	// time they were added or taken off one, or seen joining or leaving a
	// Live List; a removal also keeps the note the membership carried.
	// LiveListMemberships is each Live List whose last check held them, and
	// since when.
	ListMemberships       []map[string]any `json:"list_memberships"`
	ListMembershipHistory []map[string]any `json:"list_membership_history"`
	LiveListMemberships   []map[string]any `json:"live_list_memberships"`
	// ProviderClaims is what a licensed data provider asserted about the
	// subject and this installation retained — bought from a third party
	// rather than given by them, which is precisely the holding Art. 15(1)(g)
	// makes the SOURCE of disclosable too.
	ProviderClaims []map[string]any `json:"provider_claims"`
	// ProviderAppliedFields is what those purchases put ON the record: which
	// field each run filled, and for a plain column the value it wrote. A
	// subject asking what we hold is owed the fact that a purchase changed
	// their record, not only that we made one.
	ProviderAppliedFields []map[string]any `json:"provider_applied_fields"`
	// ProviderRuns is why and when that purchase happened: which provider was
	// asked, what was requested, what came back. Art. 15(1)(a)-(d) asks for
	// the purposes and the categories, and a values-only export would answer
	// what we hold while hiding that we went out and bought it.
	ProviderRuns []map[string]any `json:"provider_runs"`
	// What capture decided about the subject's own address, and why — an
	// automated decision the subject is owed sight of (CAP-DDL-8).
	CaptureDispositions []map[string]any `json:"capture_dispositions"`
	// The governed outbound messages addressed to the subject: what was sent
	// to them, when, and whether it left (comms_outbound).
	MeetingProposals   []map[string]any `json:"meeting_proposals"`
	MeetingInvitations []map[string]any `json:"meeting_invitations"`
	SentMessages       []map[string]any `json:"sent_messages"`
	// The messages addressed to the subject that have NOT been sent: waiting
	// for their moment, withdrawn, or held for a human (scheduled_send,
	// ADR-0104). Held apart from SentMessages because the distinction is the
	// subject's to know — one is a message they received, the other is one
	// somebody wrote to them that the system is still holding.
	ScheduledMessages []map[string]any `json:"scheduled_messages"`
	// The messages a rep started to the subject and has not sent or scheduled:
	// the composer's saved drafts (mail_draft). Apart from ScheduledMessages
	// because nobody has decided to send these at all.
	DraftMessages []map[string]any `json:"draft_messages"`
	// The messages written to the subject that nobody has DECIDED yet: an
	// automation composed them and they are waiting in somebody's approval
	// inbox. Held apart from ScheduledMessages for the same reason that
	// is held apart from SentMessages — the distinction is the subject's to
	// know. One is a message with a moment already chosen; this is one a
	// colleague has not yet agreed to send at all, and may never.
	StagedMessages []map[string]any `json:"staged_messages"`
}

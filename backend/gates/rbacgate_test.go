// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// The store-entry-point admission rule as a fitness function: every
// exported method on a module's *Store or *Service — the seam both the
// HTTP handlers and the MCP tool surface call through — references the
// platform auth gate (object RBAC and/or the row-scope spellings),
// directly or through a same-package helper. A store method without one
// is an ungoverned door into tenant data: reachable by any transport
// wired to it, invisible to review. Row-scope composition itself stays
// a call-site obligation until it moves into the database (the ADR
// direction); this gate pins the half that is statically checkable.
//
// Gatedness is resolved transitively over same-package calls, matched
// by name within the RECEIVER the entry point is declared on, plus the
// package-level functions every receiver may call. Bucketing by receiver
// is what stops an unrelated same-named method from vouching for a store:
// *Store and Handlers in one module routinely spell the same names
// (GetActivity, ListActivities, SendEmail), and a flat by-name index let
// the handler's gate answer for the store's.
//
// Optimism is kept where dispatch is genuinely unresolvable rather than
// merely unbucketed: a name held by BOTH the receiver and the package
// level merges, because a bare `foo(...)` and a `s.foo(...)` are the same
// token in the index this gate builds, so it cannot tell which was meant.
//
// Exceptions are explicit, keyed by "package-dir:FuncName", each with
// the rationale that ratified it; a reasonless or stale waiver fails. The
// key stays coarser than the resolution above — it names the FUNCTION, not
// the receiver — so if two receivers in one package ever hold the same
// ungated name, one waiver ratifies both. No pair does today, and the
// stale-waiver check would not notice if one appeared, so a receiver-keyed
// waiver is the change to make when one does.
//
// The tree the gate reads is itself proven rather than assumed:
// storeEntryPointScope sweeps the whole module for the same entry-point
// shape and reports any that lies outside internal/modules, so a store
// that grows in another tier fails this gate instead of falling out of
// its reach unnoticed.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// ungatedEntryPoints are the ratified auth-free store/service methods.
//
// This list is not bookkeeping. It is the enumeration of every place a read or
// a write reaches the database without an authorization gate, and there is
// NOTHING BENEATH IT: `platform/auth` is the only thing deciding who may see
// what (ADR-0091 §3). Several of the rationales below used to lean on
// row-level security as a tenant backstop — not all of them did, since some
// name tables that never carried it — and every one that did has been re-read
// and now states what actually bounds the call: a CAS on an id the admitted
// job carries, a predicate written in the SQL, an aggregate that returns no
// row, or authentication itself.
//
// So an entry added here is a security decision, and the reason has to survive
// being read on its own: "a sweep runs it" is not one, "no row leaves this
// call" is.
var ungatedEntryPoints = gatekit.Waive(map[string]string{ // #nosec G101 -- waiver rationales for the fitness gate, not credentials
	"internal/modules/knowledge:ReconcileHandbook":          "brings the shipped handbook corpus in line with the pages THIS BINARY carries, reached from nothing but the api boot step, which runs under a system principal holding no object grants at all \u2014 so there is no human grant a gate here could consult, and adding one would refuse the only caller rather than protect anything. Nothing about its subject is caller-chosen: the pages come from the embedded filesystem compiled into the binary, and every row it touches is found by `managed_source = 'handbook'`, so a corpus or document a person created is unreachable from here by construction \u2014 TestTheReconciliationLeavesAWorkspacesOwnCorpusAlone holds that. The tenant is bound the way every boot write is: bootLedgerScope resolves the installation's workspace and binds the transaction to it, and the boot cannot pick another. It returns a COUNT of pages written, never a row and never any prose",
	"internal/modules/knowledge:EmbedDocument":              "gives one document's passages their vectors, reached from the ingest worker and from the drift sweep, both of which run under a system principal holding no object grants. There is no human grant to consult and no row a caller chose: it takes a document id the calling job already read from a row, reads only that document's passages, and writes only vectors back onto them. It exposes NOTHING — no text leaves, no row is returned, and the count it answers is a number of rows touched. A gate here would refuse the only two callers and protect no one. Reading what a passage says is the ask's business, and the ask is gated",
	"internal/modules/knowledge:SweepAbandonedIngests":      "closes the ingests that stopped without saying so \u2014 a document left `running` by a process that died on its LAST attempt, which River cannot rescue because there is no attempt left to run. Reached only from the periodic drift sweep, under the same system principal and the same workspace binding as SweepCorpusDrift beside it, and bounded the same way: the sweep's args declare one workspace and workspaceJobCtx binds the transaction to it. It takes no id and no caller-chosen value \u2014 its subject is derived entirely from the database's own clock against ingest_started_at \u2014 and it returns a count, never a row. A gate here would refuse the only caller and protect nothing; what it protects AGAINST is a corpus permanently unaskable because one upload's worker was killed",
	"internal/modules/knowledge:SweepCorpusDrift":           "re-embeds the workspace's passages whose vectors were computed under a binding that is no longer live. It repairs an index over the WHOLE workspace rather than one caller's row scope \u2014 the same posture as search's SweepWorkspaceEmbeddingDrift, which it rides beside \u2014 so a row-scope gate would be the wrong shape and an object gate would refuse the periodic worker that is its only caller. The tenant is bound as every job's is: the sweep's args declare one workspace and workspaceJobCtx binds the transaction to it. Like EmbedDocument it returns no row and no text, only a count of what it repaired",
	"internal/modules/knowledge:BeginIngest":                "the ingest job's own lifecycle, reached from nothing but the knowledge_ingest worker, which runs under a system principal with no object grants at all — so there is no human grant a gate here could consult, and adding one would refuse the worker rather than protect anything. What bounds the call is the DOCUMENT ID the admitted job carries: every statement is keyed on it, and the job is only ever enqueued inside the transaction that writes that row, by an upload that HAS passed knowledge_document:create. The tenant is bound the same way every job's is — the args declare one workspace, workspaceJobCtx binds the transaction to it, and a worker cannot pick its own. Anything a person may do to a corpus document — upload it, list it, delete it — is a separate gated entry point. BeginIngest additionally writes nothing a caller chose: it moves the row to running and deletes what the previous attempt left",
	"internal/modules/knowledge:WriteChunks":                "the ingest job's own lifecycle, reached from nothing but the knowledge_ingest worker, which runs under a system principal with no object grants at all — so there is no human grant a gate here could consult, and adding one would refuse the worker rather than protect anything. What bounds the call is the DOCUMENT ID the admitted job carries: every statement is keyed on it, and the job is only ever enqueued inside the transaction that writes that row, by an upload that HAS passed knowledge_document:create. The tenant is bound the same way every job's is — the args declare one workspace, workspaceJobCtx binds the transaction to it, and a worker cannot pick its own. Anything a person may do to a corpus document — upload it, list it, delete it — is a separate gated entry point. WriteChunks re-reads the document's archived_at under a row lock before it writes, so an archive that raced the attempt still wins",
	"internal/modules/knowledge:FinishIngest":               "the ingest job's own lifecycle, reached from nothing but the knowledge_ingest worker, which runs under a system principal with no object grants at all — so there is no human grant a gate here could consult, and adding one would refuse the worker rather than protect anything. What bounds the call is the DOCUMENT ID the admitted job carries: every statement is keyed on it, and the job is only ever enqueued inside the transaction that writes that row, by an upload that HAS passed knowledge_document:create. The tenant is bound the same way every job's is — the args declare one workspace, workspaceJobCtx binds the transaction to it, and a worker cannot pick its own. Anything a person may do to a corpus document — upload it, list it, delete it — is a separate gated entry point. FinishIngest only closes a row this attempt opened",
	"internal/modules/knowledge:FailIngest":                 "the ingest job's own lifecycle, reached from nothing but the knowledge_ingest worker, which runs under a system principal with no object grants at all — so there is no human grant a gate here could consult, and adding one would refuse the worker rather than protect anything. What bounds the call is the DOCUMENT ID the admitted job carries: every statement is keyed on it, and the job is only ever enqueued inside the transaction that writes that row, by an upload that HAS passed knowledge_document:create. The tenant is bound the same way every job's is — the args declare one workspace, workspaceJobCtx binds the transaction to it, and a worker cannot pick its own. Anything a person may do to a corpus document — upload it, list it, delete it — is a separate gated entry point. FailIngest has TWO callers and the record names both: the ingest worker, once River's attempts are exhausted, and SweepAbandonedIngests, for a document whose worker died on its LAST attempt \u2014 where no attempt was ever exhausted because there was nothing left to rescue. Both run under the same system principal and the same workspace binding, so the waiver holds for both; a rationale naming one of them would be a ratification record that is quietly false",
	"internal/modules/people:RetractCaptureOnlyPersonTx":    "withdraws a contact CAPTURE created, once a classifier's verdict disowned it. It has THREE callers and the record names all of them: the confidentiality verdict engine's apply, for a thread judged the mailbox owner's private life; the counterparty verdict engine's noise arm, for a sender judged newsletter, transactional or spam; and the link-reconcile retraction sweep, for senders whose noise verdict or standing keep_out predates that arm. Every one runs under a system principal holding no object grants — the two engines inside the transaction that carries the verdict, the sweep in a per-row transaction that first re-reads the answer that selected the contact (NoiseJudgedStandsTx) — so there is no human grant a gate here could consult, and adding one would refuse every caller. Nothing about its subject is caller-chosen in the sense a gate protects: each caller derives WHICH records from capture's own tables, and this refuses every record that is not narrowly its own to withdraw — a human-created one, one a human has edited, one already promoted to the workspace, or one belonging to another seat. It archives, which is reversible and audited: archivePersonRows lands the write shape, so a retraction leaves the same audit row a human's archive would. It returns a bool, never a row and never any of the person's data",
	"internal/modules/people:CaptureOnlyHoldersOfAddressTx": "the candidate scan feeding RetractCaptureOnlyPersonTx: which capture-created, still owner-scoped people hold this address. Reached only from the counterparty verdict engine's noise arm, on the verdict's own transaction, under a system principal holding no object grants — no human grant exists for a gate to consult, and one would refuse the only caller. Its subject is not caller-chosen in the sense a gate protects: the address comes off the ledger row being resolved, which capture wrote from the message's own headers. It returns ids and owner ids only — never a name, an email row or any of the person's data — and every id it returns is handed to the retraction beside it, which re-checks the full eligibility predicate before touching anything",
	"internal/modules/capture:NoiseJudgedStandsTx":          "the per-retraction recheck the two entries above already name as what confines their write: it re-asks, on the retraction's own transaction, the very predicate NoiseJudgedContacts selected on, because the scan and the archive are separate transactions and a keep_out withdrawn or a verdict corrected between them must call the archive off. Run under the system principal with no request and no human actor, from the link-reconcile sweep and nowhere else — so there is no human grant a gate could consult, and one would refuse the only caller. It writes nothing and returns a BOOL: never a row, never an address, never anything about the person. Its subject is not caller-chosen in the sense a gate protects — the email and owner id come off the row the sweep's own selector produced from capture's tables, and it answers the same question for any caller who could name that pair. Ratified alongside its siblings rather than gated, on the same argument they carry",
	"internal/modules/capture:NoiseJudgedContacts":          "the link-reconcile retraction sweep's selector, run under the system principal with no request and no human actor — the same posture as StrandedContacts beside it. It reads people, the ledger and the sender overrides to find capture-only contacts an already-settled noise verdict or standing keep_out has disowned; it writes nothing. What confines the write that follows is not RBAC but three re-reads on the retraction's own transaction: NoiseJudgedStandsTx re-asks the very predicate this scan selected on, the correspondence bound is re-read beside it, and people.RetractCaptureOnlyPersonTx re-checks the full eligibility under the person row's lock",
	"internal/modules/knowledge:OpenDocument":               "streams a stored document's bytes for the ingest worker, by the STORAGE KEY BeginIngest returned from a row the same attempt just read — not by anything a caller supplies. It takes no id, resolves no row and reaches no table, so there is no subject for a row-scope gate to bound; what bounds it is that the only way to obtain a key is to have read the row that holds it, and that read is gated. The human path to a document's content is its own gated entry point, not this one",
	// Reached only from worker sweeps, approvals effect executors, or a
	// service that owns the gate above them. Each entry states which.
	// The Deal Room buyer edge. A buyer holds no seat, and every gate in
	// platform/auth refuses the buyer principal by kind, so the seat gates are
	// not what bounds these calls — the SESSION is. ResolveSession turns a
	// presented token into (participant, room) through one indexed lookup that
	// joins the participant on (id, room_id), and every other method here takes
	// that Session and puts its room and participant into the WHERE clause: a
	// buyer reaches their own row, their own room's latest release and their own
	// room's documents and conversation, and nothing else. TestPublicHandlersReachOnlyTheSessionScopedStore
	// and TestSessionScopedStoreNeverConsultsTheSeatGates (dealrooms) hold the
	// shape; the three anonymous operations are bounded by the credential digest
	// itself, which is the authentication.
	// Tx opens a transaction and runs the caller's function in it. It reads no
	// row and writes none, so there is nothing here for a gate to admit or
	// refuse: the gate belongs to the statements the caller then runs, and every
	// one of them takes its own. Exported only because storekit's list helper
	// takes the transaction opener rather than a database handle of its own.
	"internal/modules/automation:Claim":                       "the effect-level idempotency claim the workflow engine's create executor takes before writing (automation_effect_claim): reached only from ApplyActions inside a dispatched firing, which runs under the engine's system principal holding no object grants — so there is no human grant a gate could consult, and adding one would refuse the only caller. Nothing about its subject is caller-chosen: handler and trigger event come from the engine's own dispatch (engine_run.go stamps them onto the effect), the fingerprint is derived from the planned action, and the row it writes carries nothing but those three values and a timestamp. It answers a boolean and never returns a row. What a person may do to the records the claim guards — the task the create then mints — is gated where that write happens, in the datasource provider's own store path",
	"internal/modules/deals:Tx":                               "opens a transaction and runs the caller's function in it; reads and writes nothing itself, and every statement inside takes its own gate",
	"internal/modules/projects:Tx":                            "opens a transaction and runs the caller's function in it; reads and writes nothing itself, and every statement inside takes its own gate",
	"internal/modules/dealrooms:PeekCredential":               "anonymous by design: answers only whether a credential digest is exchangeable, one EXISTS over the invitation joined to its live participant and unarchived room; no row leaves the call",
	"internal/modules/dealrooms:ExchangeCredential":           "the authentication itself: consumes the credential in one UPDATE whose WHERE is the exchangeable predicate, so the row it writes is the one the presented secret names; the session it opens is attributed to that participant",
	"internal/modules/dealrooms:ResolveSession":               "the session lookup every buyer request runs: one SELECT keyed on the token digest, joined to the participant on (id, room_id); it is what BINDS the buyer's authority, so there is no earlier gate for it to take",
	"internal/modules/dealrooms:SignOut":                      "revokes the session row whose id, participant and room the resolved Session names, and nothing else",
	"internal/modules/dealrooms:NoteLinkRequest":              "the self-service link request's record half: refuses every actor but linkRequestPrincipal and stamps only link_requested_at on live, non-preview seats matching the address — no content, no credential, no row created",
	"internal/modules/dealrooms:ReissueByEmail":               "the self-service link request: refuses every actor but linkRequestPrincipal, finds seats by the address the mail will go to, and hands credentials to the mailer only — the response body never carries them",
	"internal/modules/dealrooms:BuyerView":                    "reads the caller's own participant row and their room's latest release, both predicated on the Session's (participant, room); the live deal is never read",
	"internal/modules/dealrooms:BuyerThreads":                 "reads the threads and comments WHERE room_id = the session's room, after the room's standing says it serves content",
	"internal/modules/dealrooms:OpenBuyerThread":              "writes a thread and its first comment into the session's room, attributed to the session's participant, after liveRoomForBuyerWrite settled that the room is live and the capability admits writing; a document thread must name one the latest release publishes",
	"internal/modules/dealrooms:ReplyAsBuyer":                 "appends a comment to an open thread WHERE room_id = the session's room, under the same liveRoomForBuyerWrite gate",
	"internal/modules/dealrooms:BuyerDocuments":               "reads the manifest frozen in the latest release of the session's room — the snapshot column of one deal_room_release row selected by room_id — and nothing else",
	"internal/modules/dealrooms:BuyerDocumentLocator":         "resolves one published document to its storage key through deal_room_document WHERE id AND room_id = the session's room AND attachment_id = what the release froze; a document of another room, or one removed and never published, is absent",
	"internal/modules/dealrooms:NoteDocumentDelivered":        "records that the session's own seat received one document: a single INSERT of (the Session's room, the Session's participant, the document, 'document_downloaded'), never a read, and a preview seat records nothing. The Session IS the authority, and it was already resolved and used to serve the bytes this row reports",
	"internal/modules/people:LookupPlace":                     "the place cache holds no customer data: a place NAME and the coordinates a public geocoder gave for it — 'stuttgart' → 48.77, 9.18. Nothing in it is about a company, a person or a workspace, and it is installation-global on purpose because a place is a place whoever asks. There is no subject for a grant to be about. The caller that reaches it (the geocode worker) is already gated on organization.read for the company whose address it is resolving",
	"internal/modules/people:RememberPlace":                   "the write half of the same cache, with the same rationale: what it stores is a public coordinate for a public place name. The lookup that produced it was already authorized as an organization read",
	"internal/modules/people:LookupTechnical":                 "the technical lookup cache holds no customer data: a DOMAIN and what the public internet answered about it — the MX host that receives its mail, the service labels its certificates reveal. Nothing in it is about a person, and it is installation-global on purpose because a domain's DNS records are the same for every tenant that holds that domain. There is no subject for a grant to be about. The caller that reaches it (the technical lookup worker) is already gated on organization.read for the company whose domain it is reading",
	"internal/modules/people:RememberTechnical":               "the write half of the same cache, with the same rationale, plus one of its own: what it stores has already been through the classifier, so the raw certificate hostnames — the only part that could ever carry a personal name — never reach it. The lookup that produced it was already authorized as an organization read",
	"internal/platform/settings:WriteTx":                      "a transaction wrapper only: it opens the workspace-bound transaction and runs the caller's function, and every settings write inside goes through SetRawTx, which takes the entry's own update gate per key — the gate sits where the key is known, one level down",
	"internal/modules/activities:ReconcileExtractionActivity": "worker path (compose/jobs_aiactivity.go): the pass re-announces the CURRENT state of readings the AI-activity projection may be stale about, under the system principal. It changes no reading — it re-publishes what the rows already say, plus the rotation marker that keeps a bounded pass moving — so there is no decision here for a grant to govern, and a row-scope gate would narrow the repair to one rep's readings and leave every other person's display permanently wrong. Attribution survives being ungated because reannounceCtx stamps each announcement with the READING's own requester as on-behalf-of, which is the actor the projection resolves ownership from; TestTheRepairKeepsTheReadingWithThePersonWhoAskedForIt holds that, and fails when the pass announces as itself",
	"internal/modules/aiactivity:Mine":                        "aiactivity.Store.Mine — the personal AI-activity read. It writes nothing, so there is no mutation for this gate to be the last line in front of, and its row scope is not an RBAC object but the attribution predicate itself: both statements filter on actor_user_id = the authenticated caller, so an occurrence this person did not cause is not reachable through it. The two ways an occurrence belongs to nobody are distinct columns and neither matches: work that is workspace-scoped by nature carries a NULL actor_user_id, and so does a departed person's history, which stays as history and is shown to no one. The transport refuses a principal with no user identity before the store is called, and the operation is x-agent-access: human-only, which the generated agent-policy table enforces. Ratified here only as a subject the roots do not cover",
	"internal/modules/aiactivity:Troubled":                    "aiactivity.Store.Troubled — the personal troubled-runs read behind the ai_work_health lane. Ratified on Mine's ground, stated beside it: it writes nothing, both arms filter on actor_user_id = the authenticated caller, and the store itself refuses a caller with no person behind it with the permission sentinel before any query — the arm the lane's withheld rendering depends on, held by TestTroubledWithNoPersonIsRefusedWithTheSentinel",
	"internal/modules/aiactivity:ApplyStateChange":            "the projection's only writer, reached only from HandleEvent on cg:ai-activity under the system principal. There is no request and no human actor to gate: the event's own actor is what the row is attributed to, and it is read out of the envelope rather than chosen by any caller. The read that will serve these rows is where a person's scope binds, and it is scoped to the caller's own user id per statement",
	"internal/modules/aiactivity:PurgeSettledBefore":          "worker path (compose/jobs_aiactivity.go): the retention pass drops settled occurrences past the installation's window, under the system principal. It is a whole-table delete by age with no subject and no actor — narrowing it by a grant would leave one person's aged rows behind because nobody with the grant happened to run the pass",
	"internal/modules/aiactivity:CloseAbandonedRouterRuns":    "worker path (compose/jobs_aiactivity.go): the same retention pass settles router occurrences whose lease ran out with no settle behind it, under the system principal. A router start is committed before its call and closed only by a best-effort flush, so a timed-out flush or a killed process leaves a live row nothing else reaches — the live feed has no time bound and the settled purge only sees settled rows. Like the purge beside it this is a whole-table sweep by age with no subject and no actor, and narrowing it by a grant would strand one person's abandoned rows because nobody with the grant happened to run the pass. It touches only source = ai_router: a carrier holds a durable claim it can re-arm from, and closing one of those would settle work that is still happening",
	"internal/modules/activities:OverdueScheduledSends":       "worker path: the recovery pass reads messages whose moment has passed and whose alarm is gone, under the system principal. It returns IDS and no content, and it exists precisely to find rows no human is watching — a row-scope gate would narrow it to one rep's messages and leave everybody else's stranded, which is the failure it exists to end",
	"internal/modules/activities:RearmScheduledSend":          "worker path: the recovery pass re-enqueues the alarm a message lost, under the system principal. It changes nothing about the message — not its moment, not its state — so there is no decision here for a grant to govern; what it restores is the ordinary fire path, which takes every gate itself when it runs",
	"internal/modules/activities:HoldScheduledSend":           "worker path: the scheduled-send timer hands a message back to a human when the sender no longer resolves or the ladder is exhausted, under the system principal — the human whose authority it WOULD have used is exactly the one that failed to resolve, so there is no actor to gate on",
	"internal/modules/activities:RescheduleInTx":              "the write half of RescheduleScheduledSend, which takes the gate; split so a decision releasing this work can consume its approval and do it in ONE transaction (approvals.RedeemAndApply). Both callers gate first — the endpoint through its public half, the held-card effect through the decision the inbox already narrowed to this rep",
	"internal/modules/activities:CancelInTx":                  "the write half of CancelScheduledSend, gated by the same two callers and split for the same reason: a rejection and the cancellation it releases have to commit together, which a method opening its own transaction cannot do",
	"internal/modules/ai:CostReport":                          "aggregates this installation's ai_call rows into totals and returns no record; the cost surface above it takes the grant, and there is nothing here for object RBAC to narrow",
	"internal/modules/ai:DueDeferredBuilds":                   "worker sweep: walks the fleet workspace-by-workspace for builds to re-offer, under the system principal — no human actor exists to gate",
	"internal/modules/ai:RateFor":                             "reads the provider rate card (model pricing), not tenant data — it returns no record and there is no object to grant on",
	"internal/modules/ai:ServedTaskTotals":                    "aggregate of this installation's calls for compose/costestimate; returns counts and totals, never a record, so there is no row whose visibility a gate could decide",
	"internal/modules/approvals:ExpireDue":                    "worker sweep (compose/jobs_approvalexpiry.go): it records the refusal a CLOSED WINDOW already made, which is why the audit row names the clock and decided_by stays null. Exempt from OBJECT RBAC — no human's grants can narrow a decision no human made — but NOT ungated: onlyTheExpirySweep admits the system principal presenting ExpiryActor and refuses everyone else, because an open bulk-decide would let any authenticated user refuse every pending approval at once, each one audited as though the clock had done it (TestOnlyTheClockMayExpireApprovals)",
	"internal/modules/approvals:MarkLapsedRedemptions":        "worker sweep (compose/jobs_approvalexpiry.go), riding the same tick as ExpireDue beside it and holding the same posture. It EXECUTES NOTHING \u2014 an agent-minted staging has no server-side executor by design (ADR-0055), which is the whole reason its work can go undone silently \u2014 and writes only the effect-failure mark decide.go already writes, on rows whose subject nothing about the caller chose: the predicate is the redemption clock. Exempt from OBJECT RBAC because no human's grants can narrow a fact about a window that closed, but NOT ungated: onlyTheExpirySweep admits the system principal presenting ExpiryActor and refuses everyone else, because an open bulk write here would let any authenticated user tell every approver in the workspace that their decisions were never carried out, each row indistinguishable from one this sweep found (TestOnlyTheExpirySweepMayMarkLapsedRedemptions)",
	"internal/modules/introductions:ExpireDue":                "worker sweep (compose/jobs_introexpiry.go): it records the answer a PASSED DEADLINE already gave — a colleague's silence — which is why the audit row names the clock and not a person. Exempt from OBJECT RBAC because no human's grants can narrow a close no human chose, and a row-scope gate would be the wrong shape besides: an ask is between two colleagues and the sweep is party to neither. NOT ungated: onlyTheExpirySweep admits the system principal presenting ExpiryActor and refuses everyone else, because an open bulk-close would let any authenticated user cancel every open introduction in the installation at once, each one audited as though the clock had done it (TestOnlyTheClockMayExpireIntroductions). Nothing is caller-chosen: it takes no id, its subject is derived entirely from the database's own clock against due_at, and it returns a COUNT rather than any row",
	"internal/modules/capture:AgeOutReviewTx":                 "age-out sweep write inside the caller's transaction",
	"internal/modules/capture:AwaitingReview":                 "review-queue sweep (compose/captureverdictsweeps.go) under the system principal",
	"internal/modules/capture:RecordOutcomeTx":                "writes the confidentiality verdict onto the seat's own import row, on the SAME transaction as ResolveAs, which the sweep already claimed the ledger row under. The claim is the authority and there is no principal to gate: the pass runs as the system principal, which bypasses object RBAC anyway. What confines the write is provenance — the user_id it stamps comes from the ledger row, written at capture from the authenticated connector principal",
	"internal/modules/capture:RecordOutcomeOnThreadTx":        "the same verdict write as RecordOutcomeTx, applied to the messages of the thread this seat had ALREADY imported when the answer came back. Same transaction, same claim, same absence of a principal — and the same confinement: user_id comes from the ledger row and the stamp is scoped to it, so one seat's answer never reaches a colleague's contribution. The thread and seat are the CALLER's to supply and this function does not verify they were claimed together — the one caller passes the claimed row's own fields, and a second caller owes the same. What bounds WHICH messages it may open is not RBAC but the admission rule it shares with inheritedVerdictTx: a sibling takes an opening verdict only from a party the verdict actually read",
	"internal/modules/capture:ThreadsWithUndecidedMessages":   "the repair sweep's selector, run by the confidentiality job under the system principal — no request and no human actor. It reads the ledger and the seat's own import rows to find questions whose answer never reached their messages; it writes nothing. The rows it returns are unlocked snapshots; re-reading each under a lock before acting is the CALLER's obligation, which FinishSettledThreads meets through LockSettledThreadTx and a second caller would owe too",
	"internal/modules/capture:LockSettledThreadTx":            "takes the ledger row the selector named and re-reads its answer under the lock, so the repair applies the answer the row still holds rather than the one the listing saw. Same sweep, same system principal — which this function does not itself verify: it is reached only from the confidentiality job, and a caller on a request path would be the thing to refuse. A pending row is refused because a question nobody has answered is the classifier's, not the repair's",
	"internal/modules/capture:StrandedContacts":               "the link-reconcile sweep's selector, run under the system principal with no request and no human actor. It reads people and the ledger to find captured contacts nobody was ever asked about; it writes nothing. What confines the WRITE that follows is not RBAC but provenance: the owner it carries is the person row's own owner_id, set at capture from the authenticated connector principal, and a row without one is skipped rather than assigned to somebody",
	"internal/modules/capture:AskWhoseRecord":                 "opens the question the capture path could not, through the same askWhoseRecordTx that path uses — same table, same ceiling, same terminal-answer check. Same sweep and same system principal as the selector above; the address and owner come from the row it was handed, never from a request",
	"internal/modules/capture:EnsureTx":                       "opens a thread's confidentiality question from INSIDE the capture transaction, under the connector principal the sink already took the grant for; a gate here would ask a second time about the same admission, and a message that landed would have nobody scheduled to judge it",
	"internal/modules/capture:ClaimDue":                       "worker sweep (compose/captureverdict.go): claims due pending counterparties under the system principal; no request, no human actor",
	"internal/modules/capture:ClaimReviewForAgeOut":           "the claim that serializes the age-out sweep; system principal, no request path",
	"internal/modules/capture:CorrectResolution":              "sweep correction of a verdict it wrote itself, inside the caller's transaction",
	"internal/modules/capture:Defer":                          "sweep bookkeeping for a claimed row — reschedule with backoff; reached only from the same system-principal loop as ClaimDue",
	"internal/modules/capture:ExpireExhausted":                "auto-enrich sweep expiring exhausted budget slots",
	"internal/modules/privacy:Posture":                        "PostureStore.Posture reads one setting through platform/settings.Get, which delegates to Store.Raw — and Raw takes the object grant PER SETTING against the object the entry declares (auth.Require(def.Object(), ActionRead)), which for privacy.RetainOnly is retention_policy. internal/platform/settings is one of THIS gate's roots, so that gate is judged here directly rather than assumed; re-gating in the wrapper would re-check the same read against the same object, the capture:Get shape exactly. SetPosture beside it DOES gate, because its nil branch never reaches the settings write",
	"internal/modules/capture:Get":                            "SettingsStore.Get reads one capture setting through platform/settings.Get, which delegates to Store.Raw — and Raw takes the object grant PER SETTING against the object the entry declares (auth.Require(def.Object(), ActionRead)). internal/platform/settings is one of THIS gate's roots, so that gate is judged here directly rather than assumed; re-gating in the wrapper would re-check the same read against a coarser object, the same shape as identity:GetInstallation",
	"internal/modules/capture:LadderByActivityID":             "the bound is a PREDICATE IN THE SQL, not a grant: the lookup selects `user_id = <caller>` and can express nothing else, so it reaches only rows the calling member's own connection produced. There is no object to gate on because capture_trace rows answer to their owner rather than to a grant — 0258 is categorical that no grant widens them — and an auth.Require here would check the wrong question. The sibling LadderByTraceID asks auth.Require itself, but only to decide whether to ALSO admit workspace-owned rows (user_id IS NULL); this call has no such branch, since a shared binding's rows are reached through the trace id, never through somebody's activity",
	"internal/modules/capture:LinkProposal":                   "sweep bookkeeping: binds the staged proposal to the pending row it was raised from",
	"internal/modules/capture:ListDueOrgs":                    "auto-enrich sweep (compose/captureautoenrich.go) under the system principal",
	"internal/modules/capture:MarkQueued":                     "auto-enrich sweep bookkeeping for an org it just queued",
	"internal/modules/capture:MarkResolved":                   "records the outcome of an auto-applied deep read; effect-executor path, the approval is the authority",
	"internal/modules/capture:NoiseMailForTx":                 "sweep read inside the caller's transaction, selecting the loop's own claimed activities",
	"internal/modules/capture:NoiseMailToHide":                "retention sweep read: noise mail eligible for hiding",
	"internal/modules/capture:NoiseMailToRedact":              "retention sweep read: noise mail past its redaction window",
	"internal/modules/capture:StalledBacklogSeats":            "sweep read: which seats have pending dispositions nothing has touched. It answers seat ids and counts, never an address or a subject, and its only caller runs under the verdict pass's system principal to tell each seat their own backlog stopped moving",
	"internal/modules/capture:PurgeRawCaptureTx":              "retention sweep purge inside the caller's transaction, over activities the same sweep selected",
	"internal/modules/capture:ReconcileDeclined":              "sweep reconciling declined proposals back onto their pending rows",
	"internal/modules/capture:ReleaseBudget":                  "returns the slot ReserveBudget took, same sweep",
	"internal/modules/capture:ReserveBudget":                  "auto-enrich budget reservation for the sweep's own slot; an accounting write with no record and no actor",
	"internal/modules/capture:Resolve":                        "sweep verdict write for a row the loop already claimed; the claim is the authority and there is no principal to gate",
	"internal/modules/people:PeopleOwedACohortRepair":         "the nightly repair's selector: it lists contacts whose captured mail is not on their record yet, reads nothing but ids, and is reached only from the link_reconcile sweep under the system principal — there is no human in a sweep for object-RBAC to admit",
	"internal/modules/people:RepairPersonCohort":              "PromotePersonCohortTx on its own transaction for the sweep that owns no other write; the repair it runs is ratified beside it, and attaching mail the workspace already holds to a record it already has creates nothing and admits nobody",
	"internal/modules/people:DomainsOwedTheirPeople":          "the same sweep's other selector: companies whose domain has contacts with no employer. It runs as the system BECAUSE no human may do it — attaching a person to a company is a write about the PERSON, and a rep naming a company holds no authority over contacts they cannot see",
	"internal/modules/people:AttachDomainBacklog":             "the plant that answers that selector, the same one the domain-triage verdict runs: it never reassigns anybody a human already placed, takes its own row locks, and is reached only from the sweep",
	"internal/modules/people:PromotePersonCohortTx":           "the repair that makes a person's captured cohort independent of the order it arrived in: it attaches mail the workspace already holds to a record it already has, and creates nothing. Reached from the verdict sweep on its transaction and from the person-event consumer, neither of which has a human principal for object-RBAC to admit; the rows it can touch are bounded by the person's own live addresses",
	"internal/modules/people:SuppressBulkSenderDomainTx":      "the verdict engine's own effect, inside the transaction that concluded the sender is bulk mail; it runs under the system principal with no human actor, exactly like the ClaimDue sweep that reaches it",
	"internal/modules/capture:ResolveAs":                      "the same sweep verdict write as Resolve, recording the sender kind alongside the status; same claim, same absence of a principal",
	"internal/modules/capture:ResolveReviewed":                "approvals EFFECT EXECUTOR (compose/captureverdictaccept.go): it runs after a human approved the staged review, and the approval record is the authority — the approvals surface took the grant",
	"internal/modules/capture:ResolveReviewedAs":              "the same approvals effect executor as ResolveReviewed, recording the sender kind the human's acceptance implies; same approval record, same authority",
	"internal/modules/capture:Retire":                         "sweep bookkeeping: retires a pending row the loop has finished with, same system-principal path as ClaimDue",
	"internal/modules/capture:RetireExhausted":                "sweep retiring rows that exhausted their attempts",
	"internal/modules/capture:StaleReviews":                   "sweep read: reviews past their window, for the age-out loop",
	"internal/modules/identity:GetInstallation":               "reads the three installation settings through platform/settings.Store.Raw, which takes the object grant PER SETTING against the object each entry declares — and which THIS gate judges directly, since internal/platform/settings is one of its roots. Gating again here would re-check the same thing against a coarser object; the lock-state probe beside it reads no setting value, only whether a deal has converted",
	// The kill switch inside the caller's transaction, for a caller with its
	// own half of the same fact to commit — withdrawing a standing overnight
	// grant ends the answer and the credential together. Bounded by the
	// ownership check the shared revokePassportTx performs on every path: a
	// passport whose on_behalf_of is not the caller reads as absent unless they
	// hold the admin role, so this exported form can no more reach a
	// colleague's credential than the pool-opening one it wraps.
	"internal/modules/identity:MintSetupToken":               "pre-authentication by construction (ADR-0105): it issues the credential a claim is made WITH, and it REFUSES on an installation that holds an organization — checked in its own transaction under the installation advisory lock, so the sentence is enforced rather than assumed of its callers. Before that point there are no roles, no users and nobody who could hold a grant. What bounds abuse is the single-outstanding partial index: a second call while one is live is refused rather than issuing a rival credential",
	"internal/modules/identity:ChangePassword":               "acts on the CALLER's own row and nobody else's: it reads the user id off the bound principal rather than taking one, so there is no other account it could reach and no object for a grant to narrow. What authorizes it is not the session but the CURRENT PASSWORD, verified inside the transaction — a stricter bar than any role, and the reason a stolen session cannot use it. A caller with no user behind it (agent seat, system principal) is refused outright",
	"internal/modules/identity:MyDelivery":                   "reads the CALLER's own row and nobody else's: the user id comes off the bound principal rather than from a parameter, so there is no other account it could reach and no object for a grant to narrow — the same shape as SaveMyLocale below. It refuses every principal that is not a human seat, agents included, because what lands in a person's inbox is theirs and an agent acting under their authority must not read it",
	"internal/modules/identity:SaveMyDelivery":               "writes the CALLER's own row and nobody else's, on the same terms as SaveMyLocale below: the id comes off the principal, no parameter names an account, and the values are confined to the vocabulary the column's CHECK admits. An agent acting under someone's authority is refused, because deciding how often the product may interrupt a person is that person's own call",
	"internal/modules/identity:SaveMyLocale":                 "writes the CALLER's own row and nobody else's: the user id comes off the bound principal rather than from a parameter, so there is no other account it could reach and no object for a grant to narrow — the same shape as ChangePassword above. It refuses every principal that is not a human seat, agents included, because a display language is a preference about a person's own screen and an agent acting under someone's authority must not change what they read. The value is confined to the languages the product ships a catalog for; nothing else is written, and nothing is read back that the caller did not already have",
	"internal/modules/identity:ClaimInstallation":            "pre-authentication by construction (ADR-0105): it CREATES the first admin, so there is no principal to gate on — gating it would require the grant it is about to mint. The setup token is the authorization, checked inside the same transaction as the create, and an installation that already holds an organization is refused before the token is even read",
	"internal/modules/identity:RotateSetupToken":             "operator-only recovery, the same posture as reset-password beside it (ADR-0061 §4): reachable only from cmd/migrate with the OWNER DSN, never over HTTP, because rotating invalidates a live claim credential and that is precisely what an attacker wants while the operator still holds one. Like MintSetupToken it refuses on an installation that holds an organization, under the installation advisory lock — before that point there is no principal to gate on, and after it there is nothing left to claim",
	"internal/modules/identity:SetupTokenOutstanding":        "answers one boolean — is this installation waiting to be claimed — to a caller who cannot authenticate because no user exists yet. It reads no tenant data and returns no record; the same fact is already visible to any stranger from the 503 every other route answers while unprovisioned, so there is nothing here a grant could withhold",
	"internal/modules/identity:SeatNames":                    "names colleagues by app_user id and returns nothing else. There is no object to grant on: a SEAT is not a record (it is outside datasource.RecordTypes, so nothing points at one and no grant names it), which is the same reason ai:RateFor is here. What bounds it is AUTHENTICATION — every app_user row belongs to the one installation the caller has already authenticated into — and what it discloses is a colleague's display name, which who_knows and account_coverage already answer to any authenticated reader",
	"internal/modules/identity:ActorIdentity":                "answers who the CALLER is, and nothing else: it reads display_name and email off the one app_user row the principal already authenticated as, resolving UserID then OnBehalfOf so an agent writes as the human whose authority it holds. There is no object to grant on for the same reason SeatNames has none — a seat is not a record — and this is one step narrower than SeatNames, which names colleagues where this names only the asker. Nothing here is disclosure: the caller presented these credentials to make the call, and the row it reads back is the one those credentials named. A principal with no human behind it, and a seat the installation does not hold, both answer empty rather than erroring, because an unsigned draft is the specified outcome there (DRAFT-AC-E-6)",
	"internal/modules/identity:Colleagues":                   "the workspace roster, one step wider than SeatNames beside it: SeatNames answers what an id a caller already holds is called, this answers WHICH id — the question that comes first, and the one nothing on the tool surface could ask. Same reason there is no object to grant on: a seat is not a record (outside datasource.RecordTypes, so nothing points at one and no grant names it). What bounds it is the WORKSPACE PREDICATE (workspace_id = current_setting, the same one the REST roster carries — RLS was retired in core 0217, so the predicate is the scope) plus authentication. What it discloses is a colleague's display name — which who_knows already answers to any authenticated reader — and their work address on the workspace's own domain. It lists ONLY seats that can receive work: archived, suspended and locked-out ones are filtered in the query rather than reported with a flag, because WHICH colleague is suspended is an admin's fact and the REST roster honours include_inactive for an admin alone",
	"internal/modules/identity:ActorProfile":                 "ActorIdentity's fuller answer, for a surface that must NAME the caller rather than sign as them: the same one app_user row the principal already authenticated as, resolved the same way (UserID then OnBehalfOf), plus the locale and timezone that row holds. Everything above applies unchanged — a seat is not a record, so there is no object to grant on, and nothing here is disclosure because the caller presented these credentials to make the call. It is what the whoami tool answers, and an assistant that cannot say who it acts for cannot set an owner, assign a task, or write stored prose in the reader's language",
	"internal/modules/identity:Get":                          "onboarding wizard state, SELF-scoped: onboardingActor resolves the authenticated human and the query is keyed on user_id, so no object grant applies to your own checkpoint",
	"internal/modules/identity:Put":                          "the write half of the same self-scoped wizard state; onboardingActor is the gate and the row is keyed on the acting user",
	"internal/modules/integrations:ExecuteSubmit":            "worker execution (compose/jobs_providerruns.go) under the system principal: advances a run that QueueRun already object-gated (integrations grant) AND row-gated (EnsureVisible on the person) when it was queued; the run id comes from the durable job the same transaction committed, no request path reaches it, and the egress lease inside it bounds what the call may touch",
	"internal/modules/integrations:Get":                      "SettingsStore.Get reads one integrations setting through platform/settings.Get, which delegates to Store.Raw — and Raw takes the object grant PER SETTING against the object the entry declares (auth.Require(def.Object(), ActionRead)), which for AutomaticLookup is integrations. internal/platform/settings is one of THIS gate's roots, so that gate is judged here directly rather than assumed; re-gating in the wrapper would re-check the same read against the same object, the capture:Get shape exactly. Update beside it DOES gate, because its nil-patch branch never reaches the settings write",
	"internal/modules/integrations:RunDueSweep":              "worker sweep (compose/jobs_providerruns.go) under the system principal: drains runs already admitted at queue time — polls, claim recoveries and expiries on rows the sweep's own predicates select; no record is returned and no human actor exists on this path",
	"internal/modules/overlay:BlockAutoMap":                  "same: only usermapservice.go calls it, behind requireUserMapAdmin",
	"internal/modules/finance:RecordSyncFailure":             "the sweep's own bookkeeping for the pass it just failed, on the same connector-principal path as SyncConnection; it writes no tenant record, only the connection's attempt stamp, status and error code — and the audit row that names the transition, from the same principal that stamped the connection's own captured_by",
	"internal/modules/finance:SyncConnection":                "the finance sweep's mirror write, under the worker's connector principal; the accounting source is the authority for what it says, there is no request and no human actor on this path, and the module exposes no other write at all. Every row it writes commits its audit_log row in the same transaction, so what a grant would have decided is instead RECORDED — the provenance is on the row and on its history, not asserted by a gate nobody could hold",
	"internal/modules/overlay:Get":                           "MirrorStore.Get is List's single-row twin and row-scoped the same way: resolveActingMirrorUserID + visibilityJoin put the mirror_visibility deny-join in the query itself, so an unmapped principal is answered ErrNotFound before the row is read rather than refused by auth.Require, and the datasource provider above it takes the object grant",
	"internal/modules/overlay:Ingest":                        "the sync sweep's mirror write (backfill + refetch jobs) under the worker's system principal; the incumbent connection is the authority, and no human actor exists on this path",
	"internal/modules/overlay:List":                          "row-scoped by the mirror_visibility deny-join rather than auth.Require: resolveActingMirrorUserID + visibilityJoin answer ErrNotFound for an unmapped principal BEFORE the page query runs, and the datasource provider above it takes the object grant",
	"internal/modules/overlay:ListUserMap":                   "reached only through usermapservice.go, whose every entry point takes requireUserMapAdmin (overlay_connection:update + RequireHuman) — the sanctioned Handlers->Service shape, where the service owns the gate and the store beneath it is module-internal",
	"internal/modules/overlay:LoadBackfillCursor":            "sweep checkpoint read, the mirror of SaveBackfillCursor",
	"internal/modules/overlay:LoadReconcileWatermark":        "reconcile-poller checkpoint read",
	"internal/modules/overlay:PurgeRecord":                   "deletion-feed teardown from the reconcile sweep: removes a mirror row the incumbent reports gone, under the system principal",
	"internal/modules/overlay:RecomputeForOwner":             "recomputes mirror_visibility for one incumbent owner after a mapping change; driven by the mapping writes above, which are themselves gated",
	"internal/modules/overlay:RecordReprojectionFailure":     "the re-projection sweep's own bookkeeping about the pass it just failed, on the same worker system-principal path as Ingest and StaleProjections. It writes one column of the mirror row it was handed — the declaration fingerprint the re-fetch could not reach — and no incumbent payload, so it discloses nothing and returns no record. Both producers of the re-fetch it follows reach it over a workspace-bound handle and the UPDATE states that bound in its own predicate, so it is confined to this workspace's own mirror whatever id it is handed — the sweep's, which came from StaleProjections in the same bound transaction, and the webhook lane's, which came off the wire from the incumbent; there is no row whose visibility a gate could decide",
	"internal/modules/overlay:RecordSweepFailure":            "the failure half of the same backoff bookkeeping",
	"internal/modules/overlay:RecordSweepSuccess":            "sweep health bookkeeping (backoff state) written by the sweep about itself",
	"internal/modules/overlay:RequestSweep":                  "MirrorStore.RequestSweep is the store half of the sanctioned Service-owns-the-gate shape (same as ListUserMap): the ONLY caller is Service.RequestSweep, which takes auth.Require(overlay_connection, ActionUpdate) and fences the store against a racing disconnect before delegating. The store method itself writes overlay_sync_state — a due-at and a failure ladder, not a record — and until this gate became receiver-aware the service's grant was silently answering for it",
	"internal/modules/overlay:RevalidateEmailMappings":       "sweep revalidation of owner email mappings; no request path reaches it",
	"internal/modules/overlay:SaveBackfillCursor":            "sweep checkpoint write — the backfill's own resume cursor, not a record",
	"internal/modules/overlay:SaveReconcileWatermark":        "reconcile-poller checkpoint write; sweep state, not a record",
	"internal/modules/overlay:SeedUserMap":                   "seeds mirror_user_map at connect time and on the sweep; the connect handler above it takes overlay_connection:update, and the sweep runs as system",
	"internal/modules/overlay:SetManualUserMap":              "same: only usermapservice.go calls it, behind requireUserMapAdmin",
	"internal/modules/overlay:StaleProjections":              "names the mirror rows an older mapping declaration projected, for the reconcile sweep's re-projection phase, under the worker's system principal — no request path reaches it and no human actor exists on it. What comes back is external ids of rows this workspace's own mirror already holds, inside its workspace-bound transaction: no record and no field of one leaves the call, so there is no row whose visibility a gate could decide",
	"internal/modules/overlay:UpsertAssoc":                   "the same sweep's edge write, from backfill",
	"internal/modules/overlay:UpsertUserMap":                 "the per-entry write SeedUserMap and the visibility recompute drive; same two paths, no independent entry",
	"internal/modules/people:ExhaustedDomains":               "triage sweep (compose/capturedomaintriage.go) under the system principal: reads domains whose crawl attempts are spent so the sweep can settle them rather than strand them; no record, no human actor",
	"internal/modules/people:GetMyEmailSignature":            "self-only: reads the CALLER's own email_signature row, keyed on the authenticated principal's user id, and there is no path here to another member's; a signature is the words a person signs their name with and no seat including admin has a reason to read one",
	"internal/modules/people:SaveMyEmailSignature":           "self-only, same row and same key as GetMyEmailSignature — a member writing the sign-off that goes out under their own name, which nobody may write on their behalf",
	"internal/modules/people:SignatureFor":                   "self-only, enforced in the method: it refuses any user id that is not the authenticated caller's, so the send path signs with the sender's own sign-off and can ask for no other; the argument exists because the send path holds the sender id explicitly rather than implying it",
	"internal/modules/people:GetMyLinkedInAccount":           "self-only: reads the CALLER's own linkedin_account row, keyed on the authenticated principal's user id, and there is no path here to another member's; an object grant would be the wrong question because a member needs no permission to see their own profile",
	"internal/modules/people:RetireStaleTriageRead":          "triage sweep bookkeeping under the system principal: finishes a dossier that stopped reporting so its domain can be asked again; touches no record and has no human actor",
	"internal/modules/people:ListDueDomains":                 "triage sweep (compose/capturedomaintriage.go) under the system principal: reads domains still owed an organization verdict, no record and no human actor — the twin of capture:ListDueOrgs",
	"internal/modules/people:MarkTriageQueued":               "triage sweep bookkeeping for a domain it just enqueued a crawl for; arms the retry cursor, touches no record",
	"internal/modules/people:MyLinkedInMatchTotals":          "self-only: counts the CALLER's own ghosts, keyed on the authenticated principal's user id, and returns two integers rather than a record; a member needs no permission to be told where their own import stands",
	"internal/modules/people:RenormalizeLinkedInCompanyKeys": "worker maintenance under the system principal: rewrites a DERIVED column (normalized_company) and collapses the duplicates an older normalizer left; no human actor exists to gate, and it returns counts rather than any record",
	"internal/modules/people:SaveMyLinkedInAccount":          "self-only, same row and same key as GetMyLinkedInAccount — a member editing their own LinkedIn profile URL, which no seat including admin may do on their behalf",
	// Authentication IS the gate these methods implement: they run
	// before a principal exists, or mint/retire the session itself.
	"internal/modules/identity:Login":                     "pre-principal: password verification is what admits the actor; there is no principal to gate yet",
	"internal/modules/identity:LoginViaFederatedIdentity": "pre-principal, same posture as Login: a verified Google ID token is what admits the actor here instead of a password, and there is no principal to gate until the session it mints exists",
	"internal/modules/identity:Logout":                    "session retirement; the bearer's possession of the session IS the authority being revoked",
	"internal/modules/identity:Authenticate":              "pre-principal: this resolves the session cookie INTO the principal every other gate consumes",
	"internal/modules/identity:AuthenticateAgent":         "pre-principal: passport verification is what admits the agent actor (every call re-authenticates, ADR-0055)",
	"internal/modules/identity:AuthenticateAgentByID":     "pre-principal: the by-id half of passport verification, same admission seam",
	"internal/modules/identity:InstallationWorkspace":     "singleton-organization resolution (A107/ADR-0061), bound by the middleware before any principal exists",
	"internal/modules/identity:BootstrapInstallation":     "boot-time provisioning under the system principal (A107/ADR-0061); no human principal can exist before it",
	"internal/modules/identity:CreatePasswordReset":       "pre-principal by design (A74): the caller is locked out; enumeration-resistant token mint, authority is control of the mailbox",
	"internal/modules/identity:RedeemPasswordReset":       "pre-principal by design (A74): possession of the single-use emailed token IS the authority being verified",
	"internal/modules/identity:EffectiveRBAC":             "this LOADS the merged role policy the auth gate enforces — gating it on itself would recurse",
	"internal/modules/identity:SeatType":                  "seat-tier lookup feeding the auth gate (scope ∧ tier); same layer as EffectiveRBAC, not above it",
	"internal/modules/identity:EffectiveAuthority":        "the two above, read in ONE snapshot; it is the same layer as both and gating it on itself would recurse for the same reason",
	"internal/modules/identity:AdmittedAuthority":         "the same snapshot again, with the passport's own liveness asked beside it — what the admission gate reads at every tool call, so it IS the gate's own read and gating it would recurse. It answers about a credential the caller already presented rather than about any record",
	"internal/modules/identity:IssuePassport":             "gated by the explicit Identity parameter (the authenticated session): a passport is minted for that identity only, capped by validScopes",
	"internal/modules/identity:GetUser":                   "roster read (A52): same rationale as ListUsers — a single member read is intentionally visible to every authenticated seat, and AUTHENTICATED MEMBERSHIP is the whole boundary; \"user\" is deliberately absent from policy.coreObjects",
	"internal/modules/identity:ListUsers":                 "roster read (A52): the member roster is intentionally visible to every authenticated seat, by design, not by oversight — a share-subject picker that only some roles could see would be a broken feature, not a narrower one. Authenticated membership IS the boundary; \"user\" is deliberately absent from policy.coreObjects (the closed RBAC object set), because gating it would mean granting read on it to all five default roles (no role may reasonably be refused the roster) and backfilling every already-seeded workspace's role.permissions — object-level RBAC exists to narrow WHO sees a record among peers, and there is no such narrowing here to express",
	"internal/modules/identity:ListTeams":                 "roster read (A52): same rationale as ListUsers — the team list is intentionally visible to every authenticated seat, with authenticated membership as the whole boundary, and \"team\" is deliberately absent from policy.coreObjects for the same reason: gating it would grant read to every role, not restrict it, while requiring a backfill of every seeded workspace's role.permissions",

	// Public-by-design token surfaces: possession of the emailed or
	// published capability is the authority; there is no authenticated
	// principal. What bounds each capability differs — single use, a
	// signature, an expiry, or nothing but its entropy — so each entry
	// names its own, rather than this header claiming one for all of them.
	"internal/modules/activities:ResolveBookingPage":  "public booking page (A16): resolved by slug for the anonymous visitor; writes nothing",
	"internal/modules/consent:ResolvePreferenceToken": "public preference-center resolve: possession of the emailed capability token IS the authority (no session exists). It is NOT signed and NOT single-use — the preference centre is revisitable by design, and one message's link must keep working after the next goes out — so the bounds that stand in for those properties are named here: 256-bit crypto/rand, expiry plus an age ceiling the send path rotates at (0144), and deletion by Art. 17 erasure",
	"internal/modules/consent:ResolveConfirmToken":    "confirm-details resolve: possession of the emailed link IS the authority, on the same session-less edge as the preference resolve above and with strictly tighter bounds, because this one DISPLAYS the record rather than a list of switches. 256-bit crypto/rand, sha256 at rest so a stolen table opens nobody's record, a 14-day expiry, spent on first submit, superseded by any fresh issuance, and deleted by both Art. 17 erasure and the retention anonymizer. Unknown, expired and spent all read as absent, so the surface is not an oracle for which it was",
	"internal/modules/approvals:LockPendingGroupInTx": "takes row locks on the CALLER's transaction and answers nothing but an error — no record, no count, not even whether the group it locked is empty — so there is nothing an ungated caller could learn from it. The batch stagers that call it hold their grant before the transaction opens, and each proposal they go on to stage is gated on its own way in; a gate here would re-ask that question against a coarser object",
	"internal/modules/approvals:MintApprovalToken":    "signs the approval JWS for a decision already admitted by Decide; crypto, not admission",
	"internal/modules/approvals:AutoApplyMode":        "reads whether the CALLER has put one kind on automatic. It takes no user id: the subject is the principal on the context, so there is no row a caller can ask for but not be, which bounds it more tightly than a grant would — a grant can be held over somebody else. There is also no object to gate on, `approval` not being in the closed core set, so a Require here would have to invent a vocabulary entry to check nothing extra. It answers one enum value about the caller's own preference and returns no record and no count. A principal with no person behind it is refused outright rather than reading a zero-uuid row",
	"internal/modules/approvals:SetAutoApply":         "writes the CALLER's own preference for one kind, and takes no user id for the same reason the read does not: the row is keyed by the principal on the context, so a rep cannot put a colleague on automatic and there is no cross-user write for a grant to authorize. What it may write is bounded twice over — the mode is one of two constants this function chooses, never a caller string, and the kind must be in AutoApplyKinds, which is the closed set of reversible kinds the applier will honour. A preference to apply is not itself an authority to write anything: every apply it later enables runs through the same Decide a click runs, gated on the owner's own grants, seat and row-scope visibility of the target, so turning this on changes WHEN that rep is asked and never WHAT may be released",
	"internal/modules/approvals:AutoApplySettings":    "the list form of AutoApplyMode, ungated for the same reason and on the same subject: it takes no user id, so it reads the rows of the principal on the context and there is no row a caller can ask for but not be. What it adds over the single-kind read is that rep's own decision counts for the kinds they may automate, which is a fact about their history and nobody else's — no record, no cross-user total, and nothing about a target's existence. A principal with no person behind it is refused outright rather than reading a zero-uuid row",
	"internal/modules/approvals:VerifyApprovalToken":  "verifies the approval JWS presented back; the token is the authority being checked",
	"internal/modules/approvals:Redeem":               "redeems a verified approval token: the token (minted for an admitted decision) is the authority",
	"internal/modules/approvals:RedeemInTx":           "transactional form of Redeem: the already-admitted approval token is the authority; the caller supplies only the commit boundary",
	"internal/modules/approvals:RedeemAndApply":       "atomic approval-effect boundary: Redeem performs the authority checks and the callback runs only inside that same transaction",
	"internal/modules/approvals:TaskState":            "gated by the STAGING PASSPORT rather than by object RBAC, and that is the right gate: an MCP task polls the agent's own proposal, of which it has exactly one, so ownProposal answers ErrNotFound for any approval this passport did not stage. It returns a status and a window, never a record",
	"internal/modules/approvals:ProposedChange":       "same passport binding as TaskState, over the payload the agent itself staged — read live because a human edit rewrites it, and answered only to the passport that proposed it",
	"internal/modules/approvals:Withdraw":             "same passport binding, over the agent's own live proposal: it retracts what this passport staged and nothing else, and a human's decision is never the caller's to take back (WithdrawInTx refuses a decided row)",

	// Engine/system seams that never carry a human principal: the
	// worker loop and cross-module effects run as the system actor, and
	// the admission happened at the surface that staged the work.
	"internal/modules/agents/runner:StartRun":                 "agent-runner persistence driven by the worker loop under the system principal; admission happened at the tool gate that enqueued the run",
	"internal/modules/agents/runner:SaveOutcome":              "agent-runner persistence driven by the worker loop under the system principal; admission happened at the tool gate that enqueued the run",
	"internal/modules/agents/runner:MarkFailed":               "agent-runner persistence driven by the worker loop under the system principal; admission happened at the tool gate that enqueued the run",
	"internal/modules/agents/runner:ClaimSuspendedByApproval": "agent-runner persistence driven by the worker loop under the system principal; admission happened at the tool gate that enqueued the run",
	"internal/modules/agents/runner:EnqueueJob":               "agent-runner persistence driven by the worker loop under the system principal; admission happened at the tool gate that enqueued the run",
	"internal/modules/agents/runner:ClaimDueJobs":             "agent-runner persistence driven by the worker loop under the system principal; admission happened at the tool gate that enqueued the run",
	// A rep's own standing decision, and the bound is the SIGNATURE rather than
	// a grant: MyGrant takes no user id. It reads the acting principal from the
	// context and selects on it, so there is no argument a handler could pass to
	// reach a colleague's row, and an auth.Require would be asking the wrong
	// question — no object grant widens or narrows whether you may see whether
	// YOU agreed to something. A principal with no human behind it is refused
	// rather than answered, since it has no decision of its own to read.
	"internal/modules/agents/runner:MyGrant": "the bound is the signature: it takes no user id, selects on the acting principal's own, and refuses a principal with no human behind it — there is no argument by which a caller could read another rep's decision",
	// The same read inside the caller's transaction, and bounded the same way
	// for the same reason: answering a grant mints the credential the answer
	// names, so the read that reports what committed has to join that
	// transaction rather than run after it.
	"internal/modules/agents/runner:MyGrantTx": "the bound is the signature: it takes no user id, selects on the acting principal's own, and refuses a principal with no human behind it — there is no argument by which a caller could read another rep's decision",
	// The nightly fan-out's enumeration, and the one call that reads across
	// reps. What bounds it is the CALLER rather than the payload, and the
	// rationale has to say so plainly: these rows carry passport ids, and a
	// passport id IS an in-process capability — AuthenticateAgentByID takes the
	// uuid alone and hands back that rep's identity. So the bound is not "it
	// returns nothing to act on". It is that nothing user-facing can invoke it:
	// the worker's scheduling pass calls it under the system principal, and no
	// route, tool or handler reaches it. The rep-facing read is MyGrant, which
	// takes no user id and can express nothing but the caller's own row. A
	// surface that ever needs this list needs a gate, not this waiver.
	"internal/modules/agents/runner:LiveGrantsFor":           "reached only by the worker's scheduling pass under the system principal — no route, tool or handler calls it. The rows carry passport ids, which ARE in-process capability handles, so what bounds this is that nothing user-facing can invoke it; the rep-facing read is MyGrant, which takes no user id",
	"internal/modules/agents/runner:FinishJob":               "agent-runner persistence driven by the worker loop under the system principal; admission happened at the tool gate that enqueued the run",
	"internal/modules/agents/runner:FailStuckRuns":           "worker sweep under the system principal: closes runs whose resume died mid-loop, which no human requested and none can reach by then. It only moves 'running' to 'failed', so there is no object an actor could be granted or denied",
	"internal/modules/people:BeginSiteRead":                  "worker-loop status transition (queued→running), not a human principal; the human's authority was checked at StartSiteRead, and what bounds the write is the CAS itself — one dossier id, carried by the admitted job, updated only from the statuses a claim may take",
	"internal/modules/people:DeferSiteRead":                  "worker-loop scheduling transition (running→deferred); the admitted durable job supplies the retry boundary, and the write reaches one dossier id under the same claim-guarded CAS as BeginSiteRead",
	"internal/modules/people:FinishSiteRead":                 "worker-loop status transition (running→terminal), not a human principal; the human's authority was checked at StartSiteRead, and the write reaches one dossier id under the same claim-guarded CAS as BeginSiteRead",
	"internal/modules/people:UpdateSiteReadProgress":         "worker-loop progress hint on a still-running dossier, same seam as Begin/FinishSiteRead: no human principal, StartSiteRead held the gate, and the write is keyed on the claimed dossier's own id and its live lease",
	"internal/modules/people:UpdateSiteReadDraft":            "worker-loop grounded-draft update on a still-running dossier, same seam as progress: admission happened at start, and the versioned write is keyed on the claimed dossier's own id and its live lease",
	"internal/modules/people:RecordSiteReadLogo":             "worker-loop object reference parked on an UNBOUND dossier, same seam as UpdateSiteReadDraft: no human principal, StartOnboardingSiteRead held the gate, the write is keyed on the claimed dossier's own id — and it touches no record, because the record it is for does not exist until a confirmation binds it under the organization gate",
	"internal/modules/people:DiscardSiteReadLogo":            "worker-loop clearing of RecordSiteReadLogo's own parked reference on a dossier that ended without a company; same seam and same admission as the write it undoes, it touches no record, and it names the orphaned object only while no organization does",
	"internal/modules/approvals:Stage":                       "staging is invoked BY an admitted mutation (the 🟡 path of a gated store call); the staging row records that actor",
	"internal/modules/approvals:StageAgentCall":              "Stage for a refused 🟡 agent CALL, admitted exactly as Stage is — by the gate that just refused the call, which ran scope, seat and tier before there was anything to stage. What it adds over Stage is a probe of the approvals this same call has already produced, so it can hand back the live one instead of minting a duplicate. The probe is bounded by the CALLER'S OWN credential (passport_id IS NOT DISTINCT FROM the calling principal's), which is strictly narrower than the redemption it has to agree with — deliberately, since the redemption enforces that binding only against a caller presenting a passport, and volunteering another credential's authority object is not something a deduplication may do. It answers an id and a boolean, never record data. The one thing it reads outside `approval` is the target's version column, an integer it compares and does not return",
	"internal/modules/approvals:StageInTx":                   "transactional form of Stage used by an admitted compose orchestration; it records the same actor and differs only in commit ownership",
	"internal/modules/approvals:StageOrJoinPendingInTx":      "StageInTx's joining twin, admitted the same way and by the same callers; it adds only the join-or-supersede decision over proposals of one kind against one target, never record data",
	"internal/modules/approvals:StageUnlessDeclined":         "Stage with one added refusal — it declines to re-offer a proposal a human already rejected — so it is admitted exactly as Stage is, by the gated mutation that reached it; the extra read is of the offers this same proposal produced, never record data",
	"internal/modules/approvals:HasPendingFor":               "existence probe consumed by gated sibling flows (the sweep's duplicate check); returns no record data",
	"internal/modules/approvals:HasPendingKind":              "existence probe consumed by gated sibling flows (the sweep's duplicate check); returns no record data",
	"internal/modules/approvals:RejectedChangesFor":          "reads back the proposals a human already REFUSED, so the gated sibling flow that staged them can tell whether it is about to redo one; the payloads are that flow's own, never record data",
	"internal/modules/approvals:RejectedChangesForTx":        "transactional form of RejectedChangesFor so the refusal check and the caller's write commit as one unit; same read, and the caller supplies only the commit boundary",
	"internal/modules/approvals:WithdrawInTx":                "retraction of an offer by the module that RAISED it, driven by a sweep with no human principal: the capture ledger ageing out a question nobody answered. It only ever takes a live offer AWAY (forced expiry, the supersession mechanism), so there is no authority to admit — nothing is created, decided, or disclosed, and a decided approval is left alone because what a human answered is not the caller's to take back",
	"internal/modules/deals:SeedDefaultsTx":                  "workspace-provisioning seed invoked by the boot bootstrap under the system principal (the compose-injected edge)",
	"internal/modules/deals:SeedPipelineTx":                  "the configured variant of the same boot seed (A107/ADR-0061): deployment-file pipeline, system principal, compose-injected edge",
	"internal/modules/deals:StageSemantic":                   "vocabulary lookup (stage → open/won/lost) consumed by gated flows; reads config, not records",
	"internal/modules/deals:ActiveDealColumns":               "the deal twin of people:ActivePersonColumns, on the same reasoning: it answers which cf_* columns the workspace has active and of what type — schema rather than a record — and exists because that catalog read cannot run inside the transaction UpdateDealTx is handed. The write it feeds takes deal:update before any value lands in those columns",
	"internal/modules/search:UpsertEmbedding":                "written by the outbox consumer under the system principal; reads happen through the gated search paths",
	"internal/modules/search:SeedBinding":                    "deployment-metadata marker (embed_store_binding is non-tenant, no workspace_id, no RLS) written once at boot under the system process, same posture as ai/callstore.go's EnsureConfig",
	"internal/modules/search:PopulatedIdentity":              "one-PK read of the non-tenant binding marker; the /readyz seam (Task 17) has no principal to gate on",
	"internal/modules/search:ReindexNeeded":                  "derived signal over the non-tenant marker plus a system-principal entity scan; consumed only by the compose ops surface, which is itself the gated entry point",
	"internal/modules/search:ClaimAndEnqueueReembedding":     "CAS on the non-tenant marker; the compose confirm endpoint (admin+ops write grant, ADR-0068 design §5.6-swap) is the gated entry point that calls it",
	"internal/modules/search:ReleaseReembedding":             "hands the non-tenant marker back at the end of a run, driven by the River jobs themselves, not a human principal",
	"internal/modules/search:PendingByWorkspace":             "fleet rollup read as the system principal (mirrors EmbedGen, embedgen.go:51-56); consumed only by the compose preview/status surface, which is the gated entry point",
	"internal/modules/search:TokenSumByWorkspace":            "fleet rollup read as the system principal, same posture as PendingByWorkspace — an aggregate SUM/COUNT, never row data",
	"internal/modules/search:EntitiesPending":                "totals PendingByWorkspace across the fleet; same system-principal posture, no row data",
	"internal/modules/search:Reembed":                        "the reindex pass's body, driven under the system principal, same posture as EmbedGen/PendingByWorkspace; the run's own enqueue (via ClaimAndEnqueueReembedding) is the gated entry point",
	"internal/modules/search:SweepWorkspaceEmbeddingDrift":   "periodic worker sweep (ADR-0069 §3a): heals identity-matched embedding gaps under the system principal, same posture as Reembed — no request, no human actor to gate",
	"internal/modules/collections:SegmentEngine":             "answers a filter vocabulary, never a record: a field allow-list, its types and the resource's fixed base clause. The row-scope clause is composed downstream where rows are actually selected (storekit.Query.SelectIDs), and each of the three callers is gated in its own right — list-create validation compiles the predicate and discards the SQL, membership evaluation and filtered export both select through SelectIDs. Its one database read is the workspace-bound custom-field catalogue lookup, which is ungated for the reason its own entry states",
	"internal/modules/customfields:ActiveColumns":            "called from inside a record store's own gated Get/List/Create/Update, whose object-level RBAC already ran; the column names/types it answers are workspace-visible schema (the same shape custom_field:read already exposes), not row data a second gate would need to narrow",
	"internal/modules/customfields:FilterableColumns":        "the filter-side twin of ActiveColumns and ungated for the same reason: it answers which cf_* columns exist and of what type — workspace-visible schema, the same shape custom_field:read already exposes — never row data a second gate would narrow. It additionally answers retired columns, which is still schema: the values behind them are reached only through a record store's own gated read",
	"internal/modules/activities:UnlabeledCaptureEmails":     "classify-backlog read driven by the worker sweep under the workspace GUC, no human principal (ADR-0063); the rows were admitted at capture time and the labels route attention only",
	"internal/modules/activities:SetCaptureLabel":            "classify verdict write driven by the worker sweep under the workspace GUC; a CAS on capture_label IS NULL that touches nothing but the two label columns — attention routing, not a record mutation (§3.2)",
	"internal/modules/activities:HideCapturedNoiseTx":        "the ADR-0072 noise disposition's hide, driven by the verdict engine's system principal on the caller's transaction; its authority is the floored verdict that resolved the ledger row, and there is no human principal in a sweep for object-RBAC to admit — the write is idempotent, reversible, and touches only archived_at",
	"internal/modules/activities:RedactCapturedNoiseTx":      "the same disposition's delayed content redaction, driven by the same sweep once the undo window has closed; gating it on a human's permissions would mean a workspace whose reviewer lost access keeps the mail it decided to redact — the obligation outlives any one principal",
	"internal/modules/activities:ReconcileMessageIdentityTx": "worker-loop correction on the send dispatcher's own transaction, system principal, no human principal in the call: it rewrites the transport identity of a message this workspace ALREADY sent onto the one the provider stamped. It discloses nothing and creates nothing. It reaches exactly two rows: the send's own activity, which the delivery the caller holds names, and — only when the natural-key index says another row already holds the stamped identity — the provider's captured outbound echo of this same message, which it merges in. WHICH row that second one may be is constrained IN SQL rather than trusted from the provider's string, and that predicate is now the whole boundary: same source system, an email, outbound, connector-captured, and not created before the send it echoes. A collision matching none of that is refused, not absorbed. Gating any of it on the sender's seat would let a seat revoked between staging and transmit strand a sent message under an identity that exists nowhere on the wire",
	"internal/modules/people:SetChannelIdentityBlocked":      "reachability bookkeeping driven by the telegram ingest worker under the workspace-channel connector principal (compose/telegramingest.go builds that principal onto the context before it classifies the update, so this write has a named actor), never a human caller: the trigger is Telegram's own my_chat_member delivery, which the poller received through a resolved status='connected' connection. It writes no record data — only blocked_at on the one identity the delivery itself names, which is what bounds the write — and the flip is not silent: the audit row and person.updated event it commits alongside are stamped from that same principal, so a reachability change is traceable to the delivery that caused it",
	"internal/modules/people:EnqueueIdentityConflict":        "the sink capture's ensure path hands routeExact's rival pair to (compose/capture.go's raiseIdentityConflict), under the same connector principal with no human in the call: recording that two independently-established keys name two DIFFERENT people writes no key onto either of them, so it can neither merge nor disclose — there is nothing for object-RBAC to admit, and the human authority sits on the queue's own disposition, not on recording that a disagreement exists. Reachability is honestly stated: a conflict needs a candidate carrying two different KINDS of exact key, and no shipped ensure path builds one — Telegram supplies a channel identity and nothing else, and the API create's address lane is refused before the ladder reads (people/creatededupe.go) — so today only the phone and channel lanes can speak, one at a time, and this entry point waits for a second key kind rather than serving traffic",
	"internal/modules/people:SignatureCandidates":            "enrich-backlog read driven by the worker sweep under the workspace GUC, no human principal (ADR-0063 §2.9); reads only connector-created rows still missing both fields",
	"internal/modules/people:MarkSignatureRead":              "the same sweep's read cursor: it records WHICH mail the model was already shown for a person, so the pass stops paying for the same empty signature nightly — a bookkeeping row with no record data, written under the workspace GUC with no human principal to admit",
	"internal/modules/people:OrgNameCandidates":              "promotion-backlog read driven by the nightly sweep under the workspace GUC, no human principal (PO-F-2a); reads only provisionally-named organizations and the signature evidence naming them",
	"internal/modules/privacy:EvaluateInstallation":          "the retention pass: its one production caller is the privacy-retention River worker (compose/jobs_privacyretention.go), which builds a PrincipalSystem actor onto the context before the call, so no human principal exists here to admit. Gating it on one would be wrong rather than merely redundant — a retention obligation outlives any seat, and an installation whose reviewer lost access must still age out the data it promised to",

	// comms: delivery machinery, not the message. StageTx runs inside the
	// caller's own transaction, alongside the activity write that already
	// passed the gated activity:create check — the outbound send itself was
	// admitted there. But activity:create alone would only prove the actor
	// may create an activity, not that the delivery may send through THEIR
	// mailbox — the security-relevant fact this store owns — so StageTx
	// itself derives user_id from the authenticated principal on ctx
	// (storekit.Actor) and fails closed when none resolves to an app_user;
	// no caller input can name a different sender. Object-RBAC has nothing
	// left to narrow once that derivation stands. Load/RecordSent/Park/
	// ParkTransmitted/RecordFailure/RecordDeferral/MarkInFlight/ClearInFlight
	// are the dispatcher's own state-machine steps, driven by the outbox/River worker
	// under the system principal with no human principal in the call at all;
	// nothing here discloses a record to anyone — the reason each of them
	// writes is an operator-facing transport diagnosis, not tenant data.
	"internal/modules/comms:StageTx":           "derives user_id from the authenticated principal (storekit.Actor) and fails closed with no caller-suppliable override; the activity:create check on the shared transaction admits the send action itself, but the sending IDENTITY is enforced here, in the store, not inherited from that check",
	"internal/modules/comms:StageChannelTx":    "the channel-shaped twin of StageTx and the same posture: it derives user_id from the authenticated principal (stagingUser, shared with StageTx so neither can grow a caller-suppliable override) and runs inside the caller's already-gated activity transaction",
	"internal/modules/comms:StageControllerTx": "the INSTALLATION sending in its own name, so there is no human identity to derive and no seat to check: the caller is a compose seam acting for the system principal, and what constrains the write instead is the registered template — an unregistered key, a placeholder that disagrees with the material, and an absent registry are all refused before any row is written. Admission for the underlying act happened where the timeline activity was logged, on the same transaction",
	"internal/modules/comms:ClearPayloadRef":   "retires spent one-time link material, called by the dispatcher under the system principal after the relay has taken the message or the delivery reached a terminal state. It only ever NULLs a reference — there is no read, no other column and nothing a caller could name — and gating it on a human would leave live credentials standing whenever a job, rather than a person, closed the send",
	"internal/modules/comms:Load":              "worker-loop step: the dispatcher claims the next attempt under the system principal (no human principal in a job); admission happened when the message was staged",
	"internal/modules/comms:RecordSent":        "worker-loop terminal transition on the connector's own success receipt, system principal, same posture as Load",
	"internal/modules/comms:RecordBounce":      "capture-loop write under the connector principal — no human is present when a mailbox pull reads a delivery report. Authorization is the three-way match the store itself enforces: the named message must be a row this store sent, owned by the capturing mailbox's user (the connector principal's own user_id), to the address the report says failed; a report failing any of the three writes nothing",
	"internal/modules/notices:Create":          "the notice transport's one writer, reached from the automation engine's notify action and the lead-SLA escalation under the system principal — there is no human request to gate, the recipient comes from the producing flow's own decoded configuration, and captured_by records the engine identity. The row's FK refuses a recipient who is not a live seat, and the read side is where a person's scope binds",
	"internal/modules/notices:UnreadFor":       "the personal notices read. It writes nothing, its one predicate is the attribution itself — recipient_user_id = the authenticated caller, never a parameter — and a caller with no person behind it is refused with the permission sentinel before any query, which the Worklist renders as a withheld lane",
	"internal/modules/notices:MarkRead":        "the recipient settling their own notice: the statement is scoped recipient_user_id = the authenticated caller, so another person's notice reads as absent (404) and existence stays hidden; a caller with no person behind it gets the permission sentinel before any query",
	"internal/modules/comms:Park":              "worker-loop terminal transition on an unretryable provider failure, system principal, same posture as Load",
	"internal/modules/comms:ParkTransmitted":   "the same terminal transition for a delivery the provider ALREADY accepted, system principal, same posture as Park: it keeps the provider's own message id on the row when the receipt write failed, so a message the recipient is holding is not recorded as unsent. It reads nothing back and discloses nothing",
	"internal/modules/comms:RecordFailure":     "worker-loop retry-bookkeeping transition on a transient provider failure, system principal, same posture as Load",
	"internal/modules/comms:RecordDeferral":    "worker-loop pacing transition, system principal, same posture as Load: it notes which rule is holding a delivery back and gives back the attempt that dispatch counted, because a deferral reached no provider. It discloses nothing and can only ever leave a pending delivery pending",
	"internal/modules/comms:MarkInFlight":      "worker-loop at-most-once transition, system principal, same posture as Load: it stamps one timestamp on a pending delivery before the provider call so a crashed attempt is visible to the next one. It reads nothing back and discloses nothing",
	"internal/modules/comms:ClearInFlight":     "the retraction half of MarkInFlight, same posture: it nulls that timestamp once the provider gave a definite answer. Both are timestamps about this system's own transport attempt, not tenant data",
})

// storeEntryPointScope proves the gate's roots: every file that declares an
// entry point of this shape lives under one of them, or is ratified below.
//
// internal/platform/settings is here rather than in the exempt set because the
// gate CAN judge it and does: the settings store is a real governed write path
// (ADR-0090/A135), and the `setting` table carries no RLS beneath it — so the
// object gate is the only control there, which is exactly what this gate
// exists to check. Ratifying it as "outside the gate's business" would have
// hidden the one store whose gate has no backstop.
//
// Both entry points are METHODS on *Store (Raw, SetRaw) for this reason. The
// typed Get/Set helpers beside them are generic, and Go forbids generic
// methods — a package-level generic function does not match the shape this
// gate collects, so writing the store that way would have left the write path
// invisible here while this comment claimed otherwise.
var storeEntryPointScope = gatekit.Scope{
	Roots:   []string{modulesDir, settingsStoreDir},
	Subject: declaresStoreEntryPoint,
	Exempt:  entryPointsOutsideModules,
}

// settingsStoreDir is the platform tier this gate reaches into. It is one
// package, named explicitly rather than by widening to all of
// internal/platform: the rest of that tier owns no domain rows, and sweeping
// it in would mean waiving files this gate has not judged.
const settingsStoreDir = "internal/platform/settings"

// entryPointsOutsideModules ratifies the files that hold this entry-point shape
// outside internal/modules. Each says what the methods are; none says they are
// correctly gated, because this gate has not judged them — bringing a tier under
// it is its own decision, taken with its own evidence, and ratifying the sweep is
// not that decision. The entries are the ratchet: a file that stops holding the
// shape is reported stale here, so the question cannot be forgotten.
var entryPointsOutsideModules = gatekit.Waive(map[string]string{
	"internal/compose/attention/waitinglane.go":     "attention.Service.HiddenBacklog — the guardrail over the queue's own hiding rules, and a projection with no read of its own: every figure comes from activities.Store.HiddenWaiting across the Waiting seam, and THAT method is inside this gate's roots and opens with auth.Require on activity plus the same ActivityContentClause the waiting lane composes, so the row scope is applied where the SQL is. This file reads no table and writes nothing. Ratified here only as a subject the roots do not cover",
	"internal/compose/org360/assemble.go":           "org360.Service.Assemble — a compose read service assembling the record-360 view across domain tables; this package does reference the platform auth gate (auth.Require, EnsureVisible, the scope clauses), but whether the gate's transitive resolution proves that for each entry point is a judgement this change has not made",
	"internal/compose/org360/graph.go":              "org360.Service.Graph — the same read service's relationship graph, in a package that reaches auth through its section helpers; enrolling compose in this gate is a separate decision from proving where the entry points are",
	"internal/compose/org360/coverage.go":           "org360.Service.Coverage — the account's coverage reading, ratified on the same terms as its siblings: it opens with the people store's own GetOrganizationTx (auth.Require + EnsureVisible), the roster it folds carries the person row scope as a predicate, and the deals it lists go through scopeClause; enrolling compose in this gate is the separate decision the other org360 entry points are ratified under",
	"internal/compose/org360/contactlist.go":        "org360.Service.ContactPage — the account's contact list, the paging surface behind the 360's people section; it opens with the same people-store GetOrganizationTx (auth.Require + EnsureVisible) that Graph and Assemble open with, and the roster read it pages carries the person row scope as a predicate, but enrolling compose in this gate is the same separate decision its siblings are ratified under",
	"internal/compose/org360/introdraft.go":         "org360.Service.IntroRequestDraft — the ask a rep sends a colleague. It opens with auth.RequireHuman, then the people store's own GetOrganizationTx (auth.Require + EnsureVisible) refuses an account this caller cannot open; the contact is read through contactIdentity under the person row scope, and the route through mayReadRoutes + contactRoutes, so a caller without activity:read learns nothing about who can reach whom. It WRITES nothing at all — no draft row, no activity, no audit entry beyond the model call's own — which is why there is no mutation for this gate to be the last line in front of. Ratified here only as a subject the roots do not cover",
	"internal/compose/org360/roleproposals.go":      "org360.Service.ProposeRoles — the buying-role reading, and the one WRITE in this package's ratified set. It opens with auth.RequireHuman, then asks relationship.create and deal.update as OBJECT grants, then auth.EnsureWritableLive on the deal itself — that last one matters because the seats are written under a substituted system principal, which is unbounded, so the caller's row-level write authority has to be established while they are still the actor. Every read that feeds the prompt is the coverage card's own (people.StrengthForOrgContacts, contactIdentity, deals.Stakeholders) and the message bodies carry auth.ActivityContentClause. The seats themselves go through people.Store.CreateRelationshipTx, which this gate DOES judge directly; enrolling compose in this gate is the separate decision its siblings are ratified under",
	"internal/compose/org360/dismissal.go":          "org360.Service.DismissSuggestion — the suggestion-dismissal write; it opens with auth.RequireHuman and auth.Require, so it is not a suspected gap, but this gate has not been the thing that checked it",
	"internal/compose/org360/viewbaseline.go":       "org360.Service.Acknowledge — the record-view acknowledgement write, likewise opening with auth.RequireHuman and auth.Require; ratified here only as a subject the roots do not cover",
	"internal/compose/project360/assemble.go":       "project360.Service.Assemble — the project record-360 read, the company and person views' sibling and ratified on the same terms as person360/assemble.go: its mandatory anchor read opens with the deals store's own auth.Require + EnsureVisible (GetProjectTx) and every section is a module store's gated transaction-taking read, but whether this gate's transitive resolution proves that per entry point is the judgement enrolling compose would have to make",
	"internal/compose/person360/assemble.go":        "person360.Service.Assemble — the person record-360 read, the company view's sibling and ratified on the same terms as org360/assemble.go: its mandatory root read opens with the people store's own auth.Require + EnsureVisible and every section carries its object grant, but whether this gate's transitive resolution proves that per entry point is the judgement enrolling compose would have to make",
	"internal/compose/person360/handlers.go":        "person360.Handlers.AcknowledgePersonView — the person view's visit acknowledgement, the twin of org360/viewbaseline.go and likewise opening with auth.RequireHuman; ratified here only as a subject the roots do not cover",
	"internal/compose/person360/momentdismissal.go": "person360.Service.DismissMoment — the person page's moment dismissal, the twin of org360/dismissal.go: it opens with auth.RequireHuman, auth.Require and auth.EnsureVisibleLive, so it is not a suspected gap, but this gate has not been the thing that checked it",
	"internal/compose/dealstatus/service.go":        "dealstatus.Service.Get — the deal page's status card. It gathers its facts through gated reads and nothing else: deals.Store.GetDeal and DealHealth (deal.read plus the deal row scope), activities.Store.ListActivities and ListOpenTasks (activity.read plus the link-walk scope, narrowed to the deal), dealrooms.Store.ListRooms and ListThreads (deal_room.read plus the deal scope; a caller without the grant reads no room and the card says nothing about one). The writing over them is a pure function of what those reads returned. The one row it writes is its own cache entry in deal_status_card, keyed by the reading user so no card crosses readers, and derived content carries no audit or outbox row",
	"internal/compose/dealstatus/moves.go":          "dealstatus.Service.CachedMoves — the same card's move, read for a page of deals at once so the worklist can name a step it does not decide. It READS ONLY THE CACHE and assembles nothing, so it makes no record read this gate could be the last line in front of; what it returns was written by Service.Get above, under that caller's own gated reads. The admission is the cache key itself: the WHERE names user_id, and the workspace binding is WithWorkspaceTx's, so a reader reaches only cards written from records they were already admitted to. A deal they have no card for is simply absent from the answer, which is the same shape a refused read would take. It writes nothing at all",
	"internal/compose/dealstatus/cards.go":          "dealstatus.Service.CachedCards — the same card's VERDICT, read for a page of deals at once so the worklist can carry a standing it does not decide. It stands where CachedMoves beside it stands: it READS ONLY THE CACHE and assembles nothing, so it makes no record read this gate could be the last line in front of, and what it returns was written by Service.Get under that caller's own gated reads. The admission is the cache key itself — the WHERE names user_id, and the workspace binding is WithWorkspaceTx's — so a reader reaches only cards written from records they were already admitted to. It is NOT ungated beyond that: the verdict sentence is model-written FROM the timeline and grounding.go requires it to cite the records it rests on, so its text restates their content, and content derived from an activity carries the audience predicate wherever it is served. So the activities the sentence cites are re-asked on every read through the same readableActivities CachedMoves uses (auth.Require plus auth.ActivityContentClause), and a standing whose cited message this reader may no longer read is dropped whole rather than served. It writes nothing at all",
	"internal/compose/worklistsnap/store.go":        "worklistsnap.Service.Freeze and Resume — one reader's position in one walk through their own worklist. It reads and writes NO tenant record: the row holds a list of (source, row_id) identities, the four frozen bucket counts, a fingerprint and two timestamps, and deliberately no title, subject, name or evidence — every page re-reads the live rows through the assembler's own gated lanes and renders only what that caller may see at that moment. So there is no record this gate could be the last line in front of. The admission is the key itself: both statements name reader_id from the authenticated principal, readerOf refuses anything but a human seat, and the workspace binding is WithWorkspaceTx's — a colleague's walk simply does not match, which TestAColleaguesWalkIsNotResumable holds against the real table. Per-reader derived state, so no audit and no outbox row, ratified in moduleaudits alongside org_brief and deal_status_card",
	"internal/compose/attention/teamexceptions.go":  "attention.Service.TeamExceptions — the lead's read of what is going wrong on their team. It opens no transaction and reads no table: every row comes from Service.Assemble, which is already ratified above as a projection over the owning modules' own gated reads, and this file only decides which of those rows is a condition a lead can act on. So there is no record it could be the last line in front of. The admission is requireLeadTier plus the caller's own visibility, exactly as TeamBoard and HiddenBacklog take it — a rep is refused before any row is judged, and the rows themselves were already narrowed by the lanes that produced them. Ratified here only as a subject the roots do not cover",
	"internal/compose/attention/handled.go":         "attention.Service.HandledForYou — the reader's own receipt of what was done for them. It opens no transaction and reads no table: every row comes from the Receipts seam, whose implementation is the approvals module's own gated read, and this file only bounds and shapes what that returned. So there is no record it could be the last line in front of. The admission is auth.RequireHuman plus that seam's own scope — a receipt is the acting reader's, and a principal with no human behind it has nobody the acts were taken for. Ratified here only as a subject the roots do not cover", "internal/compose/meetingbrief/service.go": "meetingbrief.Service.Get — the pre-meeting brief read. UNLIKE its personbrief sibling it carries no auth.RequireHuman: agents read it through prep_for_meeting, so that one tool and the person page cannot answer the same question differently. What admits the read instead is auth.Require on activity and person, then auth.EnsureActivityContentVisibleLive on the meeting itself, and every record it is written from arrives through the caller's own gated 360 and the people store's own claim read, so the row-scope gates run in those reads rather than here. An agent is capped the same way a person is: a passport's authority is re-derived from the granting human's live seat on every call, never read off the principal. It also writes nothing — there is no cache — so there is no mutation for this gate to be the last line in front of",
	"internal/compose/orgdossier/cachedread.go":       "orgdossier.Service.CachedSections — the cache-READ half of the company dossier, for a drafter that must never trigger an assembly. It opens with auth.RequireHuman and resolves the acting user, and the cache it reads is keyed per reader, so a row assembled for somebody else is not reachable through it; the dossier itself was assembled through the caller's own gated reads. Ratified here only as a subject the roots do not cover",
	"internal/compose/personbrief/service.go":         "personbrief.Service.Get — the relationship brief read, ratified on the same terms as its orgbrief sibling: it opens with auth.RequireHuman and every record it is written from arrives through the caller's own gated 360, so the row-scope gates run in that read rather than here",
	"internal/compose/personresearch/service.go":      "personresearch.Service.Run and .Save — the deep-research surface. Run opens with auth.RequireHuman and reads only through the caller's own gated person360, so it can research nobody they cannot open; Save carries its write into people.Store.SaveResearchClaims, which this gate DOES judge directly. Ratified here only as a subject the roots do not cover",
	"internal/compose/orgdossier/service.go":          "orgdossier.Service.Get — the company dossier read, opening with auth.RequireHuman and assembling only from reads the caller makes themselves, so the row-scope gates run in the people store this calls rather than here",
	"internal/compose/orgdossier/growthfitservice.go": "orgdossier.GrowthFitService.Get — the growth-fit read, ratified on the same terms as its dossier sibling: it opens with auth.RequireHuman and every record it counts arrives through a read the caller makes themselves, so the row-scope gates run in the people store rather than here",
	"internal/compose/orgbrief/service.go":            "orgbrief.Service.Get and .Ask — the organization brief read and its question surface, both opening with auth.RequireHuman; whether RequireHuman alone is the right admission for them is a question for the tier's own review, not for this sweep",
	"internal/compose/orgscan/service.go":             "orgscan.Service.Get and .Ensure — the account scan, ratified on orgbrief's terms: both open with auth.RequireHuman and resolve the acting user, the row they read is keyed per reader behind auth.EnsureVisible on the account, and everything the scan is written from arrives through the caller's own gated 360 and the content-gated words read, so the row-scope gates run in those reads rather than here. Run is the worker's entry and binds the reader's own principal before any read",
	"internal/compose/org360/advice.go":               "org360.Service.UndismissedAdvice and .KeepUndismissed — the advice seam the account scan merges with. Both resolve the acting user, both run behind auth.EnsureVisible on the account, and the rules they run are the same grant-gated reads the composite's own suggestions section runs, so the object and row gates are those reads' rather than a second spelling here",
	"internal/compose/persondraft/service.go":         "persondraft.Service.Draft — the person-side email draft, ratified on the same terms as its accountdraft mirror: it opens with auth.RequireHuman and every record it grounds in arrives through the caller's own gated person 360, so the row-scope gates run in that read rather than here. It also writes nothing, so there is no mutation for this gate to be the last line in front of",
	"internal/compose/leaddraft/service.go":           "leaddraft.Service.Draft — the lead-side email draft, ratified on the same terms as its persondraft mirror: it opens with auth.RequireHuman, the lead arrives through the people store's own gated GetLead and the correspondence through the activities store's own list, so the row-scope gates run in those reads rather than here. It also writes nothing, so there is no mutation for this gate to be the last line in front of",
	"internal/compose/accountdraft/service.go":        "accountdraft.Service.Draft — the account-started email draft, ratified on the same terms as its orgbrief sibling: it opens with auth.RequireHuman and every record it grounds in arrives through the caller's own gated 360, so the row-scope gates run in that read rather than here. It also writes nothing, so there is no mutation for this gate to be the last line in front of",
	"internal/compose/analyticssharestore.go":         "AnalyticsShareStore.Issue, .Resolve and .Revoke — the share link's writer and reader. Issue and Revoke DO open with auth.Require on forecast (create and delete), so neither is a suspected gap; Resolve deliberately opens with NO gate on the caller, because its caller is a recipient holding a token and not a seat, and what admits it is the token digest plus identity.IssuerStillHolds re-evaluating the forecast:read of the issuer as it stands today. The reading a resolved share then serves runs through the forecast engine under the principal of the RECIPIENT, so the row scope and the field masks are applied there. Ratified here only as a subject the roots do not cover",
	"internal/compose/attention/feed.go":              "attention.Service.Assemble — the day's read, matched because its dependency interfaces carry List/Count methods. It holds no store and opens no transaction: every lane is a read through the owning module's own gated entry point (approvals.Service.ListWire, people.Store.ListDedupeCandidates and CountOpenDedupeCandidates, activities.Store.ListActivities), and a lane whose read is refused is omitted and named rather than returned empty. Ratified here only as a subject the roots do not cover",
	"internal/compose/attention/worklist.go":          "attention.Service.Worklist — the same day's read, ranked. Matched for the reason feed.go is: its dependency interfaces carry List/Count methods. It holds no store and opens no transaction of its own, and it reads NOTHING the lane feed did not already read — it calls Assemble and re-projects the result, so every gate that admitted a lane there is the gate that admitted it here, and a lane refused there arrives refused. Ratified here only as a subject the roots do not cover",
	"internal/compose/attention/teamboard.go":         "attention.Service.TeamBoard — the manager's counts over the same work, matched for the reason its two siblings are. It holds no store and opens no transaction. It admits the read on the reader's own ROW SCOPE first, before touching a source: a tier below team is ErrPermissionDenied, and an unbound membership reader is refused rather than answered as an empty team. Every count then comes from a read the caller makes themselves — identity.Service.LiveTeammatesOfCaller (auth.RequireHuman, and the roster it walks is the caller's own teams), activities.Store.WaitingReplies and OverdueLoadByAssignee (auth.Require plus the activity scope clauses), the at-risk lane's own gated list — so what it reports is bounded by what this reader may already open, and a source that refuses fails the board rather than drawing a column of zeros. It writes nothing. Ratified here only as a subject the roots do not cover",
	"internal/compose/runnerservice.go":               "RunnerService.TickWorkspace and .HandleEvent — the agent runner's worker-loop and event-bus seams, which carry no human principal at all; that posture is what the module-side waivers spell out for their sweep entry points, and applying the same reasoning to compose needs the tier brought under the gate first",
	"internal/platform/blobstore/memory.go":           "memoryStore's Put/Get/Delete/Health — a blobstore.Store driver, matched only because the receiver type name ends in \"Store\". It moves opaque bytes under a caller-supplied key and holds no record and no workspace column, so there is no RBAC object for this gate's rule to name",
	"internal/platform/blobstore/s3.go":               "s3Store's Put/Get/Delete/Health — the same driver interface over S3, matched by the same receiver-name suffix; the admission that matters for a blob is taken by the module surface that mints its key, not by an object-storage client",
	"internal/platform/blobstore/fs.go":               "fsStore's Put/Get/Delete/DeletePrefix/Health — the same driver interface over a local directory, matched by the same receiver-name suffix, and ratified on the same terms as its two siblings: the admission that matters for a blob is taken by the module surface that mints its key. What is different here is worth naming rather than hiding behind the sameness — a key becomes a PATH, so fsStore.path refuses an absolute or traversing key outright (ErrInvalidKey). That is not this gate's object rule; it is the key prefix that carries tenant isolation, and a traversal would walk through it and serve a different object than the row named",
})

// declaresStoreEntryPoint reports whether the file holds an entry point of the
// shape this gate judges. Integration-tagged files are excluded for the same
// reason the walk excludes them: the obligation binds production stores, and a
// tagged file can never reach a shipped binary.
func declaresStoreEntryPoint(path string, file *ast.File) bool {
	if isIntegrationTagged(path) {
		return false
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && isStoreEntryPoint(fn) {
			return true
		}
	}
	return false
}

// isStoreEntryPoint is the three-part shape: exported, a pointer receiver on a
// *Store or *Service type, and a context.Context parameter.
func isStoreEntryPoint(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || !fn.Name.IsExported() {
		return false
	}
	star, ok := fn.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	receiver, ok := star.X.(*ast.Ident)
	return ok && storeReceiver(receiver.Name) && takesContext(fn)
}

// gateFnInfo is what the gate needs to know about one function name in a
// package: whether any body under that name references auth, and every
// name it mentions (the transitive-resolution edges).
type gateFnInfo struct {
	auth  bool
	calls map[string]bool
}

// gatePkg is one package's function index, bucketed by the receiver each
// function is declared on. The "" bucket holds package-level functions,
// which every receiver in the package may call.
type gatePkg map[string]map[string]*gateFnInfo

// visibleTo returns the functions an entry point on recv can reach by name:
// its own receiver's methods, plus the package-level ones. A name held by
// both buckets is merged rather than shadowed — see the package comment on
// where this gate stays optimistic and why.
func (p gatePkg) visibleTo(recv string) map[string]*gateFnInfo {
	fns := make(map[string]*gateFnInfo, len(p[""])+len(p[recv]))
	for name, info := range p[""] {
		fns[name] = info
	}
	for name, info := range p[recv] {
		if pkgLevel, both := fns[name]; both {
			fns[name] = mergeGateFnInfo(pkgLevel, info)
			continue
		}
		fns[name] = info
	}
	return fns
}

// mergeGateFnInfo unions two same-named functions the index cannot tell
// apart at a call site. It builds a THIRD value rather than mutating
// either: the buckets are shared across every entry point on the package,
// so folding one into the other would leak this receiver's edges into the
// next receiver's view.
func mergeGateFnInfo(a, b *gateFnInfo) *gateFnInfo {
	merged := &gateFnInfo{auth: a.auth || b.auth, calls: make(map[string]bool, len(a.calls)+len(b.calls))}
	for _, src := range []*gateFnInfo{a, b} {
		for name := range src.calls {
			merged.calls[name] = true
		}
	}
	return merged
}

// gateEntry is one exported *Store/*Service method — a store entry point
// the gate must prove reaches auth. recv carries the receiver type name so
// resolution is scoped to it.
type gateEntry struct{ dir, recv, name string }

// collectStoreEntryPoints returns, per package dir, the receiver-bucketed
// function index plus the list of exported *Store/*Service methods to check.
func collectStoreEntryPoints(t *testing.T) (map[string]gatePkg, []gateEntry) {
	t.Helper()
	var entries []gateEntry
	for _, src := range storeEntryPointScope.Files(t) {
		dir := filepath.ToSlash(filepath.Dir(src.Path))
		for _, decl := range src.File.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || !isStoreEntryPoint(fn) {
				continue
			}
			entries = append(entries, gateEntry{dir, receiverName(fn), fn.Name.Name})
		}
	}
	return packageFunctionIndex(t), entries
}

// receiverName is the receiver's type name with any pointer stripped, or ""
// for a package-level function. Both spellings are collected because the
// index holds whole packages: a value-receiver type (Handlers) is a bucket
// this gate must keep separate just as much as a pointer one.
func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	switch typ := fn.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if ident, ok := typ.X.(*ast.Ident); ok {
			return ident.Name
		}
	case *ast.Ident:
		return typ.Name
	}
	return ""
}

// packageFunctionIndex indexes every function in every package under the
// scope's roots, bucketed by receiver. It reads WHOLE packages, not only the
// files that hold an entry point, because the auth call that gates a method
// routinely sits in a same-package helper in another file — indexing only the
// entry-point files would report those methods ungated.
//
// Two functions can still land in one bucket under the same name when build
// tags mean only one of them ever compiles, so the merge below stays.
func packageFunctionIndex(t *testing.T) map[string]gatePkg {
	t.Helper()
	pkgs := map[string]gatePkg{}
	fset := token.NewFileSet()
	for _, root := range storeEntryPointScope.Roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				isIntegrationTagged(path) {
				return err
			}
			path = filepath.ToSlash(path)
			dir := filepath.ToSlash(filepath.Dir(path))
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			if pkgs[dir] == nil {
				pkgs[dir] = gatePkg{}
			}
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil {
					continue
				}
				recv := receiverName(fn)
				if pkgs[dir][recv] == nil {
					pkgs[dir][recv] = map[string]*gateFnInfo{}
				}
				info := pkgs[dir][recv][fn.Name.Name]
				if info == nil {
					info = &gateFnInfo{calls: map[string]bool{}}
					pkgs[dir][recv][fn.Name.Name] = info
				}
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					if sel, ok := n.(*ast.SelectorExpr); ok {
						if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "auth" {
							info.auth = true
						}
						info.calls[sel.Sel.Name] = true
					}
					if id, ok := n.(*ast.Ident); ok {
						info.calls[id.Name] = true
					}
					return true
				})
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return pkgs
}

// reachesAuthGate resolves gatedness transitively over the calls a name can
// reach, matched by name within the view visibleTo built for one receiver;
// seen breaks recursion cycles.
func reachesAuthGate(fns map[string]*gateFnInfo, name string, seen map[string]bool) bool {
	if seen[name] {
		return false
	}
	seen[name] = true
	info, ok := fns[name]
	if !ok {
		return false
	}
	if info.auth {
		return true
	}
	for c := range info.calls {
		if _, ok := fns[c]; ok && reachesAuthGate(fns, c, seen) {
			return true
		}
	}
	return false
}

func TestEveryStoreEntryPointIsAuthGated(t *testing.T) {
	t.Parallel()
	defer ungatedEntryPoints.AssertAllMatched(t)
	defer entryPointsOutsideModules.AssertAllMatched(t)

	pkgs, entries := collectStoreEntryPoints(t)

	for _, e := range entries {
		if reachesAuthGate(pkgs[e.dir].visibleTo(e.recv), e.name, map[string]bool{}) {
			continue
		}
		key := e.dir + ":" + e.name
		if ungatedEntryPoints.Waived(t, key) {
			continue
		}
		t.Errorf("%s: exported %s reaches no auth gate (directly or via same-package helpers) — every store entry point is RBAC-gated, or the exception is ratified in ungatedEntryPoints", e.dir, e.name)
	}
}

// storeReceiver matches the store and service receivers by SUFFIX, not
// by exact name. A module whose store is called MirrorStore or RunStore
// is no less a store, and matching only the bare names left those
// outside this gate entirely — invisible coverage reads exactly like
// real coverage.
func storeReceiver(name string) bool {
	return strings.HasSuffix(name, "Store") || strings.HasSuffix(name, "Service")
}

// takesContext keeps the gate on ENTRY POINTS. A method that takes no
// context does no request work — option setters (WithClock), accessors
// and constructors — and demanding an auth gate of them would grow a
// ratification list that says nothing about whether the real entry
// points are covered.
func takesContext(fn *ast.FuncDecl) bool {
	for _, param := range fn.Type.Params.List {
		if sel, ok := param.Type.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "context" && sel.Sel.Name == "Context" {
				return true
			}
		}
	}
	return false
}

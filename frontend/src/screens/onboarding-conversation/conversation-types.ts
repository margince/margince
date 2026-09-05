import type { MessageKey } from "../../i18n/en";

// The vocabulary of the onboarding conversation: acts, phases, thread
// entries, and the event union the reducer in conversation-machine.ts
// consumes. Types only — behaviour lives with the transition table.

export type ConversationAct =
  | "welcome"
  | "company"
  // The installation's reporting basis — base currency and reporting timezone
  // — asked of the creator once the company is confirmed and before any step
  // about the person answering. A member's installation has already settled
  // it, so the act is the creator's alone.
  | "basis"
  // The question a creator is asked once the basis is settled: will they work
  // in Margince themselves? Voice and connect are walked now by someone who
  // will; an administrator setting the installation up for others finishes
  // here, and the first person they invite walks them instead.
  | "invite"
  // Where a creator who declined the invite adds the first person who will
  // work in Margince, and finishes.
  | "team"
  | "voice"
  | "connect"
  // The last word before the app: what the agent may change on its own,
  // prefilled from what is already recorded. Every path ends here — the team
  // act's and the connect act's.
  | "prefs"
  | "done";

export type ConversationPhase =
  | "co.intro"
  | "co.reading"
  | "co.clarify"
  | "co.review"
  | "co.manual"
  // The restore landing spot: a returning session whose company is already
  // confirmed is reconstructed here, and RESUME routes it onward. Live
  // confirmation advances straight to the next act because a reducer cannot
  // self-advance out of a momentary state.
  | "co.confirmed"
  | "bs.ask"
  | "vo.collecting"
  | "vo.speaker"
  | "vo.building"
  | "vo.result"
  | "vo.skipped"
  | "in.ask"
  | "tm.ask"
  // The connect act carries BOTH mail and LinkedIn on one surface: mail is
  // the required gate (it is what CONNECT_DONE waits on), LinkedIn is the
  // profile saved beside it (linkedinStatus tracks its own resolution
  // independently and never gates the act's finish).
  | "cn.consent"
  // The preferences act: asked once, then the one terminal every path shares.
  // The team act reaches it without ever entering connect.
  | "pf.ask"
  | "pf.done";

// Exactly one label source — a blank button is unrepresentable.
type LabelSource =
  | { labelKey: MessageKey; label?: never }
  | { label: string; labelKey?: never };

// A thread entry's `params` are STRINGS, not numbers, and that is the carrier's
// job rather than a narrowing for tidiness: a quantity handed across as a raw
// number reaches the catalog sentence through string coercion, which groups for
// nobody — so the producer has to have decided, before it packs the entry, that
// the figure is a magnitude the reader groups (`formatNumber`) or a position
// nobody groups (`String`). A number here lets that decision be skipped
// silently, and the reader is the one who finds out.

export type QuestionOption = {
  value: string;
  /**
   * The exact string this answer puts on the record, when it puts one there.
   *
   * SEPARATE FROM `value`, which is only what the answer is called on the wire.
   * The two happen to agree for a clarify, where the server says the value is
   * verbatim what the selection authorizes; they do not for a question that
   * chooses something other than a profile field, and a surface that printed
   * `value` as "this is what gets written" would be inventing a consequence for
   * those. Absent means this option writes nothing a screen can quote, which is
   * a fact worth stating and never one to guess at.
   */
  writes?: string;
  detailKey?: MessageKey;
  params?: Record<string, string>;
} & LabelSource;

export type ConversationQuestion = {
  id: string;
  i18nKey: MessageKey;
  params?: Record<string, string>;
  options: QuestionOption[];
  /** The subordinate local-dismiss action's label (humans outrank the
   * reader: a clarify is never an unanswerable gate). Absent on questions
   * that genuinely need an answer to proceed (the speaker ask). */
  dismissLabelKey?: MessageKey;
};

export type OutcomeTone = "success" | "deferred" | "failure";

// Entries enter the reducer with a semantic id (`read:ready`, `stage:extract`);
// withEntries stamps each appended entry with the state's monotonic sequence
// (`17:read:ready`), so a retried URL or a rebuilt stage never collides with
// an earlier occurrence as a React key.
export type ThreadEntry =
  | {
      kind: "narration";
      id: string;
      i18nKey: MessageKey;
      params?: Record<string, string>;
      /** Params that are i18n keys themselves; the renderer translates them. */
      paramKeys?: Record<string, MessageKey>;
      findingIds?: string[];
    }
  | { kind: "question"; id: string; question: ConversationQuestion }
  // A user turn says something — catalog copy XOR literal text, never blank.
  | ({
      kind: "user";
      id: string;
      params?: Record<string, string>;
    } & (
      | { i18nKey: MessageKey; text?: never }
      | { text: string; i18nKey?: never }
    ))
  | {
      kind: "outcome";
      id: string;
      i18nKey: MessageKey;
      params?: Record<string, string>;
      tone: OutcomeTone;
    };

export type NarrationEntry = Extract<ThreadEntry, { kind: "narration" }>;

export type ConversationState = {
  act: ConversationAct;
  phase: ConversationPhase;
  memberPath: boolean;
  pendingQuestion: ConversationQuestion | null;
  thread: ThreadEntry[];
  /** Monotonic entry-id sequence; see ThreadEntry. */
  seq: number;
  /**
   * The read run whose events are current. Null between URL_SUBMITTED and
   * READ_STARTED and after a terminal is recorded — read events for ANY run
   * are stale in those windows.
   */
  activeReadId: string | null;
  /** A ready/partial terminal was recorded; answering the last clarify (or
   * REVIEW_READY) may proceed to review without waiting on a re-read. */
  readCompleted: boolean;
  /** The read that concluded readCompleted, retained past activeReadId's
   * null-out so a clarify promoted later, from co.review itself, can still
   * be checked against the run it actually belongs to — never just any run
   * that happens to have finished. Cleared whenever a run resumes with no
   * completion recorded (a failed/deferred terminal), alongside readCompleted. */
  concludedReadId: string | null;
  /** The build run whose events are current; stale runs are ignored. */
  activeBuildId: string | null;
  /** Last narrated build stage, so a repeated stage poll appends nothing. */
  lastBuildStage: BuildStage | null;
  /** How the last voice build ended: failed may be retried, deferred
   * resumes on its own and its later events re-enter vo.building. */
  lastBuildStatus: BuildTerminalStatus | null;
  /** LinkedIn's own resolution on the connect screen, independent of mail:
   * "pending" admits LINKEDIN_SAVED/LINKEDIN_SKIPPED exactly once; either
   * resolves it, and neither ever gates CONNECT_DONE. */
  linkedinStatus: "pending" | "saved" | "skipped";
};

export type ReadTerminalStatus = "ready" | "partial" | "failed" | "deferred";
export type BuildStage = "snapshot" | "extract" | "evaluate" | "activate";
export type BuildTerminalStatus = "succeeded" | "failed" | "deferred";

/**
 * Where a restored session may land after RESUME. Only phases that are
 * stable waiting points qualify: transient phases (reading, building) cannot
 * be reconstructed from wizard state and restart from their act's entry.
 */
export type ResumePoint = Extract<
  ConversationPhase,
  "bs.ask" | "in.ask" | "tm.ask" | "vo.collecting" | "vo.skipped" | "cn.consent"
>;

export type ConversationEvent =
  // LinkedIn's own two resolutions on the connect screen: the member's
  // profile is saved, or they chose not to give it now. Skipping is a
  // first-class outcome, not a failure: said once, it is never asked again in
  // this journey — and unlike mail, saying so never blocks finishing the act.
  | { type: "LINKEDIN_SAVED"; profile: string }
  | { type: "LINKEDIN_SKIPPED" }
  | {
      type: "START";
      memberPath: boolean;
      /** Server-derived recap turns seeded on restore. Narration is never
       * persisted; these entries are recomputed from server state, so a
       * reload summarizes instead of replaying the original narration. */
      recap?: readonly NarrationEntry[];
      /** Restore landing: the server already recorded a confirmed company,
       * so the conversation reopens in co.confirmed and RESUME routes on. */
      companyConfirmed?: boolean;
    }
  | { type: "URL_SUBMITTED"; url: string }
  | { type: "READ_STARTED"; readId: string }
  // Narration carries the id of the run that produced it: readId for
  // site-read events, buildId for build events, neither for run-agnostic
  // narration (corpus growth). Company phases DROP uncorrelated narration.
  | {
      type: "NARRATION";
      readId?: string;
      buildId?: string;
      entry: NarrationEntry;
    }
  | { type: "READ_TERMINAL"; readId: string; status: ReadTerminalStatus }
  | { type: "CLARIFY"; readId: string; question: ConversationQuestion }
  // dismissed: the human declined the question locally (nothing written,
  // nothing asked again); legal only for questions carrying a dismiss label.
  | {
      type: "QUESTION_ANSWERED";
      questionId: string;
      value: string;
      dismissed?: boolean;
    }
  | { type: "REVIEW_READY" }
  | { type: "MANUAL_CHOSEN" }
  | { type: "COMPANY_CONFIRMED" }
  // Restore routing out of co.confirmed. No target takes the same route the
  // live confirmation takes; a target fast-forwards to the stable point the
  // wizard state recorded. A member's journey has no creator acts to land in,
  // so their default — and any creator-only target — is the voice act.
  | { type: "RESUME"; target?: ResumePoint }
  // Leaving the basis act opens the invite.
  | { type: "BASIS_DONE" }
  // The two answers to the invite. Accepting opens the voice act; declining
  // opens the team act instead, because the steps left are all about the
  // person who just said they will not be here — so the one thing left to do
  // is name who will be.
  | { type: "INVITE_ACCEPTED" }
  | { type: "INVITE_DECLINED" }
  // Leaving the team act, with or without an invite sent, ends the journey.
  | { type: "TEAM_DONE" }
  | { type: "VOICE_SKIPPED" }
  | { type: "UPLOAD_ADDED"; id: string; name: string }
  | { type: "SPEAKER_NEEDED"; question: ConversationQuestion }
  | { type: "BUILD_STARTED"; buildId: string }
  | { type: "BUILD_STAGE"; buildId: string; stage: BuildStage }
  | { type: "BUILD_TERMINAL"; buildId: string; status: BuildTerminalStatus }
  // Leaving the voice act, built or skipped, lands straight on connect.
  | { type: "VOICE_DONE" }
  // A succeeded build the reader does not recognise as their own: back to
  // collecting, so more of their writing can go in before the next build.
  | { type: "VOICE_REVISE" }
  // Leaving the connect act and leaving the team act both land on the
  // preferences act; PREFS_DONE is the one terminal move.
  | { type: "CONNECT_DONE" }
  | { type: "PREFS_DONE" };

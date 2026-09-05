import type { MessageKey } from "../../i18n/en";
import { isLegal } from "./conversation-legality";
import type {
  BuildStage,
  BuildTerminalStatus,
  ConversationAct,
  ConversationEvent,
  ConversationPhase,
  ConversationState,
  OutcomeTone,
  ResumePoint,
  ThreadEntry,
} from "./conversation-types";

// The onboarding conversation as a pure reducer. Every legal move lives in
// the transition table in conversation-legality.ts; an event that is not
// legal in the current phase returns the state unchanged, so a stale poll or
// a double click can never corrupt the conversation. React effects hold no
// hidden state: the thread, the pending question, and the act/phase pair ARE
// the conversation. The vocabulary (acts, phases, entries, events) lives in
// conversation-types.ts and is re-exported here so callers have one import.

export type {
  BuildStage,
  BuildTerminalStatus,
  ConversationAct,
  ConversationEvent,
  ConversationPhase,
  ConversationQuestion,
  ConversationState,
  NarrationEntry,
  OutcomeTone,
  QuestionOption,
  ReadTerminalStatus,
  ResumePoint,
  ThreadEntry,
} from "./conversation-types";

export const initialConversationState: ConversationState = {
  act: "welcome",
  phase: "co.intro",
  memberPath: false,
  pendingQuestion: null,
  thread: [],
  seq: 0,
  activeReadId: null,
  readCompleted: false,
  concludedReadId: null,
  activeBuildId: null,
  lastBuildStage: null,
  lastBuildStatus: null,
  linkedinStatus: "pending",
};

// Only the two broken statuses speak at all: a ready or partial terminal is
// silent — the review it leads straight into already shows it is ready, and
// the coverage detail (what was skipped and why) lives there, not here.
const readTerminalKeys: Record<"failed" | "deferred", MessageKey> = {
  failed: "ob.conv.read.failed",
  deferred: "ob.conv.read.deferred",
};

const buildStageKeys: Record<BuildStage, MessageKey> = {
  snapshot: "ob.conv.build.snapshot",
  extract: "ob.conv.build.extract",
  evaluate: "ob.conv.build.evaluate",
  activate: "ob.conv.build.activate",
};

const buildTerminalKeys: Record<BuildTerminalStatus, MessageKey> = {
  succeeded: "ob.conv.build.succeeded",
  failed: "ob.conv.build.failed",
  deferred: "ob.conv.build.deferred",
};

const buildTerminalTones: Record<BuildTerminalStatus, OutcomeTone> = {
  succeeded: "success",
  failed: "failure",
  deferred: "deferred",
};

// Which act owns each restorable landing point; RESUME derives the act from
// the phase so the pair can never disagree.
const resumeActs: Record<ResumePoint, ConversationAct> = {
  "bs.ask": "basis",
  "in.ask": "invite",
  "tm.ask": "team",
  "vo.collecting": "voice",
  "vo.skipped": "voice",
  "cn.consent": "connect",
};

export const THREAD_CAP = 200;

// The thread is a working transcript, not an archive. Past the cap the oldest
// narration goes first: questions, answers, and outcomes carry decisions and
// must outlive ambient progress chatter.
function appendEntries(
  thread: readonly ThreadEntry[],
  entries: readonly ThreadEntry[],
): ThreadEntry[] {
  const next = [...thread, ...entries];
  while (next.length > THREAD_CAP) {
    const oldestNarration = next.findIndex(
      (entry) => entry.kind === "narration",
    );
    next.splice(oldestNarration === -1 ? 0 : oldestNarration, 1);
  }
  return next;
}

function withEntries(
  state: ConversationState,
  patch: Partial<Omit<ConversationState, "thread" | "seq">>,
  entries: readonly ThreadEntry[] = [],
): ConversationState {
  if (entries.length === 0) {
    return { ...state, ...patch };
  }
  const stamped = entries.map((entry, offset) => ({
    ...entry,
    id: `${state.seq + offset}:${entry.id}`,
  }));
  return {
    ...state,
    ...patch,
    seq: state.seq + entries.length,
    thread: appendEntries(state.thread, stamped),
  };
}

export function conversationReducer(
  state: ConversationState,
  event: ConversationEvent,
): ConversationState {
  return isLegal(state, event) ? applyEvent(state, event) : state;
}

// Where an answered question lands: the speaker question back to collecting;
// a clarify to review when the read already finished (its completion must
// never be lost), otherwise back to the still-running read.
function answeredPhase(state: ConversationState): ConversationPhase {
  if (state.phase === "vo.speaker") return "vo.collecting";
  return state.readCompleted ? "co.review" : "co.reading";
}

function applyReadTerminal(
  state: ConversationState,
  event: Extract<ConversationEvent, { type: "READ_TERMINAL" }>,
): ConversationState {
  if (event.status === "ready" || event.status === "partial") {
    // The pending clarify question is never stranded: co.clarify holds until
    // it is answered. readCompleted records the completion so the final
    // answer (or a REVIEW_READY from co.reading) proceeds straight to
    // review; the concluded run's id retires so its late events are stale.
    // Neither terminal status appends a word — the review it leads straight
    // into already says "ready," and whatever it skipped along the way is
    // the review's own coverage detail to show, not the rail's to repeat.
    return withEntries(state, {
      activeReadId: null,
      readCompleted: true,
      concludedReadId: event.readId,
    });
  }
  // A failed or deferred read moots its clarify question and waits in
  // co.reading for a new URL or the manual path.
  return withEntries(
    state,
    {
      phase: "co.reading",
      pendingQuestion: null,
      activeReadId: null,
      readCompleted: false,
      concludedReadId: null,
    },
    [
      {
        kind: "outcome",
        id: `read:${event.status}`,
        i18nKey: readTerminalKeys[event.status],
        tone: event.status === "deferred" ? "deferred" : "failure",
      },
    ],
  );
}

// The landing points that exist only on the creator's route. A member
// resumed there would stand in an act whose every event is illegal for them,
// so the member path resolves these to the first act that is theirs.
const creatorOnlyResume = new Set<ResumePoint>(["bs.ask", "in.ask", "tm.ask"]);

// Restore normalization out of co.confirmed: the same routing the live
// confirmation takes, without repeating the confirmation outcome. A target
// fast-forwards to the stable point the wizard state recorded; without one, a
// creator opens the basis act the confirmation would have opened, and a
// member opens the voice act their journey begins with.
function applyResume(
  state: ConversationState,
  event: Extract<ConversationEvent, { type: "RESUME" }>,
): ConversationState {
  let phase: ResumePoint =
    event.target ?? (state.memberPath ? "vo.collecting" : "bs.ask");
  if (state.memberPath && creatorOnlyResume.has(phase)) {
    phase = "vo.collecting";
  }
  return withEntries(state, { act: resumeActs[phase], phase });
}

// The answered question leaves the thread as the user's own turn: the
// chosen option's label, or — for a dismissal — the dismiss action the human
// clicked (legality already required the pending question to carry it).
function applyAnswer(
  state: ConversationState,
  event: Extract<ConversationEvent, { type: "QUESTION_ANSWERED" }>,
): ConversationState {
  const option = state.pendingQuestion?.options.find(
    (candidate) => candidate.value === event.value,
  );
  const answerId = `answer:${event.questionId}`;
  const dismissed: ThreadEntry = {
    kind: "user",
    id: answerId,
    i18nKey:
      state.pendingQuestion?.dismissLabelKey ?? "ob.conv.clarify.dismiss",
  };
  const chosen: ThreadEntry =
    option?.labelKey !== undefined
      ? {
          kind: "user",
          id: answerId,
          i18nKey: option.labelKey,
          params: option.params,
        }
      : {
          kind: "user",
          id: answerId,
          text: option?.label ?? event.value,
          params: option?.params,
        };
  return withEntries(
    state,
    { phase: answeredPhase(state), pendingQuestion: null },
    [event.dismissed === true ? dismissed : chosen],
  );
}

// Legality is already settled: every branch below only computes the next
// state for an event the table admitted in the current phase.
function applyEvent(
  state: ConversationState,
  event: ConversationEvent,
): ConversationState {
  switch (event.type) {
    case "START":
      // A restore seeds the thread with server-derived recap turns and, when
      // the company is already confirmed, opens in co.confirmed so RESUME
      // can route onward without replaying the confirmation outcome.
      return withEntries(
        state,
        {
          act: "company",
          phase: event.companyConfirmed === true ? "co.confirmed" : "co.intro",
          memberPath: event.memberPath,
        },
        event.recap ?? [],
      );
    case "URL_SUBMITTED":
      // Until READ_STARTED names the new run, read events for ANY run are
      // stale — the previous run is retired here, not at READ_STARTED.
      return withEntries(state, { activeReadId: null, readCompleted: false }, [
        { kind: "user", id: `url:${event.url}`, text: event.url },
      ]);
    case "READ_STARTED":
      return withEntries(state, {
        phase: "co.reading",
        activeReadId: event.readId,
        readCompleted: false,
      });
    case "NARRATION": {
      // A monotonic counter (pages read, corpus words) narrates under ONE
      // stable semantic id with the count in params: a fresh emission
      // REPLACES the earlier bubble in place — same position, same stamped
      // id (so React keys hold) — instead of stacking near-identical lines.
      const index = state.thread.findIndex(
        (entry) =>
          entry.kind === "narration" &&
          entry.id.slice(entry.id.indexOf(":") + 1) === event.entry.id,
      );
      if (index !== -1) {
        const thread = [...state.thread];
        thread[index] = { ...event.entry, id: state.thread[index].id };
        return { ...state, thread };
      }
      return withEntries(state, {}, [event.entry]);
    }
    case "READ_TERMINAL":
      return applyReadTerminal(state, event);
    case "CLARIFY":
      return withEntries(
        state,
        { phase: "co.clarify", pendingQuestion: event.question },
        [
          {
            kind: "question",
            id: `question:${event.question.id}`,
            question: event.question,
          },
        ],
      );
    case "QUESTION_ANSWERED":
      return applyAnswer(state, event);
    case "REVIEW_READY":
      return withEntries(state, { phase: "co.review" });
    case "MANUAL_CHOSEN":
      return withEntries(state, { phase: "co.manual", pendingQuestion: null }, [
        { kind: "user", id: "manual:chosen", i18nKey: "ob.conv.manual.chosen" },
      ]);
    case "COMPANY_CONFIRMED":
      return withEntries(state, { act: "basis", phase: "bs.ask" }, [
        {
          kind: "outcome",
          id: "company:confirmed",
          i18nKey: "ob.conv.company.confirmed",
          tone: "success",
        },
      ]);
    case "RESUME":
      return applyResume(state, event);
    case "BASIS_DONE":
      return withEntries(state, { act: "invite", phase: "in.ask" }, [
        {
          kind: "outcome",
          id: "basis:done",
          i18nKey: "ob.conv.basis.done",
          tone: "success",
        },
      ]);
    case "INVITE_ACCEPTED":
      return withEntries(state, { act: "voice", phase: "vo.collecting" }, [
        {
          kind: "user",
          id: "invite:accepted",
          i18nKey: "ob.conv.invite.accepted",
        },
      ]);
    case "INVITE_DECLINED":
      return withEntries(state, { act: "team", phase: "tm.ask" }, [
        {
          kind: "user",
          id: "invite:declined",
          i18nKey: "ob.conv.invite.declined",
        },
      ]);
    case "TEAM_DONE":
      return withEntries(state, { act: "prefs", phase: "pf.ask" }, [
        {
          kind: "outcome",
          id: "team:done",
          i18nKey: "ob.conv.team.done",
          tone: "success",
        },
      ]);
    case "VOICE_SKIPPED":
      return withEntries(
        state,
        { phase: "vo.skipped", pendingQuestion: null },
        [
          {
            kind: "user",
            id: "voice:skipped",
            i18nKey: "ob.conv.voice.skipped",
          },
        ],
      );
    case "UPLOAD_ADDED":
      return withEntries(state, {}, [
        {
          kind: "user",
          id: `upload:${event.id}`,
          i18nKey: "ob.conv.voice.uploadAdded",
          params: { name: event.name },
        },
      ]);
    case "SPEAKER_NEEDED":
      return withEntries(
        state,
        { phase: "vo.speaker", pendingQuestion: event.question },
        [
          {
            kind: "question",
            id: `question:${event.question.id}`,
            question: event.question,
          },
        ],
      );
    case "BUILD_STARTED":
      return withEntries(state, {
        phase: "vo.building",
        activeBuildId: event.buildId,
        lastBuildStage: null,
        lastBuildStatus: null,
      });
    case "BUILD_STAGE":
      // A stage from vo.result means a deferred build resumed on its own:
      // re-enter vo.building and clear the deferred status.
      return withEntries(
        state,
        {
          phase: "vo.building",
          lastBuildStage: event.stage,
          lastBuildStatus: null,
        },
        [
          {
            kind: "narration",
            id: `stage:${event.stage}`,
            i18nKey: buildStageKeys[event.stage],
          },
        ],
      );
    case "BUILD_TERMINAL":
      return withEntries(
        state,
        { phase: "vo.result", lastBuildStatus: event.status },
        [
          {
            kind: "outcome",
            id: `build:${event.status}`,
            i18nKey: buildTerminalKeys[event.status],
            tone: buildTerminalTones[event.status],
          },
        ],
      );
    case "VOICE_DONE":
      return withEntries(state, { act: "connect", phase: "cn.consent" });
    case "VOICE_REVISE":
      // The corpus is kept; only the verdict on the build is withdrawn, so
      // the next build starts from everything already in hand plus what the
      // reader adds now.
      return withEntries(state, { phase: "vo.collecting" }, [
        {
          kind: "user",
          id: "voice:revise",
          i18nKey: "ob.conv.voice.revise",
        },
      ]);
    // Neither resolution moves the act: both sections of the connect screen
    // stay on the same phase, and only mail's own consent gates CONNECT_DONE.
    case "LINKEDIN_SAVED":
      return withEntries(state, { linkedinStatus: "saved" }, [
        {
          kind: "outcome",
          id: "linkedin:saved",
          i18nKey: "ob.conv.linkedin.saved",
          tone: "success",
        },
      ]);
    case "LINKEDIN_SKIPPED":
      return withEntries(state, { linkedinStatus: "skipped" }, [
        {
          kind: "outcome",
          id: "linkedin:skipped",
          i18nKey: "ob.conv.linkedin.skipped",
          tone: "deferred",
        },
      ]);
    case "CONNECT_DONE":
      return withEntries(state, { act: "prefs", phase: "pf.ask" });
    case "PREFS_DONE":
      return withEntries(state, { act: "done", phase: "pf.done" }, [
        {
          kind: "outcome",
          id: "prefs:done",
          i18nKey: "ob.conv.done",
          tone: "success",
        },
      ]);
  }
}

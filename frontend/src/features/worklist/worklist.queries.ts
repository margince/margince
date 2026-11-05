import type { components } from "../../api/schema";

export {
  loadedQueue,
  useHandledForYou,
  useTeamBoard,
  useTeamExceptions,
  useWorklist,
  worklistKey,
} from "./worklist.reads";
export {
  subjectAcceptsAnOwner,
  useApproval,
  useClearDisposition,
  useCoachTeammate,
  useNudgeDismissal,
  usePinRow,
  useReassignTask,
  useRefreshWalk,
  useSetDisposition,
  useTakeOwnership,
} from "./worklist.writes";

export type Worklist = components["schemas"]["Worklist"];

export type WorklistItem = components["schemas"]["WorklistItem"];

export type WorklistReason = components["schemas"]["WorklistReason"];

export type WorklistComparison = components["schemas"]["WorklistComparison"];

export type WorklistValue = components["schemas"]["WorklistValue"];

// Derived from the contract's own enums rather than spelled again here: a
// hand-written union goes stale the moment the server adds a value, and it goes
// stale silently — nothing compares the two.
export type WorklistScope = Worklist["scope"];

/**
 * The one scope word an ADDRESS carries: `#/worklist/unassigned`.
 *
 * Here rather than on the screen, so Brief can name the same pile without
 * importing the queue, and typed as WorklistScope so a scope renamed in
 * crm.yaml fails to compile instead of leaving an address that silently stops
 * opening what it names.
 */
export const UNASSIGNED: WorklistScope = "unassigned";

export type WorklistFilter = NonNullable<Worklist["filter"]>;

export type WorklistCategory = WorklistItem["category"];

// The ways a row can be put down, derived from the item rather than spelled
// again: the contract declares them inline, and a hand-written union would go
// stale the moment the server gained a fourth — silently, because nothing
// compares the two.
export type WorklistDisposition = NonNullable<
  WorklistItem["dispositions"]
>[number];

// What a snooze waits for. Derived from the contract's shared schema for the
// reason the disposition union is: a hand-written union goes stale the moment
// the server gains a fourth condition, and nothing compares the two.
export type ReopenCondition = components["schemas"]["ReopenCondition"];

export type TeamBoard = components["schemas"]["TeamBoard"];

export type TeamExceptions = components["schemas"]["TeamExceptions"];

export type WorklistWalk = components["schemas"]["WorklistWalk"];

export type { HandledForYou, Receipt } from "../../lib/handled";

export type TeamException = components["schemas"]["TeamException"];

export type TeamBoardMember = components["schemas"]["TeamBoardMember"];

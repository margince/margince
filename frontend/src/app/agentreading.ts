import type { MarginceCoreState } from "../design-system/margince-core";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { AgentEdgeRegister } from "./agent-edge-signal";
import type { AgentFault } from "./agent-fault";
import type { AiActivityItem, Signals } from "./agentrail-reads";
import type { RailWords } from "./agentrail-resting";
import type { AiActivity } from "./ai-activity";
import { laneFor } from "./ai-activity-orb";
import { plain, type SpokenLine, speak } from "./ai-activity-speak";

/**
 * The state the section shows when nobody has overridden it.
 *
 * It has one subject: the agent. Deriving `ingest` from a query in flight and
 * `working` from any mutation would report the reader's own clicks in the
 * agent's vocabulary. The orb would go to `ingest` because a list was loading.
 * What the tool is doing has its own quieter line under this one.
 *
 * Three observers watch the same work, and they rank. The server's projection
 * knows what a run is, and only it may name one. Next is this tab's count of
 * requests held open to a model route (`asking`, api/model-inflight.ts). It
 * covers the gap between a contact pressing "Draft with AI" and the next poll.
 * Without it the orb would rest through the work it exists to show. It ranks
 * below every occurrence in the feed. It answers `working`, because a request
 * in flight cannot say which half of the lifecycle it is in.
 * A mailbox import is the third, and the feed cannot replace it. It reaches the
 * feed only as settled classifications, never as work in flight. So the
 * import's status row (capture-progress.ts) reports it, as `ingest`. It ranks
 * above `asking` because it knows its half.
 *
 * The order is severity. It starts with the faults that stop the agent running
 * at all: an agent with no model bound has no runs to break. Under those comes
 * a run that broke, then one that stalled. Then the agent's own live work, and
 * at the bottom, rest.
 *
 * The licence is not in the order. The line answers what the agent is doing
 * now. A licence posture is a standing condition of the installation, still
 * true in an hour. On the live line it would hide every run behind it. It
 * has its own persistent chrome (`LicenseBanner`).
 *
 * It answers the cause alongside the state, and the two travel together. The
 * sentence the block carries belongs to the cause. A caller that re-derived the
 * cause could caption a colour with a run that did not produce it. An unread
 * failure over a second run still in flight must read as the failure.
 */
type Reading = Readonly<{
  state: MarginceCoreState;
  /** The occurrence the state is about, or null when no occurrence caused it. */
  cause: AiActivityItem | null;
  /**
   * Which register the margins light in. It is the import's only when the
   * import is the only work in flight. A named run or a call this tab holds
   * open is the agent's own work, whatever the orb shows, and the margin says so.
   */
  register: AgentEdgeRegister;
}>;

export function derive(
  signals: Signals,
  server: AiActivity,
  fault: AgentFault | null,
): Reading {
  if (signals.ai === "unconfigured" || signals.offline.length > 0) {
    return { state: "error", cause: null, register: "agent" };
  }
  // A run that broke, and that this reader has not been shown yet. It clears by
  // being read rather than by being repaired.
  if (fault !== null) {
    return { state: fault.severity, cause: fault.item, register: "agent" };
  }
  // A live run past the lease its own source declared. The server derives it,
  // so a worker that died silently does not stay displayed as busy. Amber fits
  // because the work may yet land. Some kinds have a way out: opening the
  // account or the document again re-arms the read. That kind's own line says
  // so, because a warning with no next step is only a worry.
  const stalled = server.running.find((item) => item.state === "stalled");
  if (stalled) {
    return { state: "warning", cause: stalled, register: "agent" };
  }
  // The agent's own live work. Its half of the lifecycle comes from the kind
  // of work, not from its progress (ai-activity-orb.ts). Evidence arriving is
  // `ingest`, and reasoning over evidence already held is `working`.
  // A queued occurrence counts as live: the agent has taken the work up, and
  // the reader has no use for the difference. The line under the orb carries
  // that detail, in copy the feed already has for every state.
  const live = server.running.find((item) => item.state !== "stalled");
  if (live) {
    return { state: laneFor(live), cause: live, register: "agent" };
  }
  // Mail being imported. No cause travels with it, since the feed cannot name
  // it as an occurrence. So the line is the import's own, from the signals.
  // The margins take the import's register unless this tab has a call open.
  // A model call in flight is the agent's own work, and the margin lights for
  // that.
  if (signals.capture !== null) {
    return {
      state: "ingest",
      cause: null,
      register: server.asking ? "agent" : "capture",
    };
  }
  // Work the feed cannot name: this tab's own ask, before the feed catches up,
  // or live work of a kind the rail does not narrate. It answers `working`
  // rather than a lane read from the kind, because the kind is unknown here.
  // No cause travels with either. So the line falls back to the generic word
  // until the feed carries an occurrence it can name. It never borrows a
  // sentence about some other run.
  if (server.asking || server.unnamed) {
    return { state: "working", cause: null, register: "agent" };
  }
  // A request that failed a moment ago does not colour the orb. Otherwise one
  // dropped request on a flaky connection would flash every screen's corner
  // red, green and red again. Nobody reads a light that does that. What the orb reports is standing state; the screen that made
  // the request reports the request.
  return { state: "idle", cause: null, register: "agent" };
}

/**
 * What that occurrence is called, in the reader's words.
 *
 * A kind the copy map does not narrate answers with the plainest true thing,
 * but only where the state is a fault. An unnamed failure still has to say a
 * failure happened. An unnamed run in flight can fall through to the state's
 * own generic word without losing anything.
 */
export function causeLine(
  cause: AiActivityItem | null,
  t: (key: MessageKey) => string,
): SpokenLine | null {
  if (cause === null) {
    return null;
  }
  const said = speak(cause, t);
  if (said !== null) {
    return said;
  }
  if (cause.state === "failed") {
    return plain(t("agent.line.runFailed"));
  }
  return cause.state === "degraded" || cause.state === "stalled"
    ? plain(t("agent.line.runStopped"))
    : null;
}

/**
 * The one line the block carries, for whichever state is showing.
 *
 * `agentLine` is the sentence of the occurrence that put the orb in this state,
 * in the reader's locale. That is the run that broke, the one past its lease,
 * or the one running now. It leads wherever it exists: a state is a colour, and
 * a named run is an answer. It is null for a state no occurrence caused (an
 * unbound model, an unreachable source). It is also null for a kind this build
 * writes no sentence for.
 */
export function barLine(
  state: MarginceCoreState,
  signals: Signals,
  devLine: string,
  agentLine: SpokenLine | null,
  said: RailWords,
): SpokenLine {
  if (state === "error" || state === "warning") {
    return faultLine(state, signals, agentLine, said.t);
  }
  if (state === "working") {
    // The named run outranks the generic word. "Working" is true of an
    // overnight brief and of a one-line summary, and only one of them is news.
    return agentLine ?? plain(said.t("agent.state.working"));
  }
  if (state === "ingest") {
    return (
      agentLine ?? plain(signals.capture?.line ?? said.t("agent.state.ingest"))
    );
  }
  if (signals.waiting !== undefined && signals.waiting > 0) {
    return plain(said.waiting(signals.waiting));
  }
  // A deployment on the development path answers, but every answer it gives is
  // invented. A reader who does not know that is misled by a product that looks
  // like it works. It is a standing fact, so the resting line states it calmly
  // instead of raising a fault. It uses the words the sign-in screen uses.
  if (signals.ai === "development") {
    return plain(devLine);
  }
  return plain(said.t("agent.state.idle"));
}

/**
 * The line for a fault, red or amber.
 *
 * It is its own function because the two colours rank their causes
 * differently, and the ranking is the whole content. No model bound and a
 * source that stopped answering are different repairs. Both outrank a run that
 * broke, because an agent that cannot run at all has no runs. Amber always has
 * an occurrence behind it, a broken run or a stalled one. causeLine gives every
 * kind a sentence, so amber's fallback is the red branch's last resort.
 */
function faultLine(
  state: "error" | "warning",
  signals: Signals,
  agentLine: SpokenLine | null,
  t: Translator,
): SpokenLine {
  if (state === "warning") {
    return agentLine ?? plain(t("agent.line.runStopped"));
  }
  if (signals.ai === "unconfigured") {
    return plain(t("agent.fact.noModel"));
  }
  if (signals.offline.length > 0) {
    return plain(
      t("agent.line.cannotReach", {
        sources: signals.offline.map((source) => source.label).join(", "),
      }),
    );
  }
  return agentLine ?? plain(t("agent.line.runFailed"));
}

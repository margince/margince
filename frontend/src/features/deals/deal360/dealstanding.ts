// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The readings How it stands draws for a deal, worked out from rows the deal
// page already holds. Nothing here formats a word: each reading says what it
// found, and dealhowitstands.tsx says it in the reader's language.

import type { components } from "../../../api/schema";
import type { ReadingTone } from "../../../design-system/readingtone";
import { calendarDaysBetween } from "../../../format/format";
import { messageFromOutside } from "../../../lib/messagevoice";
import { ghostCount } from "./dealcommittee";

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];
type Offer = components["schemas"]["Offer"];
type Activity = components["schemas"]["Activity"];
type DealCoverage = components["schemas"]["DealCoverage"];
type DealStatusCard = components["schemas"]["DealStatusCard"];
type FieldHistoryEntry = components["schemas"]["FieldHistoryEntry"];

/** The ladder's write path, owned by the screen that holds the close dialog. */
export type DealLadderControl = Readonly<{
  advancing: boolean;
  advanceRefused: boolean;
  refusedReasonId?: string;
  onAdvance: (toStage: Stage) => void;
}>;

/** What the deal page already holds that the card reads from. */
export type DealStandingInputs = Readonly<{
  deal: Deal;
  // Sorted by position.
  stages: readonly Stage[];
  // The instant the page was read at: where today stands on the axis.
  asOf: string;
  activities: readonly Activity[];
  offers: readonly Offer[] | undefined;
  coverage: Readonly<{
    coverage?: DealCoverage;
    withheld: boolean;
    pending: boolean;
  }>;
  card?: DealStatusCard;
  ladder: DealLadderControl;
}>;

/**
 * The tone of the model's standing. A word this build does not know is
 * neutral, never the all-clear.
 */
export function dealVerdictTone(standing: string | undefined): ReadingTone {
  switch (standing) {
    case "live":
      return "success";
    case "drifting":
      return "warning";
    case "blocked":
    case "cold":
      return "danger";
    default:
      return "neutral";
  }
}

/**
 * How many records the verdict's reasoning cites, each counted once however
 * many of its sentences cite it.
 */
export function verdictSourceCount(card: DealStatusCard): number {
  const cited = new Set<string>();
  for (const sentence of card.verdict?.because.sentences ?? []) {
    for (const evidence of sentence.evidence) {
      cited.add(`${evidence.entity_type}:${evidence.entity_id}`);
    }
  }
  return cited.size;
}

export type Momentum =
  | Readonly<{ state: "stalled"; since?: string }>
  | Readonly<{ state: "on_hold"; until: string }>
  | Readonly<{ state: "active"; last: string }>
  | Readonly<{ state: "quiet" }>;

/**
 * Whether the deal is moving. `stalled` is the server's flag. The server
 * already lifts it while the buyer asked us to wait. So a wait date reads as
 * on hold only on a deal that is not stalled.
 */
export function momentumOf(
  deal: Pick<Deal, "stalled" | "wait_until" | "last_activity_at">,
): Momentum {
  if (deal.stalled) {
    return deal.last_activity_at
      ? { state: "stalled", since: deal.last_activity_at }
      : { state: "stalled" };
  }
  if (deal.wait_until) {
    return { state: "on_hold", until: deal.wait_until };
  }
  if (deal.last_activity_at) {
    return { state: "active", last: deal.last_activity_at };
  }
  return { state: "quiet" };
}

export function momentumTone(momentum: Momentum): ReadingTone {
  switch (momentum.state) {
    case "stalled":
      return "danger";
    case "active":
      return "success";
    default:
      return "neutral";
  }
}

export type Committee =
  | Readonly<{ state: "unknown" }>
  | Readonly<{ state: "read"; named: number; engaged: number; gaps: number }>;

/**
 * Who is named on the buying side, and how many seats the coverage rules say
 * are missing. There is no fixed list of roles to count against, so the
 * reading never says "of N". Withheld, pending and failed all read as
 * unknown: none of them says who is on the deal.
 */
export function committeeOf(
  coverage: DealCoverage | undefined,
  withheld: boolean,
): Committee {
  // A reply without its seats is not a reading of who is on the deal.
  if (!coverage?.stakeholders || withheld) {
    return { state: "unknown" };
  }
  return {
    state: "read",
    named: coverage.stakeholders.length,
    engaged: coverage.stakeholders.filter((seat) => seat.engaged).length,
    gaps: ghostCount(coverage.risks ?? []),
  };
}

// One seat or none is a deal that runs through one seat, which is the risk
// the committee reading exists to show.
export function committeeTone(committee: Committee): ReadingTone {
  if (committee.state === "unknown") {
    return "neutral";
  }
  if (committee.named <= 1) {
    return "danger";
  }
  return committee.gaps > 0 ? "warning" : "success";
}

/**
 * How far the close date can be believed. A machine-moved date and a date in
 * the past on an open deal both ask for a look. A closed deal's date is when
 * it closed.
 */
export function closeTone(
  deal: Pick<Deal, "expected_close_date" | "close_date_provisional" | "status">,
  daysLeft: number | undefined,
): ReadingTone {
  if (!deal.expected_close_date) {
    return "neutral";
  }
  if (deal.status !== "open") {
    return "success";
  }
  if (daysLeft !== undefined && daysLeft < 0) {
    return "danger";
  }
  return deal.close_date_provisional ? "warning" : "success";
}

/**
 * When the deal entered each stage, from the advance rows of its field
 * history. A stage visited twice keeps its latest entry, because the time
 * spent there now runs from the last arrival. When the history is whole but
 * has no creation row (an import), the stage the first advance left is dated
 * by the deal's creation. A cut history dates nothing it cannot see.
 */
export function stageEntries(
  history: readonly FieldHistoryEntry[],
  deal: Pick<Deal, "created_at">,
  complete: boolean,
): ReadonlyMap<string, string> {
  const entries = new Map<string, string>();
  const rows = history
    .filter((row) => row.field === "stage_id" && row.new_value)
    .sort((a, b) => Date.parse(a.changed_at) - Date.parse(b.changed_at));
  const first = rows[0];
  if (complete && first?.old_value) {
    entries.set(first.old_value, deal.created_at);
  }
  for (const row of rows) {
    if (row.new_value) {
      entries.set(row.new_value, row.changed_at);
    }
  }
  return entries;
}

export type StageTime = Readonly<{ days: number; current: boolean }>;

/**
 * Whole days spent in each open stage the deal has passed or stands in. They run from
 * its entry to the next stage's entry, and for the current one to `asOf`. A
 * stage with no known entry gets no reading rather than a guess.
 */
export function stageTimes({
  stages,
  currentId,
  entries,
  asOf,
  zone,
}: Readonly<{
  stages: readonly Pick<Stage, "id">[];
  currentId: string | null;
  entries: ReadonlyMap<string, string>;
  asOf: string;
  zone: string;
}>): ReadonlyMap<string, StageTime> {
  const times = new Map<string, StageTime>();
  const dated = stages
    .map((stage) => ({ id: stage.id, at: entries.get(stage.id) }))
    .filter((stage): stage is { id: string; at: string } => Boolean(stage.at))
    .sort((a, b) => Date.parse(a.at) - Date.parse(b.at));
  dated.forEach((stage, index) => {
    const current = stage.id === currentId;
    const next = dated[index + 1];
    const end = current || !next ? asOf : next.at;
    if (!current && !next) {
      return;
    }
    times.set(stage.id, {
      days: calendarDaysBetween(new Date(stage.at), new Date(end), zone),
      current,
    });
  });
  return times;
}

/**
 * The offer that stands: the newest revision not replaced by a later one.
 * Drafts count, because a draft is what the deal is about to say.
 */
export function standingOffer(
  offers: readonly Offer[] | undefined,
): Offer | undefined {
  let best: Offer | undefined;
  for (const offer of offers ?? []) {
    if (offer.status === "superseded" || offer.archived_at) {
      continue;
    }
    if (!best || offer.revision > best.revision) {
      best = offer;
    }
  }
  return best;
}

export type NextStep = Readonly<{
  kind: "meeting" | "task";
  at: string;
  subject?: string;
}>;

/**
 * The next dated thing on the deal: a booked meeting ahead of `asOf`, else
 * the open task due soonest. Read off the loaded page of the timeline, so a
 * step on an older page is not found. The figure then says none is on hand
 * rather than inventing one.
 */
export function nextStepOf(
  activities: readonly Activity[],
  asOf: string,
): NextStep | undefined {
  const now = Date.parse(asOf);
  const meeting = soonest(
    activities.filter(
      (row) =>
        row.kind === "meeting" &&
        row.meeting_status === "booked" &&
        Date.parse(row.occurred_at) >= now,
    ),
    (row) => row.occurred_at,
  );
  if (meeting) {
    return stepOf("meeting", meeting.occurred_at, meeting);
  }
  const task = soonest(
    activities.filter((row) => row.kind === "task" && !row.is_done),
    (row) => row.due_at ?? undefined,
  );
  return task?.due_at ? stepOf("task", task.due_at, task) : undefined;
}

function stepOf(kind: NextStep["kind"], at: string, row: Activity): NextStep {
  return row.subject ? { kind, at, subject: row.subject } : { kind, at };
}

function soonest(
  rows: readonly Activity[],
  dateOf: (row: Activity) => string | undefined,
): Activity | undefined {
  let best: Activity | undefined;
  for (const row of rows) {
    const at = dateOf(row);
    if (!at) {
      continue;
    }
    const bestAt = best ? dateOf(best) : undefined;
    if (!bestAt || Date.parse(at) < Date.parse(bestAt)) {
      best = row;
    }
  }
  return best;
}

/**
 * The newest message the other side wrote, from the loaded page of the
 * timeline. Read from the recorded author, not the direction: a colleague's
 * mail arrives inbound too. A row no human wrote (automatic_kind) is no word.
 */
export function lastInbound(
  activities: readonly Activity[],
): Activity | undefined {
  let latest: Activity | undefined;
  for (const row of activities) {
    if (row.automatic_kind || !messageFromOutside(row)) {
      continue;
    }
    if (
      !latest ||
      Date.parse(row.occurred_at) > Date.parse(latest.occurred_at)
    ) {
      latest = row;
    }
  }
  return latest;
}

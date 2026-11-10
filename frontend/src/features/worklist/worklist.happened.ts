// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What happened behind a row, and what waiting costs, in sentences.
//
// The row's ask says what to do and its reasons say why it ranks. Neither says
// what took place: who wrote when, which run stopped, when the task fell due.
// These lines say it from facts the row already carries, and a line whose
// facts are missing is left out rather than said without them.

import { activityDay } from "../../format/activitywhen";
import { formatDayMonthNearYear } from "../../format/format.dates";
import { useNow } from "../../format/now";
import { type Locale, useLocale } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useCompanyZone } from "../../lib/companyzone";
import { valueFor } from "../../lib/labelfor";
import { useViewerZone } from "../../lib/viewerzone";
import { dealFactsText, partyName, type T } from "../../lib/worklistcopy";
import { askKey } from "./worklist.ask";
import type { WorklistItem } from "./worklist.queries";

type Clock = Readonly<{
  locale: Locale;
  // The reader's zone, for moments they lived through.
  zone: string;
  // The record's zone, for a deadline the team agreed on.
  recordZone: string;
  now: Date;
}>;

/** The clock these sentences are dated by, for the reader on this page. */
export function useHappenedClock(): Clock {
  const { locale } = useLocale();
  const zone = useViewerZone();
  const recordZone = useCompanyZone();
  // Only the year is read off it, so the clock never ticks.
  const now = useNow(0);
  return { locale, zone, recordZone, now: new Date(now) };
}

// The sentence for each source that has one, and the facts it needs. A source
// that is absent here has nothing a sentence could add to its ask.
const EVENT: Readonly<
  Partial<
    Record<
      WorklistItem["source"],
      Readonly<{ named: MessageKey; bare?: MessageKey }>
    >
  >
> = {
  customer_waiting: {
    named: "worklist.happened.customer_waiting",
    bare: "worklist.happened.customer_waiting.bare",
  },
  awaiting_reply: {
    named: "worklist.happened.awaiting_reply",
    bare: "worklist.happened.awaiting_reply.bare",
  },
  lead_response: {
    named: "worklist.happened.lead_response",
    bare: "worklist.happened.lead_response.bare",
  },
  meeting_outcome: {
    named: "worklist.happened.meeting_outcome",
    bare: "worklist.happened.meeting_outcome.bare",
  },
  meeting_follow_up: {
    named: "worklist.happened.meeting_follow_up",
    bare: "worklist.happened.meeting_follow_up.bare",
  },
  relationship_decay: { named: "worklist.happened.relationship_decay" },
  conversation_claim: {
    named: "worklist.happened.conversation_claim",
    bare: "worklist.happened.conversation_claim.bare",
  },
  bounce: {
    named: "worklist.happened.bounce",
    bare: "worklist.happened.bounce.bare",
  },
  undelivered: {
    named: "worklist.happened.undelivered",
    bare: "worklist.happened.undelivered.bare",
  },
  automation_run: {
    named: "worklist.happened.automation_run",
    bare: "worklist.happened.automation_run.bare",
  },
  capture_health: {
    named: "worklist.happened.capture_health",
    bare: "worklist.happened.capture_health.bare",
  },
  ai_work_health: {
    named: "worklist.happened.ai_work_health",
    bare: "worklist.happened.ai_work_health.bare",
  },
  failed_approval: {
    named: "worklist.happened.failed_approval",
    bare: "worklist.happened.failed_approval.bare",
  },
  deal_suggestion: {
    named: "worklist.happened.deal_suggestion",
    bare: "worklist.happened.deal_suggestion.bare",
  },
  tag_suggestion: {
    named: "worklist.happened.tag_suggestion",
    bare: "worklist.happened.tag_suggestion.bare",
  },
  domain_question: {
    named: "worklist.happened.domain_question",
    bare: "worklist.happened.domain_question.bare",
  },
};

// Who a source's sentence names: a contact for the conversation lanes, the
// stopped thing for the health lanes, the record for a suggestion.
const NAMES_THE_CAUSE = new Set<WorklistItem["source"]>([
  "automation_run",
  "capture_health",
  "ai_work_health",
]);
const NAMES_THE_RECORD = new Set<WorklistItem["source"]>([
  "failed_approval",
  "deal_suggestion",
  "tag_suggestion",
  "domain_question",
]);

/**
 * The lines that say what happened. `told` is whether the row's own event was
 * among them, which then need not open what the row asks.
 */
export function happenedLines(
  item: WorklistItem,
  t: T,
  clock: Clock,
): Readonly<{ lines: string[]; told: boolean }> {
  const event = ownEvent(item, t, clock);
  const deal = dealLine(item, t, clock);
  const lines = [event, deal, ...touchLines(item, t, clock)].filter(
    (line): line is string => Boolean(line),
  );
  // A deal at risk has no event of its own; its figures are what happened.
  const dealTold = item.source === "deal_at_risk" && deal !== null;
  return { lines, told: event !== null || dealTold };
}

/**
 * A card's line: its own event, then what to do about it by name. Null for a
 * row with no event, which keeps its whole ask.
 */
export function cardLine(
  item: WorklistItem,
  t: T,
  clock: Clock,
): string | null {
  const event = ownEvent(item, t, clock);
  return event ? `${event} ${todoLine(item, t, true)}` : null;
}

function ownEvent(item: WorklistItem, t: T, clock: Clock): string | null {
  return (
    eventLine(item, t, clock) ??
    taskLine(item, t, clock) ??
    meetingLine(item, t, clock)
  );
}

// What to do, for a row whose event is already said. It is the ask without
// its first sentence, which repeats the event with no date or name.
const TODO: Readonly<
  Partial<
    Record<
      WorklistItem["source"],
      Readonly<{ bare: MessageKey; named?: MessageKey }>
    >
  >
> = {
  task: { bare: "worklist.todo.task", named: "worklist.todo.task.named" },
  deal_at_risk: {
    bare: "worklist.todo.deal_at_risk",
    named: "worklist.todo.deal_at_risk.named",
  },
  customer_waiting: {
    bare: "worklist.todo.customer_waiting",
    named: "worklist.todo.customer_waiting.named",
  },
  awaiting_reply: {
    bare: "worklist.todo.awaiting_reply",
    named: "worklist.todo.awaiting_reply.named",
  },
  lead_response: {
    bare: "worklist.todo.lead_response",
    named: "worklist.todo.lead_response.named",
  },
  meeting: {
    bare: "worklist.todo.meeting",
    named: "worklist.todo.meeting.named",
  },
  meeting_outcome: { bare: "worklist.todo.meeting_outcome" },
  meeting_follow_up: {
    bare: "worklist.todo.meeting_follow_up",
    named: "worklist.todo.meeting_follow_up.named",
  },
  relationship_decay: {
    bare: "worklist.todo.relationship_decay",
    named: "worklist.todo.relationship_decay.named",
  },
  conversation_claim: {
    bare: "worklist.todo.conversation_claim",
    named: "worklist.todo.conversation_claim.named",
  },
  bounce: { bare: "worklist.todo.bounce", named: "worklist.todo.bounce.named" },
  undelivered: {
    bare: "worklist.todo.undelivered",
    named: "worklist.todo.undelivered.named",
  },
  automation_run: {
    bare: "worklist.todo.automation_run",
    named: "worklist.todo.automation_run.named",
  },
  capture_health: {
    bare: "worklist.todo.capture_health",
    named: "worklist.todo.capture_health.named",
  },
  ai_work_health: { bare: "worklist.todo.ai_work_health" },
  failed_approval: { bare: "worklist.todo.failed_approval" },
  deal_suggestion: { bare: "worklist.todo.deal_suggestion" },
  tag_suggestion: { bare: "worklist.todo.tag_suggestion" },
  domain_question: { bare: "worklist.todo.domain_question" },
};

/**
 * What a row asks, by name where it knows one. A row whose event is told
 * drops the ask's first sentence; an invitation keeps its RSVP ask whole.
 */
export function todoLine(item: WorklistItem, t: T, told: boolean): string {
  const ask = askKey(item);
  const keys = Object.hasOwn(TODO, item.source) ? TODO[item.source] : undefined;
  if (!told || !keys || ask === "worklist.ask.invitation") return t(ask);
  const name = todoName(item);
  return keys.named && name ? t(keys.named, { name }) : t(keys.bare);
}

// Whom the ask names: the deal at risk, the stopped thing, else the contact.
function todoName(item: WorklistItem): string | undefined {
  if (item.source === "deal_at_risk")
    return item.subject?.type === "deal" ? item.subject.label : undefined;
  if (NAMES_THE_CAUSE.has(item.source)) return item.cause_label || undefined;
  return partyName(item);
}

/** The row's own event, for the lanes that have one apart from a date. */
export function eventLine(
  item: WorklistItem,
  t: T,
  clock: Clock,
): string | null {
  const keys = Object.hasOwn(EVENT, item.source)
    ? EVENT[item.source]
    : undefined;
  if (!keys || !item.occurred_at) return null;
  const when = activityDay(
    item.occurred_at,
    item.occurred_precision,
    clock.locale,
    clock.zone,
    (at, locale, zone) => formatDayMonthNearYear(at, locale, zone, clock.now),
  );
  const name = eventName(item);
  if (name) return t(keys.named, { name, when });
  return keys.bare ? t(keys.bare, { when }) : null;
}

function eventName(item: WorklistItem): string | undefined {
  if (NAMES_THE_CAUSE.has(item.source)) return item.cause_label || undefined;
  if (NAMES_THE_RECORD.has(item.source))
    return item.subject?.label ?? item.source_record?.label ?? undefined;
  return partyName(item);
}

// A task's own deadline, against today: when it fell due and that it is open.
function taskLine(item: WorklistItem, t: T, clock: Clock): string | null {
  if (item.source !== "task" && item.source !== "weekly_commitment")
    return null;
  const dueAt = deadlineOf(item);
  if (!dueAt) return t("worklist.happened.task.undated");
  const due = formatDayMonthNearYear(
    dueAt,
    clock.locale,
    clock.recordZone,
    clock.now,
  );
  return t(
    item.overdue ? "worklist.happened.task.overdue" : "worklist.happened.task",
    { due },
  );
}

// A row's `due_at` as a deadline. On a meeting it is the start, which is an
// activity time and is read through its precision instead.
function deadlineOf(item: WorklistItem): string | undefined {
  return item.source !== "meeting" ? item.due_at : undefined;
}

// An upcoming meeting, which has its start rather than a past moment.
function meetingLine(item: WorklistItem, t: T, clock: Clock): string | null {
  if (item.source !== "meeting" || !item.due_at) return null;
  const when = activityDay(
    item.due_at,
    item.occurred_precision,
    clock.locale,
    clock.zone,
    (at, locale, zone) => formatDayMonthNearYear(at, locale, zone, clock.now),
  );
  const name = partyName(item);
  return name
    ? t("worklist.happened.meeting", { name, when })
    : t("worklist.happened.meeting.bare", { when });
}

// The deal the row is about, with its own figures. Its name alone says
// nothing the title has not.
function dealLine(item: WorklistItem, t: T, clock: Clock): string | null {
  const deal = item.subject?.type === "deal" ? item.subject.label : undefined;
  const figures = dealFactsText(item, t, clock.locale, clock.recordZone);
  return deal && figures
    ? t("worklist.happened.deal", { deal, figures })
    : null;
}

// The last contact each way with whoever the row is with. A contact's moments
// count only what this reader can open, so its gap is "none you can open".
function touchLines(item: WorklistItem, t: T, clock: Clock): string[] {
  const held = item.contact?.touch
    ? { touch: item.contact.touch, name: item.contact.label, of: "contact" }
    : {
        touch: item.company?.touch,
        name: item.subject?.type === "company" ? item.subject.label : undefined,
        of: "company",
      };
  const { touch, name } = held;
  if (!touch || !name) return [];
  const gap =
    held.of === "contact"
      ? (["worklist.happened.noneFrom", "worklist.happened.noneTo"] as const)
      : (["worklist.happened.neverFrom", "worklist.happened.neverTo"] as const);
  const day = (at: string, precision: WorklistItem["occurred_precision"]) =>
    activityDay(at, precision, clock.locale, clock.zone, (d, l, z) =>
      formatDayMonthNearYear(d, l, z, clock.now),
    );
  return [
    touch.last_inbound_at
      ? t("worklist.happened.lastFrom", {
          name,
          when: day(
            touch.last_inbound_at,
            touch.last_inbound_precision ?? "instant",
          ),
        })
      : t(gap[0], { name }),
    touch.last_outbound_at
      ? t("worklist.happened.lastTo", {
          name,
          when: day(
            touch.last_outbound_at,
            touch.last_outbound_precision ?? "instant",
          ),
        })
      : t(gap[1], { name }),
  ];
}

const CONSEQUENCE: Readonly<
  Record<
    WorklistItem["consequence"],
    Readonly<{ named?: MessageKey; bare: MessageKey }> | null
  >
> = {
  none: null,
  buyer_waits: {
    named: "worklist.consequence.buyer_waits.named",
    bare: "worklist.consequence.buyer_waits",
  },
  promise_breaks: {
    named: "worklist.consequence.promise_breaks.named",
    bare: "worklist.consequence.promise_breaks",
  },
  deal_drifts: {
    named: "worklist.consequence.deal_drifts.named",
    bare: "worklist.consequence.deal_drifts",
  },
  deal_slips_past_close: {
    named: "worklist.consequence.deal_slips_past_close.named",
    bare: "worklist.consequence.deal_slips_past_close",
  },
  meeting_unprepared: { bare: "worklist.consequence.meeting_unprepared" },
  task_slips: {
    named: "worklist.consequence.task_slips.named",
    bare: "worklist.consequence.task_slips",
  },
  work_blocked: { bare: "worklist.consequence.work_blocked" },
  customer_never_received: {
    named: "worklist.consequence.customer_never_received.named",
    bare: "worklist.consequence.customer_never_received",
  },
  you_believe_it_happened: {
    bare: "worklist.consequence.you_believe_it_happened",
  },
  legal_deadline_missed: { bare: "worklist.consequence.legal_deadline_missed" },
  mailbox_blind: { bare: "worklist.consequence.mailbox_blind" },
  data_drifts: { bare: "worklist.consequence.data_drifts" },
};

// The deal consequences name the deal; every other one names the contact.
const DEAL_CONSEQUENCES = new Set<string>([
  "deal_drifts",
  "deal_slips_past_close",
]);

/**
 * What happens if nobody acts, or null when nothing does. A consequence this
 * build has no sentence for is left out, as an unknown reason is.
 */
export function consequenceLine(item: WorklistItem, t: T): string | null {
  const consequence = item.consequence;
  const keys = valueFor(CONSEQUENCE, consequence, null);
  if (!keys) return null;
  const name = DEAL_CONSEQUENCES.has(consequence)
    ? item.subject?.type === "deal"
      ? item.subject.label
      : undefined
    : partyName(item);
  return keys.named && name ? t(keys.named, { name }) : t(keys.bare);
}

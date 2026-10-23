import type { components } from "../../api/schema";
import type { ReadingTone } from "../../design-system/readingtone";
import {
  formatDayMonth,
  formatMoneyWhole,
  formatNumber,
} from "../../format/format";
import { daysPast } from "../../format/lateness";
import { type Locale, type Translator, translatePlural } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import type { TakenOver } from "./claimtakeover";
import { omitted } from "./contact360";

type Contact360 = components["schemas"]["Contact360"];

/** One row of an overview card: what it is, the line under it, and either a
 *  figure or a pill at its right. `tick` is a read of a promise's done state. */
export type OverviewRow = Readonly<{
  key: string;
  title: string;
  sub?: string;
  value?: string;
  pill?: Readonly<{ text: string; tone: ReadingTone }>;
  tick?: boolean;
  // A promise of ours nobody holds, which the reader may take over.
  takeable?: boolean;
  takenBy?: TakenOver;
}>;

// --- Commitments ----------------------------------------------------------

// ours / theirs / questions, in that order: what WE owe leads, because it is
// the only one entirely within the reader's control. A null prefix key means
// the contact's own name, which no catalog can carry.
const LOOPS: ReadonlyArray<{ kind: string; prefixKey: MessageKey | null }> = [
  { kind: "commitment_ours", prefixKey: "contact.loops.team" },
  { kind: "commitment_theirs", prefixKey: null },
  { kind: "open_question", prefixKey: "contact.loops.question" },
];

type OpenLoop = Readonly<{
  key: string;
  body: string;
  prefixKey: MessageKey | null;
  dueAt: string | null;
  done: boolean;
  theirs: boolean;
  quote?: string;
  takeable: boolean;
  takenBy?: TakenOver;
}>;

/**
 * The promises the record's conversations hold, both ways, with the open
 * questions after them. A claim that became a task is the task's while the
 * task is open, so it is not listed twice. Once the task is kept, the claim
 * stays as the record that it was kept.
 */
function openLoops(view: Contact360): readonly OpenLoop[] {
  const claims = view.claims ?? [];
  const openTasks = new Set(
    (view.next_steps?.data ?? []).map((task) => task.id),
  );
  return LOOPS.flatMap((loop) =>
    claims
      .filter(
        (claim) =>
          claim.kind === loop.kind &&
          claim.status !== "dismissed" &&
          !claim.needs_review &&
          (!claim.task_activity_id ||
            claim.taken_at ||
            (claim.status === "done" &&
              !openTasks.has(claim.task_activity_id))),
      )
      .map(
        (claim): OpenLoop => ({
          key: claim.id,
          body: claim.body,
          prefixKey: loop.prefixKey,
          dueAt: claim.due_at ?? null,
          done: claim.status === "done",
          theirs: loop.kind === "commitment_theirs",
          quote: claim.source_quote,
          takeable:
            loop.kind === "commitment_ours" &&
            claim.status === "open" &&
            !claim.task_activity_id &&
            !claim.taken_by_user_id,
          takenBy: claim.taken_at
            ? { name: claim.taken_by_name ?? null, at: claim.taken_at }
            : undefined,
        }),
      ),
  );
}

// What the band counts as still owed: a finding awaiting review is not an
// obligation yet, and a kept promise no longer is one.
export function hasOpenCommitments(view: Contact360): boolean {
  return openLoops(view).some((loop) => !loop.done);
}

export function commitmentRows(
  view: Contact360,
  firstName: string,
  nowMs: number,
  t: Translator,
  locale: Locale,
): OverviewRow[] {
  return openLoops(view).map((loop) => {
    const prefix = loop.theirs
      ? firstName
      : loop.prefixKey
        ? t(loop.prefixKey)
        : "";
    return {
      key: loop.key,
      title: prefix ? `${prefix}: ${loop.body}` : loop.body,
      sub: loop.quote
        ? t("commitment.quote", { quote: loop.quote })
        : undefined,
      pill: loopPill(loop, nowMs, t, locale),
      tick: loop.done,
      takeable: loop.takeable,
      takenBy: loop.takenBy,
    };
  });
}

// A kept promise names no deadline, so a ticked row never reads "overdue".
// The verdict comes from the instant. The count only picks the words: a
// promise 23 hours past due is late by no whole days and still late.
function loopPill(
  loop: OpenLoop,
  nowMs: number,
  t: Translator,
  locale: Locale,
): OverviewRow["pill"] {
  if (loop.done) {
    return undefined;
  }
  const dueMs = loop.dueAt ? Date.parse(loop.dueAt) : Number.NaN;
  if (!Number.isNaN(dueMs)) {
    const { days, late } = daysPast(dueMs, nowMs);
    if (late) {
      return {
        tone: "danger",
        text:
          days > 0
            ? translatePlural(locale, "contact.loops.overdue", days, {
                count: formatNumber(days, locale),
              })
            : t("contact.loops.overdueUnderDay"),
      };
    }
    return {
      tone: "warning",
      text: t("contact.loops.due", { when: dueWord(nowMs, dueMs, t, locale) }),
    };
  }
  return {
    tone: "neutral",
    text: loop.theirs
      ? t("contact.loops.waiting")
      : t("contact.loops.openBadge"),
  };
}

// The days still ahead of a due moment are the days `now` is past it with
// the arguments swapped, so ahead and late are one count.
function dueWord(
  nowMs: number,
  dueMs: number,
  t: Translator,
  locale: Locale,
): string {
  const { days } = daysPast(nowMs, dueMs);
  if (days === 0) {
    return t("contact.loops.dueToday");
  }
  if (days === 1) {
    return t("contact.loops.dueTomorrow");
  }
  return t("contact.loops.dueInDays", { count: formatNumber(days, locale) });
}

// --- The deal they decide ---------------------------------------------------

type DealCard =
  | Readonly<{ state: "withheld" | "none" }>
  | Readonly<{
      state: "deal";
      dealId: string;
      row: OverviewRow;
      foot: Readonly<{ text: string; tone: ReadingTone }>;
    }>;

/**
 * The open deal this contact sits on, as one row, with a foot about the
 * buying committee. The wire says who holds which role and nothing about
 * how engaged each seat is, so the foot says only what the roles say.
 */
export function dealCard(
  view: Contact360,
  t: Translator,
  locale: Locale,
  recordZone: string,
): DealCard {
  const commercial = view.commercial;
  if (!commercial || omitted(view, "commercial")) {
    return { state: "withheld" };
  }
  const deal = commercial.deal;
  if (!deal) {
    return { state: "none" };
  }
  const sub = [
    deal.stage,
    // A close date is a date-only value, read in the record's zone so two
    // colleagues quote the same day.
    deal.close_date
      ? t("contact.commercial.closes", {
          date: formatDayMonth(deal.close_date, locale, recordZone),
        })
      : null,
  ]
    .filter(Boolean)
    .join(" · ");
  const roles = [
    ...(commercial.role ? [commercial.role] : []),
    ...commercial.committee.map((member) => member.role),
  ];
  const seated = roles.length;
  return {
    state: "deal",
    dealId: deal.deal_id,
    row: {
      key: deal.deal_id,
      title: deal.title,
      sub: sub || undefined,
      value:
        deal.amount_minor != null && deal.currency
          ? formatMoneyWhole(deal.amount_minor, deal.currency, locale)
          : undefined,
    },
    foot: roles.includes("economic_buyer")
      ? {
          tone: "neutral",
          text: translatePlural(locale, "contact.card.committee", seated, {
            count: formatNumber(seated, locale),
          }),
        }
      : { tone: "warning", text: t("contact.card.noEconomicBuyer") },
  };
}

// --- Understanding them ----------------------------------------------------

const MATTERS: ReadonlyArray<{ kind: string; labelKey: MessageKey }> = [
  { kind: "priority", labelKey: "contact.matters.priorities" },
  { kind: "objection", labelKey: "contact.matters.objections" },
  { kind: "success_criterion", labelKey: "contact.matters.successCriteria" },
];

/** The three things a seller asks about a buyer, each with the live claim
 *  that answers it or the honest "nothing captured yet". Null when the
 *  claims are withheld from this reader. */
export function understandingRows(
  view: Contact360,
  t: Translator,
): OverviewRow[] | null {
  if (omitted(view, "claims")) {
    return null;
  }
  const claims = view.claims ?? [];
  return MATTERS.map((row) => {
    const match = claims.find(
      (claim) =>
        claim.kind === row.kind &&
        claim.status !== "dismissed" &&
        !claim.needs_review,
    );
    return {
      key: row.kind,
      title: t(row.labelKey),
      sub: match ? match.body : t("contact.matters.absent"),
    };
  });
}

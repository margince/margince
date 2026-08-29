import { formatDateTime, formatNumber } from "../format/format";
import type { Locale, useT } from "../i18n";
import { approvalKindLabel } from "./approvalkind";
import type { AttentionItem } from "./today.queries";

type T = ReturnType<typeof useT>;

// One item's headline and supporting line, written once for every lane that
// draws an item. Both lanes need them — the reporting rows and the decision
// card's report body — and a second spelling is how one product comes to
// describe the same item two ways on one page.

// One item's headline.
//
// The server sends a sentence when it HAS one — an approval summary is composed
// at staging time out of the proposal's own facts. It deliberately sends none
// for a duplicate pair, because that sentence would have to be invented and the
// server has no language to invent it in. So the client writes that one, in the
// reader's.
export function itemTitle(item: AttentionItem, t: T): string {
  if (item.title) {
    return item.title;
  }
  if (item.source === "dedupe_candidate") {
    return t(`day.duplicate.${duplicateNoun(item.kind)}` as const);
  }
  // A data-subject request's sentence is the client's to write, like the
  // duplicate's: the server sends the request's kind, and the three locales
  // each say what that kind obliges.
  if (item.source === "dsr") {
    return t(`day.dsr.kind.${dsrNoun(item.kind)}` as const);
  }
  // A sync concern's sentence is the client's to write for the same reason:
  // the server names the condition, and each locale says what it means.
  if (item.source === "sync_health") {
    return t(`day.syncHealth.kind.${syncNoun(item.kind)}` as const);
  }
  // A capture concern's sentence likewise: the server names the condition,
  // the card's detail names the mailbox, and the locale writes the line.
  if (item.source === "capture_health") {
    return t(`day.captureHealth.kind.${captureNoun(item.kind)}` as const);
  }
  // A troubled AI run without its own recorded summary (the title branch at
  // the top already used one where it exists) gets the locale's line for
  // failed or stalled.
  if (item.source === "ai_work_health") {
    return t(`day.aiWork.kind.${aiWorkNoun(item.kind)}` as const);
  }
  return item.kind ? approvalKindLabel(item.kind, t) : t("day.item.untitled");
}

// Which sync condition a concern names. Only a condition with its own
// sentence names itself; anything else takes the generic line rather than
// guessing at an operational fact.
function syncNoun(
  kind: string | undefined,
):
  | "sync_failing"
  | "budget_degraded"
  | "objects_stale"
  | "backfill_incomplete"
  | "generic" {
  if (
    kind === "sync_failing" ||
    kind === "budget_degraded" ||
    kind === "objects_stale" ||
    kind === "backfill_incomplete"
  ) {
    return kind;
  }
  return "generic";
}

// Which trouble an AI run is in; anything unrecognised takes the generic
// line rather than guessing.
function aiWorkNoun(
  kind: string | undefined,
): "failed" | "stalled" | "generic" {
  if (kind === "failed" || kind === "stalled") {
    return kind;
  }
  return "generic";
}

// Which mailbox condition a capture concern names; anything unrecognised
// takes the generic line rather than guessing at what broke.
function captureNoun(
  kind: string | undefined,
):
  | "reauth_required"
  | "connection_error"
  | "sync_failing"
  | "backfill_failed"
  | "generic" {
  if (
    kind === "reauth_required" ||
    kind === "connection_error" ||
    kind === "sync_failing" ||
    kind === "backfill_failed"
  ) {
    return kind;
  }
  return "generic";
}

// Which obligation a data-subject request carries. Only a kind with its own
// sentence names itself; anything else takes the generic line rather than
// mislabelling a legal duty.
function dsrNoun(
  kind: string | undefined,
): "access" | "erasure" | "rectify" | "generic" {
  if (kind === "access" || kind === "erasure" || kind === "rectify") {
    return kind;
  }
  return "generic";
}

// Which noun a duplicate pair is about.
//
// Only a kind this reader has a sentence for names itself; anything else takes
// the generic line. Naming the wrong noun is worse than naming none: the pair
// the server found may be two deals or two projects, and calling them contacts
// tells the reader to compare something that is not on screen.
function duplicateNoun(
  kind: string | undefined,
): "person" | "org" | "lead" | "generic" {
  if (kind === "person") {
    return "person";
  }
  if (kind === "organization") {
    return "org";
  }
  if (kind === "lead") {
    return "lead";
  }
  return "generic";
}

// A lapsed relationship says how long the silence has run and when it started.
// Both, because the number is what a rep acts on and the date is what makes it
// checkable against the contact's own timeline.
function decayDetail(
  item: AttentionItem,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  if (!item.detail) {
    return null;
  }
  const days = formatNumber(Number(item.detail), locale);
  return item.occurred_at
    ? t("day.decay.quietSince", {
        days,
        date: formatDateTime(item.occurred_at, locale, zone),
      })
    : t("day.decay.quiet", { days });
}

// A risk card's sentence is written HERE, not sent: the server has no language,
// and "quiet 19 days" versus "the close date passed" read differently in each of
// the three. `kind` names the ground and `detail` carries the number the server
// actually measured, so the line can never imply a patience nobody applied.
function riskDetail(
  item: AttentionItem,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  if (item.kind === "close_overdue" && item.due_at) {
    return t("day.risk.closeOverdue", {
      date: formatDateTime(item.due_at, locale, zone),
    });
  }
  if (item.detail) {
    return t("day.risk.quiet", {
      days: formatNumber(Number(item.detail), locale),
    });
  }
  return null;
}

// The localized line for a sync concern's cause, when its detail is one of
// the two closed vocabularies (the sweep's failure class, the budget band).
// Anything else — the affected object classes — is data and stays as sent.
function syncCauseLine(item: AttentionItem, t: T): string | null {
  if (
    item.kind === "sync_failing" &&
    (item.detail === "auth" ||
      item.detail === "rate_limited" ||
      item.detail === "internal")
  ) {
    return t(`day.syncHealth.cause.${item.detail}` as const);
  }
  if (
    item.kind === "budget_degraded" &&
    (item.detail === "warn" || item.detail === "shed")
  ) {
    return t(`day.syncHealth.band.${item.detail}` as const);
  }
  return null;
}

// The supporting line under a headline: how sure the detector was, or when this
// is due, or when it happened. At most one — a card that stacked all three
// would make the reader read three things to learn one.
export function itemDetail(
  item: AttentionItem,
  t: T,
  locale: Locale,
  zone: string,
): string | null {
  if (item.source === "relationship_decay") {
    return decayDetail(item, t, locale, zone);
  }
  if (item.source === "deal_at_risk") {
    return riskDetail(item, t, locale, zone);
  }
  // A promise is the one item whose supporting line carries TWO facts, and it
  // needs both: the words it was read from are what make the claim checkable,
  // and the deadline is why it is on today's page at all. Showing only the
  // quote would drop the date the lane is ordered by.
  if (item.source === "conversation_claim" && item.detail && item.due_at) {
    return t("day.commitment.detail", {
      quote: item.detail,
      due: formatDateTime(item.due_at, locale, zone),
    });
  }
  // A sync concern's cause travels in the producer's own vocabulary; the two
  // closed sets get the reader's language, and the affected object classes
  // stay the data they are.
  if (item.source === "sync_health" && item.detail) {
    return syncCauseLine(item, t) ?? item.detail;
  }
  if (item.detail) {
    return item.detail;
  }
  if (item.confidence != null) {
    return t("day.match", {
      percent: formatNumber(Math.round(item.confidence * 100), locale),
    });
  }
  if (item.occurred_at) {
    return formatDateTime(item.occurred_at, locale, zone);
  }
  if (item.due_at) {
    return formatDateTime(item.due_at, locale, zone);
  }
  return null;
}

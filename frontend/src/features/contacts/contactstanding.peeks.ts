import type { components } from "../../api/schema";
import type { StandingPeek } from "../../design-system/standingdims";
import { activityHasBegun } from "../../format/activitywhen";
import { formatNumber } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { daysPast } from "../../format/lateness";
import { messageDetail } from "../../lib/standingmarks";
import { type PeekWords, partialFoot } from "../../lib/standingpeeks";
import { omitted } from "./contact360";

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];

const NINETY_DAYS_MS = 90 * 24 * 60 * 60 * 1000;
const MESSAGE_KINDS = new Set<Activity["kind"]>(["email", "message", "call"]);

/**
 * What the contact page's readings count, as the cards they open. They are the open
 * commitments, the messages of the last 90 days, the colleagues in touch and
 * the emails waiting on a reply. `open` is where a row leads (the History
 * tab); a card is only as long as the rows the page already holds.
 */
export type ContactPeeks = Readonly<{
  promises?: StandingPeek;
  exchange?: StandingPeek;
  reach?: StandingPeek;
  unanswered?: StandingPeek;
}>;

export function contactPeeks(
  view: Contact360,
  words: PeekWords,
  open: () => void,
): ContactPeeks {
  return {
    promises: promisesPeek(view, words, open),
    exchange: exchangePeek(view, words, open),
    reach: reachPeek(view, words),
    unanswered: unansweredPeek(view, words, open),
  };
}

function promisesPeek(
  view: Contact360,
  { t, locale, zone }: PeekWords,
  open: () => void,
): StandingPeek | undefined {
  if (omitted(view, "claims")) {
    return undefined;
  }
  const asOf = Date.parse(view.as_of);
  const items = (view.claims ?? [])
    .filter(
      (claim) => claim.kind === "commitment_ours" && claim.status === "open",
    )
    .map((claim) => {
      const due = claim.due_at ?? undefined;
      const day = due ? formatDayMonth(due, locale, zone) : undefined;
      const late = due ? daysPast(Date.parse(due), asOf).late : false;
      return {
        key: claim.id,
        title: claim.body,
        sub: !day
          ? t("standing.next.peek.undated")
          : late
            ? t("standing.next.peek.overdue", { date: day })
            : day,
        onOpen: open,
      };
    });
  return items.length > 0
    ? { head: t("contact.standing.peek.promises"), items }
    : undefined;
}

function exchangePeek(
  view: Contact360,
  words: PeekWords,
  open: () => void,
): StandingPeek | undefined {
  const { t, locale, zone } = words;
  const inbound = view.strength?.inbound_90d;
  const outbound = view.strength?.outbound_90d;
  if (
    omitted(view, "activities") ||
    inbound === undefined ||
    outbound === undefined
  ) {
    return undefined;
  }
  const total = inbound + outbound;
  const asOf = Date.parse(view.as_of);
  const rows = (view.activities?.data ?? [])
    .filter(
      (row) =>
        MESSAGE_KINDS.has(row.kind) &&
        (row.email_summary?.direction ?? row.direction) &&
        activityHasBegun(row.occurred_at, row.occurred_precision, asOf, zone) &&
        asOf - Date.parse(row.occurred_at) <= NINETY_DAYS_MS,
    )
    .sort((a, b) => Date.parse(b.occurred_at) - Date.parse(a.occurred_at))
    .slice(0, total);
  if (rows.length === 0) {
    return undefined;
  }
  return {
    head: t("contact.standing.peek.exchange"),
    items: rows.map((row) => ({
      key: row.id,
      title:
        (row.subject ?? row.email_summary?.subject)?.trim() ||
        t(`timeline.kind.${row.kind}`),
      sub: [
        messageDetail(row, t) ??
          ((row.email_summary?.direction ?? row.direction) === "outbound"
            ? t("timeline.sent")
            : t("timeline.received")),
        formatDayMonth(row.occurred_at, locale, zone),
      ].join(" · "),
      onOpen: open,
    })),
    foot: partialFoot(rows.length, total, words),
  };
}

function reachPeek(
  view: Contact360,
  { t, plural, locale, zone }: PeekWords,
): StandingPeek | undefined {
  if (omitted(view, "network")) {
    return undefined;
  }
  const items = (view.network?.colleagues ?? [])
    .filter((colleague) => colleague.interactions_90d > 0)
    .sort((a, b) => b.interactions_90d - a.interactions_90d)
    .map((colleague) => ({
      key: colleague.user_id,
      title: colleague.display_name,
      sub: [
        plural("contact.standing.peek.exchanges", colleague.interactions_90d, {
          count: formatNumber(colleague.interactions_90d, locale),
        }),
        colleague.last_at
          ? t("standing.peek.lastInTouch", {
              date: formatDayMonth(colleague.last_at, locale, zone),
            })
          : undefined,
      ]
        .filter(Boolean)
        .join(" · "),
    }));
  return items.length > 0
    ? { head: t("contact.standing.peek.reach"), items }
    : undefined;
}

function unansweredPeek(
  view: Contact360,
  { t, locale, zone }: PeekWords,
  open: () => void,
): StandingPeek | undefined {
  if (omitted(view, "activities")) {
    return undefined;
  }
  const items = (view.activities?.data ?? [])
    .filter((row) => row.email_summary?.move === "needs_reply")
    .map((row) => ({
      key: row.id,
      title: row.subject?.trim() || t("timeline.kind.email"),
      sub: [
        messageDetail(row, t),
        formatDayMonth(row.occurred_at, locale, zone),
      ]
        .filter(Boolean)
        .join(" · "),
      onOpen: open,
    }));
  return items.length > 0
    ? { head: t("contact.standing.peek.unanswered"), items }
    : undefined;
}

import type { PeekContent } from "../../design-system/peek";
import type { StandingPeek } from "../../design-system/standingdims";
import { formatMoney } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { routeHash } from "../../lib/router";
import { type PeekWords, partialFoot } from "../../lib/standingpeeks";
import type { Project360 } from "./projectsections";
import { commitmentsReading, dealsReading, timeUsed } from "./projectstanding";

/**
 * The open tasks the Open tasks reading and figure count, soonest due first as
 * the server sends them. `open` is where a row leads (the Open tasks card); the
 * rollup counts every open task, so a page cut short
 * says how many it shows.
 */
export function commitmentsPeek(
  view: Project360,
  words: PeekWords,
  open: () => void,
): PeekContent | undefined {
  const { t, locale, zone } = words;
  const rows = view.commitments?.data ?? [];
  const reading = commitmentsReading(view);
  if (rows.length === 0 || !reading) {
    return undefined;
  }
  return {
    head: t("project.standing.peek.commitments"),
    items: rows.map((row) => ({
      key: row.activity_id,
      title: row.subject,
      sub: [
        row.assignee_name,
        row.due_at
          ? t(
              row.overdue
                ? "standing.next.peek.overdue"
                : "project.standing.nextDue",
              { date: formatDayMonth(row.due_at, locale, zone) },
            )
          : t("standing.next.peek.undated"),
      ]
        .filter(Boolean)
        .join(" · "),
      onOpen: open,
    })),
    foot: partialFoot(rows.length, reading.open, words),
  };
}

/** Which deals the Deals reading counts, with where each stands. */
export function dealsPeek(
  view: Project360,
  { t, plural, locale, zone }: PeekWords,
): PeekContent | undefined {
  const rows = view.deals?.data ?? [];
  const reading = dealsReading(view);
  if (rows.length === 0 || !reading) {
    return undefined;
  }
  return {
    head: t("project.standing.peek.deals"),
    items: rows.map((deal) => ({
      key: deal.id,
      title: deal.name,
      sub: dealStanding(deal, { t, locale, zone }),
      right:
        deal.amount_minor != null && deal.currency
          ? formatMoney(deal.amount_minor, deal.currency, locale)
          : undefined,
      href: routeHash({ screen: "deals", id: deal.id }),
    })),
    foot: reading.isFloor
      ? plural("standing.peek.first", rows.length, {
          count: String(rows.length),
        })
      : undefined,
  };
}

/**
 * How "Day 166 of 274" is counted. It gives the day the work started, the
 * target end and the day today is, so the numbers are never a formula to trust.
 */
export function timePeek(
  view: Project360,
  { t, locale, zone }: PeekWords,
): StandingPeek | undefined {
  const used = timeUsed(view, zone);
  const { started_at: start, target_end_date: end } = view.project;
  if (used.kind !== "running" || !start || !end) {
    return undefined;
  }
  return {
    head: t("project.standing.peek.time"),
    items: [
      {
        key: "start",
        title: t("project.standing.peek.started"),
        sub: formatDayMonth(start, locale, zone),
      },
      {
        key: "today",
        title: t("project.standing.peek.today"),
        sub: t("project.head.dayOf", {
          day: String(used.day),
          total: String(used.total),
        }),
      },
      {
        key: "end",
        title: t("project.aside.targetEnd"),
        sub: formatDayMonth(end, locale, zone),
      },
    ],
  };
}

/** The files the Documents figure counts, newest first. */
export function documentsPeek(
  view: Project360,
  { t, plural }: PeekWords,
): PeekContent | undefined {
  const rows = view.documents?.data ?? [];
  if (rows.length === 0) {
    return undefined;
  }
  return {
    head: t("project.standing.peek.documents"),
    items: rows.map((file) => ({
      key: file.id,
      title: file.title || file.filename,
      sub: file.title ? file.filename : undefined,
    })),
    foot: view.documents?.page.has_more
      ? plural("standing.peek.first", rows.length, {
          count: String(rows.length),
        })
      : undefined,
  };
}

type Deal = NonNullable<Project360["deals"]>["data"][number];

// Where a deal stands: open, or won or lost on the day it closed.
function dealStanding(
  deal: Deal,
  { t, locale, zone }: Pick<PeekWords, "t" | "locale" | "zone">,
): string {
  if (deal.status === "open") {
    return t("co.deals.status.open");
  }
  const won = deal.status === "won";
  if (!deal.closed_at) {
    return t(won ? "co.deals.status.won" : "co.deals.status.lost");
  }
  return t(won ? "project.deals.wonOn" : "project.deals.lostOn", {
    date: formatDayMonth(deal.closed_at, locale, zone),
  });
}

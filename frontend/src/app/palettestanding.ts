// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { StatusKind } from "../design-system/statusbadge";
import { activitySince } from "../format/activitywhen";
import { formatMoneyWhole } from "../format/format";
import { formatDayMonthNearYear } from "../format/format.dates";
import { LEAD_STATUS_LABEL } from "../format/leadstatus";
import { isProjectPhase, PHASE_LABEL } from "../format/projectphase";
import type { Locale, Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { labelFor } from "../lib/labelfor";

type SearchResult = components["schemas"]["SearchResult"];
type Lead = components["schemas"]["Lead"];

/** What a palette row shows besides the name: a second line and one status. */
export type HitDescription = Readonly<{
  subtitle?: string;
  status?: Readonly<{ kind: StatusKind; label: string }>;
  aside?: string;
}>;

type Reading = Readonly<{
  t: Translator;
  locale: Locale;
  zone: string;
  now: Date;
}>;

const DEAL_STATE: Readonly<Record<string, [StatusKind, MessageKey]>> = {
  won: ["done", "deals.statusWon"],
  lost: ["neutral", "deals.statusLost"],
};

function isLeadStatus(value: string): value is Lead["status"] {
  return Object.hasOwn(LEAD_STATUS_LABEL, value);
}

// A new lead waits on someone; a promoted one is done; the rest are under way.
function leadStatusKind(status: Lead["status"]): StatusKind {
  if (status === "new") return "attention";
  return status === "promoted" ? "done" : "neutral";
}

/**
 * When the record was last in touch. A date-only touch names its date: an age
 * counted from its stored noon UTC is a day short far east of UTC.
 */
function lastContactOf(
  standing: NonNullable<SearchResult["standing"]>,
  { t, locale, now }: Reading,
): string | undefined {
  const at = standing.last_activity_at;
  if (!at) return undefined;
  const precision = standing.last_activity_precision ?? "instant";
  return t("palette.lastContact", {
    when: activitySince({ at, precision }, now, locale, (day, locale, zone) =>
      formatDayMonthNearYear(day, locale, zone, now),
    ),
  });
}

/**
 * The second line and status of one hit, from the standing the search answer
 * carries. `fallback` is the line a hit shows without one: an employer, a
 * partner mark, the server's snippet.
 */
export function describeHit(
  hit: SearchResult,
  fallback: string | undefined,
  reading: Reading,
): HitDescription {
  const standing = hit.standing;
  if (!standing) return { subtitle: fallback };
  const { t, locale, zone, now } = reading;
  const day = (date: string) => formatDayMonthNearYear(date, locale, zone, now);
  const lastContact = lastContactOf(standing, reading);
  switch (hit.type) {
    case "contact":
      return {
        subtitle: fallback ?? standing.job_title ?? undefined,
        aside: lastContact,
      };
    case "company":
      return { subtitle: fallback, aside: lastContact };
    case "deal":
      return describeDeal(standing, reading, day);
    case "lead": {
      const status = standing.state;
      return {
        subtitle: fallback,
        status:
          status && isLeadStatus(status)
            ? {
                kind: leadStatusKind(status),
                label: t(labelFor(LEAD_STATUS_LABEL, status)),
              }
            : undefined,
      };
    }
    case "project": {
      const phase = standing.state;
      const due = standing.due_on
        ? t("palette.due", { date: day(standing.due_on) })
        : undefined;
      return {
        subtitle: joined([fallback, due]),
        status:
          phase && isProjectPhase(phase)
            ? {
                kind: phase === "closed" ? "done" : "neutral",
                label: t(labelFor(PHASE_LABEL, phase)),
              }
            : undefined,
      };
    }
    default:
      return { subtitle: fallback };
  }
}

function describeDeal(
  standing: components["schemas"]["SearchHitStanding"],
  { t, locale }: Reading,
  day: (date: string) => string,
): HitDescription {
  const money =
    standing.amount_minor != null && standing.currency
      ? formatMoneyWhole(standing.amount_minor, standing.currency, locale)
      : undefined;
  const closes =
    standing.due_on && standing.state === "open"
      ? t("deal.closes", { date: day(standing.due_on) })
      : undefined;
  const closed = standing.state ? DEAL_STATE[standing.state] : undefined;
  const label = closed ? t(closed[1]) : standing.stage;
  return {
    subtitle: joined([money, closes]),
    status: label ? { kind: closed?.[0] ?? "neutral", label } : undefined,
  };
}

function joined(parts: readonly (string | undefined)[]): string | undefined {
  const present = parts.filter(Boolean);
  return present.length > 0 ? present.join(" · ") : undefined;
}

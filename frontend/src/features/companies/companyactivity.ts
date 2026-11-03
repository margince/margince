// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { omitted } from "../../design-system/surfacestate";
import { activityDay, activityDaysAway } from "../../format/activitywhen";
import { formatDayMonthNearYear } from "../../format/format.dates";
import type { Locale, Translator } from "../../i18n";
import type { CompanyVerdict } from "./companystanding";

type Company360 = components["schemas"]["Company360"];

// An account last spoken to inside a month counts as active.
const ACTIVE_DAYS = 30;
// Past this, an account with nothing open is dormant rather than going quiet.
const DORMANT_DAYS = 180;

/**
 * The account's call when nobody has rated it: how recently anyone here was
 * spoken to, and whether anything is open. Undefined when the page may not
 * read the last exchange, because no call is better than a guess.
 */
export function companyActivityVerdict(
  view: Company360,
  {
    t,
    locale,
    zone,
  }: Readonly<{ t: Translator; locale: Locale; zone: string }>,
): CompanyVerdict | undefined {
  if (omitted(view, "last_touch")) {
    return undefined;
  }
  const asOf = new Date(view.as_of);
  const day = (at: string, l: Locale, z: string) =>
    formatDayMonthNearYear(at, l, z, asOf);
  const last = view.last_contact;
  if (!last) {
    return {
      word: "co.verdict.noContact",
      sentence: t("co.verdict.addedOn", {
        date: day(view.company.created_at, locale, zone),
      }),
      tone: "neutral",
    };
  }
  const date = activityDay(last.at, last.occurred_precision, locale, zone, day);
  const days = -activityDaysAway(
    last.at,
    last.occurred_precision,
    Date.parse(view.as_of),
    zone,
    zone,
  );
  if (days <= ACTIVE_DAYS) {
    return {
      word: "co.verdict.active",
      sentence: t("co.verdict.lastExchange", { date }),
      tone: "success",
    };
  }
  if (days > DORMANT_DAYS && nothingOpen(view)) {
    return {
      word: "co.verdict.dormant",
      sentence: t("co.verdict.dormantSince", { date }),
      tone: "neutral",
    };
  }
  return {
    word: "co.verdict.goingQuiet",
    sentence: t("co.verdict.quietSince", { date }),
    tone: "warning",
  };
}

// A cut deal list may hide an open deal, so only a whole one can say none is.
function nothingOpen(view: Company360): boolean {
  const deals = view.deals;
  if (omitted(view, "deals") || !deals || deals.page.has_more) {
    return false;
  }
  return deals.data.every((deal) => deal.status !== "open");
}

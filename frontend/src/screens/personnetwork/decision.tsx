// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The three readings a rep opens this tab for: who can open the door, why it
// is worth doing now, and who owns the next move.
//
// They sit above everything because they are the answer. The evidence under
// them explains it; a reader who trusts the answer never has to scroll.

import type { components } from "../../api/schema";
import { StatCard } from "../../design-system/atoms";
import { StatStrip } from "../../design-system/statstrip";
import { formatNumber } from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import type { IntroRequest } from "../introrequests";
import { useOwnRoute } from "../personroutes";
import { ownerOf } from "./relay";

type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];
type Translate = ReturnType<typeof useT>;
type Pluralize = ReturnType<typeof usePlural>;

/**
 * DecisionStrip states who reaches them, the reason to act, and the handoff.
 *
 * The first slot counts the ways in rather than naming the best one: the
 * verdict panel above it already names the lead, and a strip repeating the
 * same name in a smaller type read as two findings rather than one. The
 * reader is among the routes the server ranks, so the count names them
 * instead of counting them as a colleague to ask.
 *
 * "Why now" comes from the person's own moment ladder rather than from a
 * relationship change: a relationship getting warmer is not by itself a reason
 * to spend a colleague's goodwill today.
 */
export function DecisionStrip({
  routes,
  legacyVia,
  whyNow,
  open,
}: Readonly<{
  routes: readonly RouteCandidate[];
  // The colleague named by a server that predates the candidate list. Without
  // it this strip would read "nobody reaches them" beside a card naming the
  // person who does — the page contradicting itself on its own headline.
  legacyVia: string | undefined;
  whyNow: string | undefined;
  // The ask in flight, if there is one. Its absence is a reading too: nobody
  // owes anybody anything yet.
  open: IntroRequest | undefined;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const own = useOwnRoute();
  const lead = routes[0];
  const mine = routes.filter(own).length;
  const indirect = routes.filter((r) => r.through_display_name).length;
  return (
    <StatStrip>
      <StatCard
        label={t("person.intro.stripWho")}
        value={
          lead
            ? whoReaches(
                { lead, total: routes.length, others: routes.length - mine },
                t,
                plural,
                locale,
              )
            : (legacyVia ?? t("person.graph.noRoute"))
        }
        detail={
          lead
            ? t("person.intro.stripWhoMix", {
                direct: formatNumber(routes.length - indirect, locale),
                indirect: formatNumber(indirect, locale),
              })
            : legacyVia
              ? t("person.intro.stripDirect")
              : t("person.intro.stripNoPath")
        }
      />
      <StatCard
        label={t("person.intro.stripWhyNow")}
        value={whyNow ?? t("person.intro.stripNoMoment")}
        detail={
          whyNow
            ? t("person.intro.stripWhyNowSub")
            : t("person.intro.stripNoMomentSub")
        }
      />
      <StatCard
        label={t("person.intro.stripHandoff")}
        value={
          open
            ? t(HANDOFF_VALUE[open.status])
            : t("person.intro.handoffNotStarted")
        }
        detail={
          open
            ? t("person.intro.handoffOwner", { name: ownerOf(open, t) })
            : t("person.intro.handoffNotStartedSub")
        }
      />
    </StatStrip>
  );
}

/**
 * whoReaches counts the ways in, in the person the reader is one of.
 *
 * The reader is ranked among the colleagues like anybody else, so "2
 * colleagues" was counting the person reading it as somebody to go and ask.
 * Their own relationship still belongs in the total — it is the strongest way
 * in this page can report — so it is named rather than dropped.
 *
 * `others` is derived from a COUNT of the reader's routes and not from a flag:
 * nothing in the contract stops the reader appearing on both a direct route and
 * one through a contact, and subtracting a boolean would leave the remainder
 * claiming a colleague who is not there.
 */
function whoReaches(
  reach: Readonly<{ lead: RouteCandidate; total: number; others: number }>,
  t: Translate,
  plural: Pluralize,
  locale: Locale,
): string {
  if (reach.others === reach.total) {
    return plural("person.intro.stripWhoCount", reach.total, {
      count: formatNumber(reach.total, locale),
      name: reach.lead.via_display_name,
    });
  }
  // Not a plural choice: whether anybody else reaches them AT ALL is a state,
  // and how many do is pluralised on its own below.
  if (reach.others === 0) {
    return t("person.intro.stripWhoOnlyYou");
  }
  return plural("person.intro.stripWhoWithYou", reach.others, {
    count: formatNumber(reach.others, locale),
  });
}

// Every status the contract admits reads as words. A state the server can send
// must never reach a reader as a raw enum.
const HANDOFF_VALUE: Record<
  IntroRequest["status"],
  Parameters<ReturnType<typeof useT>>[0]
> = {
  requested: "person.intro.stateRequested",
  accepted: "person.intro.stateAccepted",
  name_drop_approved: "person.intro.stateNameDropApproved",
  suggest_other: "person.intro.stateSuggestOther",
  declined: "person.intro.stateDeclined",
  introduced: "person.intro.stateIntroduced",
  name_dropped: "person.intro.stateNameDropped",
  replied: "person.intro.stateReplied",
  expired: "person.intro.stateExpired",
  cancelled: "person.intro.stateCancelled",
};

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
import { useLocale, usePlural, useT } from "../../i18n";
import type { IntroRequest } from "../introrequests";
import { ownerOf } from "./relay";

type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];

/**
 * DecisionStrip states who reaches them, the reason to act, and the handoff.
 *
 * The first slot counts the ways in rather than naming the best one: the
 * verdict panel above it already names the lead, and a strip repeating the
 * same name in a smaller type read as two findings rather than one.
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
  const lead = routes[0];
  const indirect = routes.filter((r) => r.through_display_name).length;
  return (
    <StatStrip>
      <StatCard
        label={t("person.intro.stripWho")}
        value={
          lead
            ? plural("person.intro.stripWhoCount", routes.length, {
                count: formatNumber(routes.length, locale),
                name: lead.via_display_name,
              })
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

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
import { useT } from "../../i18n";
import type { IntroRequest } from "../introrequests";

type RouteCandidate = components["schemas"]["PersonGraphRouteCandidate"];

/**
 * DecisionStrip states the best path, the reason to act, and the handoff.
 *
 * "Why now" comes from the person's own moment ladder rather than from a
 * relationship change: a relationship getting warmer is not by itself a reason
 * to spend a colleague's goodwill today.
 */
export function DecisionStrip({
  lead,
  legacyVia,
  whyNow,
  open,
}: Readonly<{
  lead: RouteCandidate | undefined;
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
  return (
    <StatStrip>
      <StatCard
        label={t("person.intro.stripPath")}
        value={lead?.via_display_name ?? legacyVia ?? t("person.graph.noRoute")}
        detail={
          lead
            ? routeDetail(lead, t)
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

function routeDetail(
  route: RouteCandidate,
  t: ReturnType<typeof useT>,
): string {
  return route.through_display_name
    ? t("person.intro.stripVia", { through: route.through_display_name })
    : t("person.intro.stripDirect");
}

// Who owes the next move. A status that nobody owes says so rather than naming
// a person who has already done their part.
function ownerOf(ask: IntroRequest, t: ReturnType<typeof useT>): string {
  switch (ask.status) {
    case "requested":
      return ask.introducer_display_name ?? t("person.intro.ownerColleague");
    case "accepted":
    case "name_drop_approved":
      return ask.requester_display_name ?? t("person.intro.ownerYou");
    default:
      return t("person.intro.ownerNobody");
  }
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

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
import { type Locale, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { useMe } from "../common";
import { useOwnRoute } from "../contactroutes";
import type { IntroRequest } from "../introrequests";
import { ownerOf } from "./relay";

type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];
type RelationshipChange = components["schemas"]["ContactRelationshipChange"];
type Translate = ReturnType<typeof useT>;

/**
 * DecisionStrip states who reaches them, what moved lately, and the handoff.
 *
 * The first slot COUNTS the ways in rather than naming the best one: the
 * verdict panel above it already names the lead, and a strip repeating the
 * same name in a smaller type read as two findings rather than one. The reader
 * is one of the routes the server ranks, so they are counted like anybody else
 * and named on a line of their own — a stat card states a reading and does not
 * address its reader.
 */
export function DecisionStrip({
  routes,
  legacyVia,
  change,
  changeWithheld,
  open,
}: Readonly<{
  routes: readonly RouteCandidate[];
  // The colleague named by a server that predates the candidate list. Without
  // it this strip would read "nobody reaches them" beside a card naming the
  // contact who does — the page contradicting itself on its own headline.
  legacyVia: string | undefined;
  // The newest change on this relationship, if there is one to read.
  change: RelationshipChange | undefined;
  // Whether the changes section was refused rather than empty. Two opposite
  // facts, and a card that folds them tells a reader nothing has moved on a
  // relationship it was simply not allowed to look at.
  changeWithheld: boolean;
  // The ask in flight, if there is one. Its absence is a reading too: nobody
  // owes anybody anything yet.
  open: IntroRequest | undefined;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const own = useOwnRoute();
  const reader = useMe().data?.user;
  // A name the server sent as whitespace is a name it does not have. A
  // StatCard value is a non-empty string by contract, and a blank one draws a
  // slot that reads as a reading which failed to load.
  const via = legacyVia?.trim() || undefined;
  const lead = routes[0];
  const mine = routes.some(own);
  const indirect = routes.filter((r) => r.through_display_name).length;
  // Every slot declares the NARROW shape, and every slot on the row must:
  // below the strip's two-up width a `row` slot folds to one full-width line
  // (statstrip.css), and the fold is the plate's — a strip where only some
  // cards carry it draws a bordered box among a column of borderless rows.
  return (
    <StatStrip>
      <StatCard
        narrow="row"
        label={t("contact.intro.stripWho")}
        value={
          lead
            ? formatNumber(routes.length, locale)
            : (via ?? t("contact.intro.stripNoRoutes"))
        }
        detail={
          lead ? (
            <>
              <span>
                {t("contact.intro.stripWhoMix", {
                  direct: formatNumber(routes.length - indirect, locale),
                  indirect: formatNumber(indirect, locale),
                })}
              </span>
              {/* The reader's own relationship is the strongest way in this
                  page can report, so it is named rather than dropped — on its
                  own line, because the count above already includes it. */}
              {mine ? <span>{t("contact.intro.stripWhoOwn")}</span> : null}
            </>
          ) : via ? (
            t("contact.intro.stripDirect")
          ) : (
            t("contact.intro.stripNoPath")
          )
        }
      />
      <ChangeCard change={change} withheld={changeWithheld} locale={locale} />
      <StatCard
        narrow="row"
        label={t("contact.intro.stripHandoff")}
        value={
          open
            ? t(HANDOFF_VALUE[open.status])
            : t("contact.intro.handoffNotStarted")
        }
        detail={open ? handoffDetail(open, reader, t) : undefined}
      />
    </StatStrip>
  );
}

/**
 * ChangeCard is the strip's own two-line vocabulary for one derived change.
 *
 * `changeSentence` (relationshipchange.ts) keeps writing sentences for the
 * rail and the moments panel, which have room for them. A slot has one line
 * for the verdict and one for what it rests on, and a sentence in a value is
 * the shape this whole reading exists to avoid.
 */
function ChangeCard({
  change,
  withheld,
  locale,
}: Readonly<{
  change: RelationshipChange | undefined;
  withheld: boolean;
  locale: Locale;
}>) {
  const t = useT();
  return (
    <StatCard
      narrow="row"
      label={t("contact.intro.stripWhyNow")}
      value={t(
        withheld
          ? "reading.restricted"
          : change
            ? CHANGE_VALUE[change.kind]
            : "contact.intro.stripNoMoment",
      )}
      detail={change && !withheld ? changeDetail(change, locale, t) : undefined}
    />
  );
}

// What the change rests on: the span for a reply or a silence, the two bands
// for a move. Undefined where the field that branch needs did not arrive — the
// verdict is still true, and a receipt standing on a substituted value is
// worse than none: "After 0 d quiet" and "no contact → no contact" both read
// as figures the server sent.
function changeDetail(
  change: RelationshipChange,
  locale: Locale,
  t: Translate,
): string | undefined {
  if (change.kind === "warmed" || change.kind === "cooled") {
    return change.from_bucket && change.to_bucket
      ? t("contact.intro.change.buckets", {
          from: t(`contact.band.${change.from_bucket}`),
          to: t(`contact.band.${change.to_bucket}`),
        })
      : undefined;
  }
  if (change.days == null) {
    return undefined;
  }
  const days = formatNumber(change.days, locale);
  return change.kind === "replied_after_gap"
    ? t("contact.intro.change.repliedSub", { days })
    : t("contact.intro.change.quietSub", { days });
}

// Who owes the next move, or what the answer was. A settled ask owes nobody
// anything, so naming an owner there would point at a colleague who has
// already done their part — the two that carry their own word say it instead.
// An owner nobody can name YET is named by nobody: `ownerOf` answers undefined
// while the session is still being read.
function handoffDetail(
  open: IntroRequest,
  reader: Readonly<{ id: string; display_name: string }> | undefined,
  t: Translate,
): string | undefined {
  if (open.status === "suggest_other") {
    return t("contact.intro.handoffOtherSub");
  }
  if (open.status === "expired") {
    return t("contact.intro.handoffExpiredSub");
  }
  if (!OWED.has(open.status)) {
    return undefined;
  }
  const name = ownerOf(open, t, reader);
  return name ? t("contact.intro.handoffOwner", { name }) : undefined;
}

// The statuses somebody still owes a move on.
const OWED: ReadonlySet<IntroRequest["status"]> = new Set([
  "requested",
  "accepted",
  "name_drop_approved",
]);

// Every status the contract admits reads as words. A state the server can send
// must never reach a reader as a raw enum.
const HANDOFF_VALUE: Record<IntroRequest["status"], MessageKey> = {
  requested: "contact.intro.stateRequested",
  accepted: "contact.intro.stateAccepted",
  name_drop_approved: "contact.intro.stateNameDropApproved",
  suggest_other: "contact.intro.stateSuggestOther",
  declined: "contact.intro.stateDeclined",
  introduced: "contact.intro.stateIntroduced",
  name_dropped: "contact.intro.stateNameDropped",
  replied: "contact.intro.stateReplied",
  expired: "contact.intro.stateExpired",
  cancelled: "contact.intro.stateCancelled",
};

// Total over the four kinds the contract admits, so a change the server can
// derive cannot reach a reader as a raw enum.
const CHANGE_VALUE: Record<RelationshipChange["kind"], MessageKey> = {
  replied_after_gap: "contact.intro.change.replied",
  went_quiet: "contact.intro.change.quiet",
  warmed: "contact.intro.change.warmed",
  cooled: "contact.intro.change.cooled",
};

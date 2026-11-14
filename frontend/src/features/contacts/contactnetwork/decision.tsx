// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { INTRO_STATUS_LABELS as HANDOFF_VALUE } from "../../../format/introstatus";

// The readings a rep opens this tab for: who can open the door, what moved
// lately, and who owns the next move. They are the answers the tab's chapters
// lead with, so a reader who trusts them never has to scroll.

import type { components } from "../../../api/schema";
import { formatNumber } from "../../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { labelFor } from "../../../lib/labelfor";
import { useMe } from "../../../lib/session";
import { useOwnRoute } from "../contactroutes";
import type { IntroRequest } from "../introrequests";
import { ownerOf } from "./relay";

type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];
type RelationshipChange = components["schemas"]["ContactRelationshipChange"];
type Translate = ReturnType<typeof useT>;

/** A chapter's reading: the short form a jump chip shows, and the sentence the
 * chapter leads with. */
export type ChapterReading = Readonly<{ chip: string; answer: string }>;

/**
 * useNetworkReadings states who reaches them, what moved lately, and the
 * handoff, as the answers of the Way in and Progress chapters.
 *
 * The ways in are COUNTED rather than named: the lead panel under the answer
 * already names the best one. The reader is one of the routes the server
 * ranks, so they are counted like anybody else and named in a sentence of
 * their own.
 */
export function useNetworkReadings({
  routes,
  routesWithheld,
  legacyVia,
  change,
  changeWithheld,
  open,
  openPending,
  openUnread,
}: Readonly<{
  routes: readonly RouteCandidate[];
  // Some colleagues were not shown, so no route in view is not "no route".
  routesWithheld?: boolean;
  // The colleague named by a server that predates the candidate list. Without
  // it the answer would read "nobody reaches them" over a card naming the
  // contact who does.
  legacyVia: string | undefined;
  change: RelationshipChange | undefined;
  // Refused rather than empty: a refused section reading as "nothing new" is
  // a claim about the relationship made out of a permission.
  changeWithheld: boolean;
  // The ask in flight. Its absence is a reading too: nobody owes anything yet.
  open: IntroRequest | undefined;
  // The asks are still being read, or could not be read. In both cases "not
  // started" would be a guess.
  openPending?: boolean;
  openUnread?: boolean;
}>): Readonly<{ wayIn: ChapterReading; progress: ChapterReading }> {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const own = useOwnRoute();
  const reader = useMe().data?.user;
  return {
    wayIn:
      routes.length === 0 && !legacyVia?.trim() && routesWithheld
        ? {
            chip: t("reading.restricted"),
            answer: t("contact.graph.withheldDirect"),
          }
        : wayInReading(routes, legacyVia, routes.some(own), t, plural, locale),
    progress: progressReading(
      changeReading(change, changeWithheld, locale, t),
      handoffReading(open, openPending, openUnread, reader, t),
      t,
    ),
  };
}

function wayInReading(
  routes: readonly RouteCandidate[],
  legacyVia: string | undefined,
  mine: boolean,
  t: Translate,
  plural: ReturnType<typeof usePlural>,
  locale: Locale,
): ChapterReading {
  // A name the server sent as whitespace is a name it does not have.
  const via = legacyVia?.trim() || undefined;
  if (routes.length === 0) {
    return via
      ? {
          chip: via,
          answer: t("contact.chapter.wayInAnswer", {
            routes: via,
            mix: t("contact.intro.stripDirect"),
          }),
        }
      : {
          chip: t("contact.intro.stripNoRoutes"),
          answer: t("contact.intro.stripNoPath"),
        };
  }
  const indirect = routes.filter((r) => r.through_display_name).length;
  const count = plural("contact.chapter.routes", routes.length, {
    count: formatNumber(routes.length, locale),
  });
  const mix = routeMix(routes.length, indirect, t, plural, locale);
  const answer = t("contact.chapter.wayInAnswer", { routes: count, mix });
  return {
    chip: count,
    // The reader's own relationship is the strongest way in this page can
    // report, so it is named rather than folded into the count.
    answer: mine ? `${answer} ${t("contact.intro.stripWhoOwn")}.` : answer,
  };
}

// A split with a zero side ("0 via a contact") counts what is not there, so
// one kind of route is named as such.
function routeMix(
  total: number,
  indirect: number,
  t: Translate,
  plural: ReturnType<typeof usePlural>,
  locale: Locale,
): string {
  if (indirect === 0) {
    return plural("contact.chapter.allDirect", total);
  }
  if (indirect === total) {
    return plural("contact.chapter.allVia", total);
  }
  return t("contact.intro.stripWhoMix", {
    direct: formatNumber(total - indirect, locale),
    indirect: formatNumber(indirect, locale),
  });
}

type Part = Readonly<{ value: string; detail?: string }>;

function withDetail(part: Part, t: Translate): string {
  return part.detail
    ? t("contact.chapter.withDetail", {
        value: part.value,
        detail: part.detail,
      })
    : part.value;
}

function progressReading(
  change: Part,
  handoff: Part,
  t: Translate,
): ChapterReading {
  return {
    chip: handoff.value,
    answer: t("contact.chapter.progressAnswer", {
      handoff: withDetail(handoff, t),
      change: withDetail(change, t),
    }),
  };
}

function changeReading(
  change: RelationshipChange | undefined,
  withheld: boolean,
  locale: Locale,
  t: Translate,
): Part {
  return {
    value: t(
      withheld
        ? "reading.restricted"
        : change
          ? labelFor(CHANGE_VALUE, change.kind)
          : "contact.intro.stripNoMoment",
    ),
    detail: change && !withheld ? changeDetail(change, locale, t) : undefined,
  };
}

function handoffReading(
  open: IntroRequest | undefined,
  pending: boolean | undefined,
  unread: boolean | undefined,
  reader: Readonly<{ id: string; display_name: string }> | undefined,
  t: Translate,
): Part {
  return open
    ? {
        value: t(labelFor(HANDOFF_VALUE, open.status)),
        detail: handoffDetail(open, reader, t),
      }
    : { value: t(noAskValue(pending, unread)) };
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

// Total over the four kinds the contract admits, so a change the server can
// derive cannot reach a reader as a raw enum.
const CHANGE_VALUE: Record<RelationshipChange["kind"], MessageKey> = {
  replied_after_gap: "contact.intro.change.replied",
  went_quiet: "contact.intro.change.quiet",
  warmed: "contact.intro.change.warmed",
  cooled: "contact.intro.change.cooled",
};

// What the handoff slot says when no ask is in flight, or none is known yet.
function noAskValue(
  pending: boolean | undefined,
  unread: boolean | undefined,
): MessageKey {
  if (pending) {
    return "common.loading";
  }
  return unread
    ? "contact.intro.handoffUnread"
    : "contact.intro.handoffNotStarted";
}

import type { components } from "../../api/schema";
import type { StandingPeek } from "../../design-system/standingdims";
import { omitted } from "../../design-system/surfacestate";
import { formatMoneyCompact } from "../../format/format";
import { formatDayMonth } from "../../format/format.dates";
import { routeHash } from "../../lib/router";
import { type PeekWords, partialFoot } from "../../lib/standingpeeks";

type Company360 = components["schemas"]["Company360"];

/**
 * Who the Relationship row counts: the contacts here the server rates as having
 * interacted at all (its own rule for "in touch"), newest first. A
 * contact list cut at a page says so instead of passing for the whole set.
 */
export function contactsInTouch(
  view: Company360,
  words: PeekWords,
): StandingPeek | undefined {
  if (omitted(view, "contacts")) {
    return undefined;
  }
  const { t, locale, zone } = words;
  const touched = (view.contacts?.data ?? [])
    .flatMap((contact) =>
      contact.strength.last_interaction
        ? [{ contact, last: contact.strength.last_interaction }]
        : [],
    )
    .sort((a, b) => Date.parse(b.last) - Date.parse(a.last));
  if (touched.length === 0) {
    return undefined;
  }
  return {
    on: "sub",
    head: t("co.standing.peek.inTouch"),
    items: touched.map(({ contact, last }) => ({
      key: contact.contact_id,
      title: contact.full_name,
      sub: [
        contact.title ?? contact.provider_title,
        t("standing.peek.lastInTouch", {
          date: formatDayMonth(last, locale, zone),
        }),
      ]
        .filter(Boolean)
        .join(" · "),
      href: routeHash({ screen: "contacts", id: contact.contact_id }),
    })),
    foot: partialFoot(
      touched.length,
      view.health?.active_contacts ?? touched.length,
      words,
    ),
  };
}

/**
 * Which deals the Commercial row and the Open deals figure count. That is
 * every open deal on the account, with its stage, close date, amount and
 * whether it has stalled. A deal nobody priced says so instead of showing
 * nothing.
 */
export function openDeals(
  view: Company360,
  words: PeekWords,
): StandingPeek | undefined {
  if (omitted(view, "deals")) {
    return undefined;
  }
  const { t, locale, zone } = words;
  const open = (view.deals?.data ?? []).filter(
    (deal) => deal.status === "open",
  );
  if (open.length === 0) {
    return undefined;
  }
  return {
    on: "sub",
    head: t("co.standing.peek.openDeals"),
    items: open.map((deal) => ({
      key: deal.deal_id,
      title: deal.name,
      sub: [
        deal.stage_name,
        deal.stalled ? t("deal.stalledBadge") : undefined,
        deal.expected_close_date
          ? t("co.standing.peek.closes", {
              date: formatDayMonth(deal.expected_close_date, locale, zone),
            })
          : undefined,
      ]
        .filter(Boolean)
        .join(" · "),
      right:
        deal.amount?.amount_minor != null && deal.amount.currency
          ? formatMoneyCompact(
              deal.amount.amount_minor,
              deal.amount.currency,
              locale,
            )
          : t("co.strip.unpriced"),
      href: routeHash({ screen: "deals", id: deal.deal_id }),
    })),
    foot: partialFoot(
      open.length,
      view.state_strip?.commercial?.open_count ?? open.length,
      words,
    ),
  };
}

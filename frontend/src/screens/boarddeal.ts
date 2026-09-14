// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The board's view model: what a deal ROW becomes on a CARD.
//
// The card takes a model rather than the wire row, so every fact it states has
// to be copied across here — a field left out is silently absent on every card
// in every column, which is the failure the tests beside this file exist for.

import type { components } from "../api/schema";
import { routeHash } from "../app/router";
import type { BoardDeal, BoardDealMail } from "../design-system/composed";
import { idleSince } from "../format/idlebase";
import type { CompanyNaming } from "./deals";
import type { OwnerNaming } from "./entityref";

type Deal = components["schemas"]["Deal"];

/**
 * What a deal's company reads as on its card, in the four readings it has.
 *
 * Withheld carries the mask, the same control the table's company cell draws. A
 * company the screen has a name for is named. A company whose read FAILED says
 * so, because a deal that names a company the reader could not fetch is not a
 * deal with no company. Only a deal naming no company draws nothing.
 *
 * A name still in flight also draws nothing rather than a uuid: the card's
 * company line is a name a reader recognises and an id is not one. That is the
 * one case where an empty slot is a wait rather than a claim, and it resolves
 * itself.
 */
function dealCompany(
  deal: Deal,
  naming: CompanyNaming,
): Pick<
  BoardDeal,
  | "company"
  | "companyHref"
  | "companyLogoUrl"
  | "companyWithheld"
  | "companyUnreadable"
> {
  if (deal.masked_fields?.includes("company_id")) {
    return { company: "", companyWithheld: true };
  }
  if (deal.company_id && naming.unreadable.has(deal.company_id)) {
    return { company: "", companyUnreadable: true };
  }
  const mark = deal.company_id ? naming.marks.get(deal.company_id) : undefined;
  return {
    company: mark?.name ?? "",
    // The company's address, built HERE because this is the tier that holds
    // routes. A deal with no company, or one whose name has not resolved,
    // gets none — the card then draws prose, which is what a slot with no
    // name has to say anyway.
    companyHref:
      deal.company_id && mark?.name
        ? routeHash({ screen: "companies", id: deal.company_id })
        : undefined,
    companyLogoUrl: mark?.logoUrl,
  };
}

export function toBoardDeal(
  deal: Deal,
  naming: CompanyNaming,
  owners?: OwnerNaming,
): BoardDeal {
  const since = idleSince(deal);
  return {
    id: deal.id,
    name: deal.name,
    ...dealCompany(deal, naming),
    // Both halves as the wire sent them. Nobody has priced every deal, and a
    // card that filled in either half would state a figure this deal does not
    // have — a zero amount, or a euro sign over an unknown currency.
    valueMinor: deal.amount_minor ?? null,
    currency: deal.currency ?? null,
    ageMs: Math.max(0, Date.now() - new Date(since).getTime()),
    stalled: deal.stalled ?? false,
    archived: deal.archived_at != null,
    closeDate: deal.expected_close_date ?? null,
    closeDateProvisional: deal.close_date_provisional ?? false,
    owner: owners?.(deal.owner_id) ?? null,
    lastEmail: boardMail(deal.last_email),
  };
}

/**
 * The wire's last email as the card states it: a span and a direction. The
 * span is measured HERE, where every other span on the card is — a card that
 * read the clock itself would be one no test could pin. Exported for the
 * table's column, which states the same chip off the same row.
 */
export function boardMail(last: Deal["last_email"]): BoardDealMail | null {
  if (!last) {
    return null;
  }
  return {
    agoMs: Math.max(0, Date.now() - new Date(last.occurred_at).getTime()),
    direction: last.direction ?? null,
  };
}

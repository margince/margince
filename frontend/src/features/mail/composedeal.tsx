// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { components } from "../../api/schema";
import { Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { CommunicationContext } from "../../lib/compose-context";
import { contact360Query } from "../../lib/contact.queries";
import type { RelinkKind } from "./composerelink";
import type { MissingField } from "./composesend";

type DealSeat = components["schemas"]["Contact360DealRole"];

/** One open deal a message to a contact can be about. */
export type DealChoice = Readonly<{ id: string; name: string }>;

// The deals a follow-up to this contact can rest on, once each: an open deal
// on which the contact's seat has not ended. That is the engine's own live
// stakeholder rule (consent liveStakeholderSQL), so a deal offered here is one
// the check can read. A deal with two seats for one contact is still one deal.
export function dealChoicesOf(seats: readonly DealSeat[]): DealChoice[] {
  const choices: DealChoice[] = [];
  for (const seat of seats) {
    if (seat.deal_status !== "open" || seat.seat_current !== true) continue;
    if (choices.some((choice) => choice.id === seat.deal_id)) continue;
    choices.push({ id: seat.deal_id, name: seat.deal_title ?? "" });
  }
  return choices;
}

/**
 * Whether a new message to a contact shows which deal it is about.
 *
 * The deal page names its own deal and the account path picks one beside the
 * subject, so only a contact's composer asks here. It asks once Active deal is
 * the reason, because that is the claim a deal can prove. A deal already
 * chosen stays on screen, so a filing the rep cannot see never travels.
 */
export function offersDealChoice(where: {
  entityType: RelinkKind;
  replying: boolean;
  context: CommunicationContext | "";
  dealId: string;
}): boolean {
  if (where.entityType !== "contact" || where.replying) return false;
  return where.context === "active_deal_followup" || where.dealId !== "";
}

/**
 * Why the list of deals may be incomplete. `loading` and `unavailable` know
 * nothing yet; `partial` lists the first page of seats and not the rest.
 */
export type DealListGap = "loading" | "unavailable" | "partial" | null;

/**
 * The deal choice of one composer: whether it shows, what it offers, the deal
 * the message files under, and whether Send waits for it.
 *
 * On a contact's composer the deal travels only while it is among the deals
 * offered. A restored deal the contact no longer sits on is dropped, and the
 * rep chooses again. A list that is not known yet holds Send on an Active deal
 * claim, because an unknown list is not an empty one.
 */
export function useDealChoice(where: {
  open: boolean;
  entityType: RelinkKind;
  entityId: string;
  replying: boolean;
  context: CommunicationContext | "";
  dealId: string;
}) {
  const offered = offersDealChoice(where);
  // The contact page's own read, so the composer costs no request there.
  const view = useQuery({
    enabled: where.open && offered,
    ...contact360Query(where.entityId),
  });
  const seats = view.data?.deal_roles;
  const choices = dealChoicesOf(seats?.data ?? []);
  const gap = dealListGap(view, seats?.page.has_more === true);
  const dealId =
    where.entityType !== "contact" ||
    choices.some((deal) => deal.id === where.dealId)
      ? where.dealId
      : "";
  const needsChoice =
    offered &&
    where.context === "active_deal_followup" &&
    dealId === "" &&
    (choices.length > 0 || gap !== null);
  return { offered, choices, gap, dealId, needsChoice };
}

function dealListGap(
  view: { isPending: boolean; isError: boolean },
  hasMore: boolean,
): DealListGap {
  if (view.isError) return "unavailable";
  if (view.isPending) return "loading";
  return hasMore ? "partial" : null;
}

export type DealChoiceState = ReturnType<typeof useDealChoice>;

// Which deal the message is about, asked beside the reason it is written for.
// Choosing one files the message under it and asks the check again; it never
// sends.
export function ComposeDeal({
  choice,
  onDealChange,
  flagged,
}: Readonly<{
  choice: DealChoiceState;
  onDealChange: (next: string) => void;
  flagged: ReadonlySet<MissingField>;
}>) {
  const t = useT();
  if (!choice.offered || (choice.choices.length === 0 && choice.gap === null)) {
    return null;
  }
  const options = [
    { value: "", label: t("compose.aboutDealNone") },
    ...choice.choices.map((deal) => ({
      value: deal.id,
      label: t("compose.relatedDeal", { name: deal.name }),
    })),
  ];
  return (
    <div className="compose-strip">
      <p className="compose-strip-text">{t("compose.aboutDeal")}</p>
      <Field
        className="compose-strip-field"
        label={t("compose.aboutDeal")}
        labelHidden
        hint={gapHint(choice.gap, t)}
        error={flagged.has("deal") ? t("compose.missingDeal") : undefined}
      >
        {(control) => (
          <Select
            {...control}
            options={options}
            value={choice.dealId}
            disabled={choice.gap === "loading"}
            onChange={onDealChange}
          />
        )}
      </Field>
    </div>
  );
}

function gapHint(
  gap: DealListGap,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (gap === "unavailable") return t("compose.aboutDealUnavailable");
  if (gap === "partial") return t("compose.aboutDealPartial");
  return undefined;
}

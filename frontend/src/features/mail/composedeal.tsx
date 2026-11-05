// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Field } from "../../design-system/atoms";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { CommunicationContext } from "../../lib/compose-context";
import { useContact360 } from "../contacts/contact360";
import type { RelinkKind } from "./composerelink";
import type { MissingField } from "./composesend";

type DealSeat = components["schemas"]["Contact360DealRole"];

/** One open deal a message to a contact can be about. */
export type DealChoice = Readonly<{ id: string; name: string }>;

// The contact's open deals, once each. A seat on a closed deal is history, and
// a deal with two seats for one contact is still one deal.
export function dealChoicesOf(seats: readonly DealSeat[]): DealChoice[] {
  const choices: DealChoice[] = [];
  for (const seat of seats) {
    if (seat.deal_status !== "open") continue;
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
 * The deal choice of one composer: whether it shows, what it offers, and
 * whether Send waits for it. Send waits only where an Active deal claim has a
 * deal to choose and none is chosen.
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
  const view = useContact360(where.entityId, where.open && offered);
  const choices = dealChoicesOf(view.data?.deal_roles?.data ?? []);
  const needsChoice =
    offered &&
    where.context === "active_deal_followup" &&
    where.dealId === "" &&
    choices.length > 0;
  return { offered, choices, needsChoice };
}

export type DealChoiceState = ReturnType<typeof useDealChoice>;

// Which deal the message is about, asked beside the reason it is written for.
// Choosing one files the message under it and asks the check again; it never
// sends.
export function ComposeDeal({
  choice,
  dealId,
  onDealChange,
  flagged,
}: Readonly<{
  choice: DealChoiceState;
  dealId: string;
  onDealChange: (next: string) => void;
  flagged: ReadonlySet<MissingField>;
}>) {
  const t = useT();
  if (!choice.offered || (choice.choices.length === 0 && dealId === "")) {
    return null;
  }
  // A restored deal the contact no longer sits on is still named, so the rep
  // sees what the message is filed under before choosing another.
  const kept = choice.choices.some((deal) => deal.id === dealId);
  const options = [
    { value: "", label: t("compose.aboutDealNone") },
    ...choice.choices.map((deal) => ({
      value: deal.id,
      label: t("compose.relatedDeal", { name: deal.name }),
    })),
    ...(dealId !== "" && !kept
      ? [{ value: dealId, label: t("compose.aboutDealKept") }]
      : []),
  ];
  return (
    <div className="compose-strip">
      <p className="compose-strip-text">{t("compose.aboutDeal")}</p>
      <Field
        className="compose-strip-field"
        label={t("compose.aboutDeal")}
        labelHidden
        error={flagged.has("deal") ? t("compose.missingDeal") : undefined}
      >
        {(control) => (
          <Select
            {...control}
            options={options}
            value={dealId}
            onChange={onDealChange}
          />
        )}
      </Field>
    </div>
  );
}

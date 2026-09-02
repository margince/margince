// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The three facts a rep checks before anything else: what it is worth, where
// it sits on the board, and whose deal it is.
//
// They were not readable together. The amount was in the subtitle AND in the
// readings strip, the stage was only the chip row below the fold, and the
// owner was on no part of the page at all — so "whose deal is this" could not
// be answered without opening Edit. This is RecordView's `controls` slot, the
// same one the company record draws its own facts box in (companyfacts.tsx),
// so the two records read the same way.
//
// Read-only on purpose. Stage and owner are both writable through Edit deal,
// and a control here that looked editable but only navigated would be worse
// than a label; making them editable in place is a deliberate change to how
// the deal is written, not a side effect of showing it.

import { FieldGuard } from "../../design-system/rbac";
import { formatMoney } from "../../format/format";
import type { Locale } from "../../i18n";
import { useT } from "../../i18n";
import { rosterOwnerName, useRoster, useRosterPartial } from "../entityref";
import "./dealfacts.css";

type Deal = {
  amount_minor?: number | null;
  currency?: string | null;
  stage_id?: string | null;
  owner_id?: string | null;
  masked_fields?: readonly string[];
};

type Stage = { id: string; name: string };

/**
 * DealFacts is the deal's standing: value, stage, owner.
 *
 * The stage NAME comes from the pipeline the page has already read, not a
 * second request — the caller passes the stages it sorted for the chip row.
 * The owner name comes from the shared roster the rest of the app resolves
 * users through, so it hits the same cache entry every EntityRef does.
 */
export function DealFacts({
  deal,
  stages,
  locale,
}: Readonly<{
  deal: Deal;
  stages: readonly Stage[];
  locale: Locale;
}>) {
  const t = useT();
  // Only asked for when there is an owner to name: an unowned deal needs no
  // roster read to say so.
  const roster = useRoster("user", Boolean(deal.owner_id));
  const partial = useRosterPartial("user", Boolean(deal.owner_id));
  const masked = deal.masked_fields ?? [];
  const stage = stages.find((s) => s.id === deal.stage_id);
  return (
    <dl className="deal-facts">
      <div className="deal-facts-item">
        <dt>{t("deals.amount")}</dt>
        <dd>{amount(deal, masked, locale)}</dd>
      </div>
      <div className="deal-facts-item">
        <dt>{t("deals.stage")}</dt>
        {/* An em dash rather than the stage id: a deal in overlay mode carries
            no native pipeline row, and printing a UUID where a stage name goes
            reads as a fault. */}
        <dd>{stage?.name ?? "—"}</dd>
      </div>
      <div className="deal-facts-item">
        <dt>{t("list.owner")}</dt>
        <dd>
          {rosterOwnerName(
            deal.owner_id,
            roster,
            partial,
            t,
            t("co.pulse.unowned"),
          )}
        </dd>
      </div>
    </dl>
  );
}

// The value, or the reason it is not shown. A rep who may read the deal but
// not its amount sees the field named and masked, which is what the subtitle
// did before this box took the amount over.
function amount(
  deal: Deal,
  masked: readonly string[],
  locale: Locale,
): React.ReactNode {
  if (masked.includes("amount_minor")) {
    return <FieldGuard mode="masked" />;
  }
  if (deal.amount_minor == null || !deal.currency) {
    return "—";
  }
  return formatMoney(deal.amount_minor, deal.currency, locale);
}

// Who owns it. An unowned deal says so rather than showing a blank, and a
// roster that has not answered says which of the three silences it is —
// still reading, read failed, or walked short of this user.

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatMoneyOrAbsent } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import type { MessageKey } from "../../../i18n/en";
import { labelFor } from "../../../lib/labelfor";
import type { DealCommercial } from "./deallines";

const MISSING_LABEL: Record<DealCommercial["missing"][number], MessageKey> = {
  lines: "deal.lines.missing.lines",
  quantity: "deal.lines.missing.quantity",
  unit_price: "deal.lines.missing.unitPrice",
  billing_model: "deal.lines.missing.billing",
  interval_count: "deal.lines.missing.term",
  tax_rate: "deal.lines.missing.taxRate",
};

/** What the deal's lines add up to, or why they do not yet. */
export function DealLinesTotal({ state }: Readonly<{ state: DealCommercial }>) {
  const t = useT();
  const { locale } = useLocale();
  if (state.money_withheld) {
    return <p className="t-caption">{t("deal.lines.withheld")}</p>;
  }
  if (state.complete) {
    return (
      <p className="t-body">
        {t("deal.lines.committedNet", {
          amount: formatMoneyOrAbsent(
            state.committed_net_minor,
            state.currency,
            locale,
          ),
        })}
      </p>
    );
  }
  return (
    <p className="t-caption">
      {t("deal.lines.incomplete", {
        known: formatMoneyOrAbsent(
          state.known_committed_net_minor,
          state.currency,
          locale,
        ),
        missing: state.missing
          .map((reason) => t(labelFor(MISSING_LABEL, reason)))
          .join(", "),
      })}
    </p>
  );
}

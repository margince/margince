// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EntityRef } from "../../components/entityref";
import { FieldGuard } from "../../design-system/rbac";
import { formatDate } from "../../format/format";
import { useT } from "../../i18n";
import type { Locale } from "../../i18n/locale";
import { MARGIN_TIER_LABELS, type Partner } from "./partners.vocab";

// Rows of the Partner tab's read view that say more than the stored value:
// a withheld tier, and when and how the partner was activated.

// Withheld is not empty. A role that may not read the commercial terms gets
// the tier as null with `masked_fields` naming it. A row that vanished would
// say this partner agreed no tier, so no row means only that none was agreed.
export function MarginTierRow({ partner }: Readonly<{ partner: Partner }>) {
  const t = useT();
  const masked = partner.masked_fields?.includes("margin_tier") ?? false;
  const tier = partner.margin_tier;
  if (!masked && !tier) {
    return null;
  }
  return (
    <>
      <dt>{t("partner.marginTier")}</dt>
      <dd>
        <FieldGuard mode={masked ? "masked" : "visible"}>
          {tier ? t(MARGIN_TIER_LABELS[tier]) : null}
        </FieldGuard>
      </dd>
    </>
  );
}

// When and how the partner was activated. A partner active before the stage
// list existed has no recorded date, so it says so rather than show one.
export function ActivationNote({
  partner,
  locale,
  zone,
}: Readonly<{ partner: Partner; locale: Locale; zone: string }>) {
  const t = useT();
  if (partner.activation_basis === "recorded_before_ladder") {
    return <span className="t-caption"> · {t("partner.activatedBefore")}</span>;
  }
  if (!partner.activated_at) {
    return null;
  }
  return (
    <span className="t-caption">
      {" · "}
      {t("partner.activated", {
        date: formatDate(partner.activated_at, locale, zone),
      })}
      {partner.activation_lead_id && (
        <>
          {" "}
          {t("partner.activatedThroughLead")}{" "}
          <EntityRef kind="lead" id={partner.activation_lead_id} />
        </>
      )}
      {partner.activation_deal_id && (
        <>
          {" "}
          {t("partner.activatedThroughDeal")}{" "}
          <EntityRef kind="deal" id={partner.activation_deal_id} />
        </>
      )}
    </span>
  );
}

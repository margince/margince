// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEntityName } from "../../components/entityref";
import { AppLink } from "../../design-system/applink";
import { FieldGuard } from "../../design-system/rbac";
import { useT } from "../../i18n";
import { routeHash } from "../../lib/router";
import type { Deal } from "./deals";
import { useNoPartnersYet } from "./deals.fields";

type DealPartnerFacts = Pick<
  Deal,
  "partner_company_id" | "partner_attribution" | "masked_fields"
>;

// The partner and what it did: a sourced deal earns commission, an influenced
// one does not, so the two must not read alike. No partner is an answer, and
// where no company is a partner yet the answer says how to get one.
export function DealPartner({ deal }: Readonly<{ deal: DealPartnerFacts }>) {
  const t = useT();
  const name = useEntityName("company", deal.partner_company_id).name;
  const noneYet = useNoPartnersYet();
  if (deal.masked_fields?.includes("partner_company_id")) {
    return <FieldGuard mode="masked" />;
  }
  if (!deal.partner_company_id) {
    return <>{t(noneYet ? "deal.partnerNoneYet" : "deal.aside.partnerNone")}</>;
  }
  const role =
    deal.partner_attribution === "sourced"
      ? t("deal.attributionSourced")
      : deal.partner_attribution === "influenced"
        ? t("deal.attributionInfluenced")
        : undefined;
  return (
    <AppLink
      className="deal-aside-link"
      href={routeHash({ screen: "companies", id: deal.partner_company_id })}
      title={role}
    >
      {[name ?? t("field.unset"), role].filter(Boolean).join(" · ")}
    </AppLink>
  );
}

/**
 * The partner as one line under the deal's name, or nothing when there is
 * none. Only a sourced deal reads as referred; an unset attribution counts as
 * sourced, because the server counts it so.
 */
export function DealReferralLine({
  deal,
}: Readonly<{ deal: DealPartnerFacts }>) {
  const t = useT();
  const name = useEntityName("company", deal.partner_company_id).name;
  if (
    !deal.partner_company_id ||
    deal.masked_fields?.includes("partner_company_id")
  ) {
    return null;
  }
  const influenced = deal.partner_attribution === "influenced";
  return (
    <div className="deal-project-line">
      {t(influenced ? "deal.supportedBy" : "deal.referredBy")}{" "}
      <AppLink
        href={routeHash({ screen: "companies", id: deal.partner_company_id })}
      >
        {name ?? t("field.unset")}
      </AppLink>{" "}
      {t(influenced ? "deal.referral.influenced" : "deal.referral.sourced")}
    </div>
  );
}

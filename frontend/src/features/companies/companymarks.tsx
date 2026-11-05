// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { type Translator, useT } from "../../i18n";
import {
  partnerKindsLabel,
  partnerStageLabel,
} from "../../lib/partnerstages.queries";
import { RecordAccess } from "../records/recordaccess";
import { CompanyNameLine } from "./companylifecycle";

type Company = components["schemas"]["Company"];

/**
 * The pills under an account's name: whether it is archived, where it stands
 * (the lifecycle, the head's one real control) and who may read it. Archived
 * leads because it outranks the rest: among the verbs it read as one more
 * control, and a reader scanning the name missed it.
 */
export function CompanyMarks({ company }: Readonly<{ company: Company }>) {
  const t = useT();
  return (
    <>
      {company.archived_at && (
        <Badge tone="warning">{t("record.archived")}</Badge>
      )}
      <CompanyNameLine company={company} />
      <CompanyRoleMarks company={company} />
      <RecordAccess key={company.id} kind="company" record={company} />
    </>
  );
}

/**
 * The partner mark's words: its kinds and stage when the reader may read the
 * programme, and the bare word when the company list sent none.
 */
export function partnerMarkText(company: Company, t: Translator): string {
  const partner = company.partner;
  if (!partner) {
    return t("company.relType.partner");
  }
  return [
    t("company.relType.partner"),
    partnerKindsLabel(partner.partner_kinds, t),
    partnerStageLabel(partner, t),
  ]
    .filter((part) => part !== "")
    .join(" · ");
}

/**
 * What the company is to the business: a customer, a partner, or both on one
 * record. Customer reads the relationship type, not the lifecycle. A partner
 * that also buys from us stays a customer whatever stage it is in. The mark is
 * left out where the lifecycle chip already names the customer relationship.
 * So the head never reads "Customer" twice, or "Former customer" against it.
 */
export function CompanyRoleMarks({ company }: Readonly<{ company: Company }>) {
  const t = useT();
  const types = company.relationship_types ?? [];
  const isPartner = types.includes("partner") || company.partner !== undefined;
  const lifecycleSaysCustomer =
    company.lifecycle === "customer" || company.lifecycle === "former_customer";
  return (
    <>
      {types.includes("customer") && !lifecycleSaysCustomer && (
        <Badge tone="success">{t("company.relType.customer")}</Badge>
      )}
      {isPartner && (
        <Badge tone="accent" wrap>
          {partnerMarkText(company, t)}
        </Badge>
      )}
    </>
  );
}

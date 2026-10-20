// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { IdentityLine } from "../../design-system/identityline";
import { useT } from "../../i18n";
import { RecordAccess } from "../records/recordaccess";
import { CompanyRelationshipBadges } from "./companyheader";

type Company = components["schemas"]["Company"];

/**
 * The row under an account's facts line: whether it is archived, what it is
 * to us, and who may read it. The lifecycle sits beside the name instead, as
 * the header's one real control (CompanyLifecycleControl). As a pill in this
 * row it read as a tag a reader could not act on. Archived leads because it
 * outranks the rest. The contact page's `ContactMarks` in the account's own
 * terms.
 */
export function CompanyMarks({ company }: Readonly<{ company: Company }>) {
  const t = useT();
  return (
    <IdentityLine separator="space">
      {company.archived_at && (
        <Badge tone="warning">{t("record.archived")}</Badge>
      )}
      <CompanyRelationshipBadges company={company} />
      <RecordAccess key={company.id} kind="company" record={company} />
    </IdentityLine>
  );
}

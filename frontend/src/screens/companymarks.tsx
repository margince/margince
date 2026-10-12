// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { IdentityLine } from "../design-system/identityline";
import { useT } from "../i18n";
import { CompanyRelationshipBadges } from "./companyheader";
import { CompanySubtitle } from "./companyheaderfacts";
import { RecordAccess } from "./recordaccess";

type Company = components["schemas"]["Company"];

/**
 * The row under an account's name: whether it is archived, what it is and the
 * way in (CompanySubtitle), what it IS to us, and who may read it. Where it
 * STANDS is not among them: the lifecycle is the header's one real control and
 * sits beside the name (CompanyLifecycleControl), because as a pill in this
 * row it read as one more tag a reader could not act on. The contact page's
 * `ContactMarks` in the account's own terms. Archived leads because it
 * outranks the rest: among the verbs it read as one more control, and a reader
 * scanning the name missed it.
 */
export function CompanyMarks({ company }: Readonly<{ company: Company }>) {
  const t = useT();
  return (
    <IdentityLine separator="space">
      {company.archived_at && (
        <Badge tone="warning">{t("record.archived")}</Badge>
      )}
      <CompanySubtitle company={company} />
      <CompanyRelationshipBadges company={company} />
      <RecordAccess key={company.id} kind="company" record={company} />
    </IdentityLine>
  );
}

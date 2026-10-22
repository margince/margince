// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { useT } from "../../i18n";
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
      <RecordAccess key={company.id} kind="company" record={company} />
    </>
  );
}

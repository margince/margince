// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { useCompanyReadOnlyReason } from "./companyheader";
import { TagsPanel } from "./tagspanel";

type Company = components["schemas"]["Company"];

/**
 * The company's tags, drawn by the SHARED panel.
 *
 * What stays here is the mount: the write gate needs a resolved Company to
 * ask `useCompanyReadOnlyReason` about, so the wrapper waits for one rather
 * than calling the hook conditionally.
 */
export function CompanyTagsSection({
  company,
  companyId,
  bare = false,
}: Readonly<{ company?: Company; companyId: string; bare?: boolean }>) {
  if (!company) {
    return null;
  }
  return <CompanyTags company={company} companyId={companyId} bare={bare} />;
}

function CompanyTags({
  company,
  companyId,
  bare,
}: Readonly<{ company: Company; companyId: string; bare: boolean }>) {
  // useCanWriteRecord, not useCan: applying a tag is a WRITE to the record, so
  // it owes the same three axes every other company control derives — the
  // object grant, the licensing seat, and the server's own `writable` for this
  // row. `useCompanyReadOnlyReason` adds the reason worth SAYING (archived, or
  // an overlay installation); it deliberately stays quiet about an ownerless
  // record, which `writable` is what answers.
  const canUpdate = useCanWriteRecord("company", company);
  const readOnlyReason = useCompanyReadOnlyReason(company);
  const canEdit = canUpdate && !readOnlyReason;

  return (
    <TagsPanel
      entityType="company"
      entityID={companyId}
      canEdit={canEdit}
      bare={bare}
    />
  );
}

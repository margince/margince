// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { useCompanyReadOnlyReason } from "./companyheader";
import { TagsPanel } from "./tagspanel";

type Organization = components["schemas"]["Organization"];

/**
 * The company's tags, drawn by the SHARED panel.
 *
 * What stays here is the mount: the write gate needs a resolved Organization to
 * ask `useCompanyReadOnlyReason` about, so the wrapper waits for one rather
 * than calling the hook conditionally.
 */
export function CompanyTagsSection({
  organization,
  orgId,
  bare = false,
}: Readonly<{ organization?: Organization; orgId: string; bare?: boolean }>) {
  if (!organization) {
    return null;
  }
  return <CompanyTags organization={organization} orgId={orgId} bare={bare} />;
}

function CompanyTags({
  organization,
  orgId,
  bare,
}: Readonly<{ organization: Organization; orgId: string; bare: boolean }>) {
  // useCanWriteRecord, not useCan: applying a tag is a WRITE to the record, so
  // it owes the same three axes every other company control derives — the
  // object grant, the licensing seat, and the server's own `writable` for this
  // row. `useCompanyReadOnlyReason` adds the reason worth SAYING (archived, or
  // an overlay installation); it deliberately stays quiet about an ownerless
  // record, which `writable` is what answers.
  const canUpdate = useCanWriteRecord("organization", organization);
  const readOnlyReason = useCompanyReadOnlyReason(organization);
  const canEdit = canUpdate && !readOnlyReason;

  return (
    <TagsPanel
      entityType="organization"
      entityID={orgId}
      canEdit={canEdit}
      bare={bare}
    />
  );
}

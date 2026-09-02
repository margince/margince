// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useCan } from "../app/capability";
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
}: Readonly<{ organization?: Organization; orgId: string }>) {
  if (!organization) {
    return null;
  }
  return <CompanyTags organization={organization} orgId={orgId} />;
}

function CompanyTags({
  organization,
  orgId,
}: Readonly<{ organization: Organization; orgId: string }>) {
  const readOnlyReason = useCompanyReadOnlyReason(organization);
  // The two questions a MOUNT can answer: the object grant, and this row's own
  // writability. `useCompanyReadOnlyReason` answers only the row — its own
  // comment says every mount ANDs it with the grant. The third question, whether
  // the vocabulary is visible at all, belongs to the panel: it draws the
  // withheld state instead of the words, and its verb never reaches that path.
  const canUpdate = useCan("organization", "update");
  const canEdit = canUpdate && !readOnlyReason;

  return (
    <TagsPanel entityType="organization" entityID={orgId} canEdit={canEdit} />
  );
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useState } from "react";

import type { components } from "../api/schema";
import { useCompanyReadOnlyReason } from "./companyheader";
import { AddTagDialog } from "./tagpicker";
import { useRecordTags } from "./tags.queries";
import { AddTagButton, TagsPanel } from "./tagspanel";
import "./company360.css";

type Organization = components["schemas"]["Organization"];

/**
 * The company's tags, drawn by the SHARED panel.
 *
 * This file used to hold a panel of its own, reading the tag names off the
 * account's 360 response. That block carried no assigner, so the menu could
 * not say who applied a word — and a person and a deal had no panel at all.
 * One component for all three answers those together.
 *
 * What stays here is the mount: the add-tag verb needs a resolved
 * Organization to ask `useCompanyReadOnlyReason` about, so the wrapper waits
 * for one rather than calling the hook conditionally.
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
  const [adding, setAdding] = useState(false);
  const read = useRecordTags("organization", orgId);
  const canEdit = !readOnlyReason;

  return (
    <>
      <TagsPanel entityType="organization" entityID={orgId} canEdit={canEdit} />
      {canEdit && (
        <div className="co-card-actions">
          <AddTagButton onOpen={() => setAdding(true)} />
        </div>
      )}
      {adding && (
        <AddTagDialog
          entityType="organization"
          entityID={orgId}
          current={read.data?.data ?? []}
          onClose={() => setAdding(false)}
        />
      )}
    </>
  );
}

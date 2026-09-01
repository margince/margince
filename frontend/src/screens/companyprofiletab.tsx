// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useId } from "react";
import type { components } from "../api/schema";
import { useCanWriteRecord } from "../app/capability";
import { Callout } from "../design-system/callout";
import { FieldGrid } from "../design-system/fieldgrid";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { CompanyFactsPanel } from "./companyfactspanel";
import { useCompanyReadOnlyReason } from "./companyheader";
import { DetailsGrid, SidecarFieldRow } from "./companyraildetails";
import { useOrgProfileFields } from "./evidenceverdict";
import { profileFieldLabel } from "./organizations";

type Organization = components["schemas"]["Organization"];
type ProfileFieldKey = components["parameters"]["ProfileFieldKey"];

// The account's own story, in the order a rep reads it: what the company
// sells, who it sells to, and how the sale actually happens. These eleven
// fields have no column on `organization` — the profile-field row IS the
// record of them — so every one is written through the same PATCH the rail's
// registration rows use.
//
// They were previously read-only: the tab listed whichever the crawl had
// found and offered a "correct" button beside each, which meant a field the
// crawl never produced could not be entered at all. The row is now the same
// editable row the rail draws, so an empty field reads as an invitation
// rather than as an absence with no remedy.
const NARRATIVE_FIELDS = [
  "offer_summary",
  "icp",
  "buying_center",
  "value_proposition",
  "usp",
  "customer_pains",
  "desired_outcomes",
  "buying_intents",
  "common_objections",
  "sales_motion",
  "history",
] as const satisfies readonly ProfileFieldKey[];

/**
 * CompanyProfileForm is the Profile tab's own body: the account's details as
 * one form, rather than the four collapsed sections this tab used to be.
 *
 * WHY A FORM AND NOT DISCLOSURES. Every section here describes the same
 * subject — this company, as the record holds it — so a reader looking for
 * one field had to guess which of four folds it was behind, open it, and
 * find that half the fields on the tab were not editable there anyway. The
 * genuinely editable ones lived in the record rail two tabs away. A record's
 * own fields belong in one place that can be read top to bottom.
 *
 * WRITE STATE IS DERIVED ONCE, HERE. `useCanWriteRecord` folds the three
 * independent axes — the object grant, the seat ceiling, and the server's own
 * per-row `writable` — where the tab previously consulted only the first.
 * A rep who cannot write a colleague's account was still shown every verb,
 * and found out at the save. The reason sentence is rendered ONCE at the top
 * and every refused control points at it by id, rather than each control
 * carrying its own copy: withholding twelve buttons individually is noise,
 * and withholding the page's one explanation is the defect.
 */
export function CompanyProfileForm({
  org,
  onOpenHistory,
  tools,
}: Readonly<{
  org: Organization;
  // Opens the record's own history drawer, for a reader following an evidence
  // mark back to what changed.
  onOpenHistory?: () => void;
  // The account's own tooling — custom fields, group rollup, the site read,
  // the technical profile. Passed in rather than built here: they are the
  // caller's existing cards and this file has no business knowing what is in
  // the set.
  tools: ReactNode;
}>): ReactNode {
  const t = useT();
  const reasonId = useId();
  const writable = useCanWriteRecord("organization", org);
  // The archived/overlay/not-yours sentence, when there is one. It is the
  // more specific answer, so it wins over the generic refusal below whenever
  // it applies.
  const specificReason = useCompanyReadOnlyReason(org);
  // Every denial owes the reader a sentence. `useCanWriteRecord` also refuses
  // a read seat and a missing grant, neither of which the specific reason
  // knows about, and a control pointing at an id that renders nothing is a
  // refusal the reader cannot resolve.
  const reason = writable
    ? undefined
    : (specificReason ?? t("record.notYoursToChange"));
  const canEdit = writable && !specificReason;
  const profileQuery = useOrgProfileFields(org.id);
  const fields = profileQuery.data ?? [];

  return (
    <div className="record-stack">
      {reason && (
        <Callout tone="info">
          <p id={reasonId}>{reason}</p>
        </Callout>
      )}
      <Panel title={t("co.details.title")}>
        <PanelBody>
          <DetailsGrid organization={org} />
        </PanelBody>
      </Panel>
      <Panel title={t("co.narrative.title")} sub={t("co.narrative.sub")}>
        <PanelBody>
          <FieldGrid>
            {NARRATIVE_FIELDS.map((field) => (
              <SidecarFieldRow
                key={field}
                orgId={org.id}
                fields={fields}
                // A pending or failed read is not the same claim as "this
                // field has no row": editing on that guess sends no If-Match
                // and overwrites a value the reader never saw.
                fieldsLoaded={profileQuery.isSuccess}
                field={field}
                // The label comes from the ONE vocabulary map
                // (PROFILE_FIELD_LABELS), not a second list here: a field
                // renamed there must not keep its old name on this tab.
                label={profileFieldLabel(field, t)}
                placeholderKey="co.narrative.add"
                canEdit={canEdit}
                readOnlyReason={reason}
                multiline
              />
            ))}
          </FieldGrid>
        </PanelBody>
      </Panel>
      {/* Facts are their own panel: many-valued, correctable one row at a
          time, and now addable and removable — which is a different act from
          stating a field, and needs the same write state this form derived. */}
      <CompanyFactsPanel
        orgId={org.id}
        canEdit={canEdit}
        reasonId={reason ? reasonId : undefined}
        onOpenHistory={onOpenHistory}
      />
      {tools}
    </div>
  );
}

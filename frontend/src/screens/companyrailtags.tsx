// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { Panel, PanelBody } from "../design-system/panel";
import { SurfaceState, sectionState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { TagAction } from "./companyactions";
import { useCompanyReadOnlyReason } from "./companyheader";
import { sectionAnswered } from "./companyrailshared";
// The row and card shapes this file draws — co-rowlink, co-row-meta, co-card —
// are defined in company360.css. Imported HERE rather than left to the caller:
// it works today only because the company record page pulls that stylesheet in
// for its own sake, so this file renders unstyled anywhere else.
import "./company360.css";

// TagsSection (companyrail.tsx's own section, split into this file so the
// rail stays under the 500-line ceiling): the tags applied to this account,
// plus the verb that writes them.

type Organization360 = components["schemas"]["Organization360"];
type Organization = components["schemas"]["Organization"];

/**
 * TagsSection shows the tags applied to this account and the verb that adds
 * one. It states its own withheld/empty/ready: a caller who may not read tags
 * sees the withheld notice, never a confirmed-empty account.
 *
 * The add-tag verb lives HERE, under the tags it writes, rather than in a
 * separate strip elsewhere on the page (CompanyFilingActions,
 * organizations.tsx — retired once this landed): reading and writing the same
 * fact from two different columns is the confusion this section exists to
 * close. The verb keeps the gate the old strip enforced — rendered only once
 * the section has answered (ready or empty) — so a caller who cannot read
 * tags is never offered a button to add one, and a section that failed to
 * load cannot say whether the write makes sense.
 */
export function TagsSection({
  view,
  orgId,
  loading,
}: Readonly<{ view?: Organization360; orgId: string; loading: boolean }>) {
  const t = useT();
  const { locale } = useLocale();
  const tags = view?.tags ?? [];
  const tagState = sectionState(
    view,
    "tags",
    Boolean(view?.tags),
    tags.length,
    loading,
  );
  const tagsAnswered = sectionAnswered(tagState);
  // Absent, not zero, until the section has actually answered: a withheld
  // section must not read as a confirmed empty account.
  const count = tagsAnswered ? tags.length : undefined;
  // Verbs, not values: an archived account still SHOWS its tags above, it
  // just does not offer to change them. `useCompanyReadOnlyReason` needs a
  // resolved Organization, so this reads it off `TagsSectionVerb` below, a
  // component mounted only once `view.organization` exists — keeping that
  // hook call unconditional (Rules of Hooks) without fabricating a stand-in
  // record just to satisfy its signature while the 360 read is still in
  // flight.
  return (
    <Panel
      title={t("co.tags.title")}
      titleAction={
        count != null ? <Badge>{formatNumber(count, locale)}</Badge> : undefined
      }
    >
      <PanelBody>
        <SurfaceState
          label={t("co.tags.tags")}
          state={tagState}
          emptyLabel={t("co.tags.noTags")}
        >
          <p className="co-row-meta">
            {tags.map((tag) => (
              <Badge key={tag.id}>{tag.name}</Badge>
            ))}
          </p>
        </SurfaceState>
        {tagsAnswered && view?.organization && (
          <TagsSectionVerb organization={view.organization} orgId={orgId} />
        )}
      </PanelBody>
    </Panel>
  );
}

// The add-tag verb, gated on writability. Split out so
// `useCompanyReadOnlyReason` — which needs a resolved Organization — is only
// ever called once one exists (the caller above renders this component only
// when `view.organization` is set), rather than called conditionally inside
// TagsSection itself or fed a stand-in record just to satisfy its signature.
function TagsSectionVerb({
  organization,
  orgId,
}: Readonly<{
  organization: Organization;
  orgId: string;
}>) {
  const readOnlyReason = useCompanyReadOnlyReason(organization);
  if (readOnlyReason) {
    return null;
  }
  return (
    <div className="co-card-actions">
      <TagAction orgId={orgId} />
    </div>
  );
}

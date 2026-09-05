// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRecordZone } from "../app/recordzone";
import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { SurfaceState } from "../design-system/surfacestate";
import {
  formatDateAbbrev,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { useLocale, useT } from "../i18n";
import { type Project360, stateOf } from "./projectsections";

// The project page's band under its header: the readings plate and the one
// line about how well its correspondence is filed. Apart from the section
// cards because they are read ACROSS, once, rather than down a column.

// Where a reading's own door leads. This page has no tabs — every section is
// already on it — so the door is a scroll to the card the figure was computed
// from rather than a route. Named here and consumed by `project360.tsx`, which
// owns the layout, so the two cannot drift apart into an id nothing carries.
export const PROJECT_DEALS_ANCHOR = "project-deals";
export const PROJECT_COMMITMENTS_ANCHOR = "project-commitments";

// A reading with a card behind it opens that card. `scrollIntoView` and not a
// fragment href: this app routes on the hash, so `#project-deals` would be read
// as an address and take the reader off the record.
function reveal(anchor: string): () => void {
  return () => {
    document
      .getElementById(anchor)
      ?.scrollIntoView({ behavior: "smooth", block: "start" });
  };
}

/**
 * The header figures: what the deals on this project are worth, what is
 * still owed, and how much has been filed. Present only when the server
 * could compute all of them — it withholds the whole strip rather than half
 * of it, because a strip showing deals and no work reads as a project with
 * no work.
 */
export function RollupsStrip({ view }: Readonly<{ view: Project360 }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const rollups = view.rollups;
  const state = stateOf(view, "rollups", Boolean(rollups), rollups ? 1 : 0);
  if (state !== "ready" || !rollups) {
    return (
      <SurfaceState
        state={state}
        emptyLabel={t("project.rollups.empty")}
        loadingLabel={t("project.rollups.openValue")}
      >
        {null}
      </SurfaceState>
    );
  }
  return (
    <StatStrip testId="project-rollups">
      <StatCard
        label={t("project.rollups.openValue")}
        value={formatMoneyOrAbsent(
          rollups.open_deal_value.amount_minor,
          rollups.open_deal_value.currency,
          locale,
        )}
        numeric
        openLabel={t("project.rollups.openDeals")}
        onOpen={reveal(PROJECT_DEALS_ANCHOR)}
      />
      <StatCard
        label={t("project.rollups.wonValue")}
        value={formatMoneyOrAbsent(
          rollups.won_deal_value.amount_minor,
          rollups.won_deal_value.currency,
          locale,
        )}
        numeric
        openLabel={t("project.rollups.openDeals")}
        onOpen={reveal(PROJECT_DEALS_ANCHOR)}
      />
      <StatCard
        label={t("project.rollups.openCommitments")}
        value={formatNumber(rollups.open_commitments, locale)}
        numeric
        openLabel={t("project.rollups.openCommitmentsList")}
        onOpen={reveal(PROJECT_COMMITMENTS_ANCHOR)}
      />
      <StatCard
        label={t("project.rollups.lastActivity")}
        value={
          rollups.last_activity_at
            ? formatDateAbbrev(rollups.last_activity_at, locale, recordZone)
            : t("project.rollups.never")
        }
      />
      <StatCard
        label={t("project.rollups.activityCount")}
        value={formatNumber(rollups.activity_count, locale)}
        numeric
      />
    </StatStrip>
  );
}

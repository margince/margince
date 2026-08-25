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
      <SurfaceState state={state} emptyLabel={t("project.rollups.empty")}>
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
      />
      <StatCard
        label={t("project.rollups.wonValue")}
        value={formatMoneyOrAbsent(
          rollups.won_deal_value.amount_minor,
          rollups.won_deal_value.currency,
          locale,
        )}
        numeric
      />
      <StatCard
        label={t("project.rollups.openCommitments")}
        value={formatNumber(rollups.open_commitments, locale)}
        numeric
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

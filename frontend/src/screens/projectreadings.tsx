// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { reveal } from "../app/reveal";
import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { SurfaceState } from "../design-system/surfacestate";
import {
  formatDateAbbrev,
  formatMoneyOrAbsent,
  formatNumber,
  MONEY_ABSENT,
} from "../format/format";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
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
// The record's own story, which `RecordView` draws under the work column: the
// activity readings are counted FROM it, so their door is a scroll to it.
export const PROJECT_ACTIVITY_ANCHOR = "project-activity";

// A money rollup as a READING: the figure where the deals carry one, and what
// their absence means where they do not. `formatMoneyOrAbsent` owns whether the
// pair can be said as money at all and its sentinel is that answer; the word
// for the absence belongs to the slot, because "no deal open" and "none won
// yet" are different facts about a project and a dash states neither.
function dealValue(
  value: components["schemas"]["Money"],
  absent: MessageKey,
  t: Translator,
  locale: Locale,
): string {
  const figure = formatMoneyOrAbsent(
    value.amount_minor,
    value.currency,
    locale,
  );
  return figure === MONEY_ABSENT ? t(absent) : figure;
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
        value={dealValue(
          rollups.open_deal_value,
          "project.rollups.noneOpen",
          t,
          locale,
        )}
        onOpen={reveal(PROJECT_DEALS_ANCHOR)}
      />
      <StatCard
        label={t("project.rollups.wonValue")}
        value={dealValue(
          rollups.won_deal_value,
          "project.rollups.noneWon",
          t,
          locale,
        )}
        onOpen={reveal(PROJECT_DEALS_ANCHOR)}
      />
      <StatCard
        label={t("project.rollups.openCommitments")}
        value={formatNumber(rollups.open_commitments, locale)}
        onOpen={reveal(PROJECT_COMMITMENTS_ANCHOR)}
      />
      <StatCard
        label={t("project.rollups.lastActivity")}
        value={
          rollups.last_activity_at
            ? formatDateAbbrev(rollups.last_activity_at, locale, recordZone)
            : t("project.rollups.never")
        }
        onOpen={reveal(PROJECT_ACTIVITY_ANCHOR)}
      />
      <StatCard
        label={t("project.rollups.activityCount")}
        value={formatNumber(rollups.activity_count, locale)}
        onOpen={reveal(PROJECT_ACTIVITY_ANCHOR)}
      />
    </StatStrip>
  );
}

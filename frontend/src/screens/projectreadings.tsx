// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { reveal } from "../app/reveal";
import { StatCard } from "../design-system/atoms";
import { StatStrip } from "../design-system/statstrip";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDateAbbrev, formatMoney, formatNumber } from "../format/format";
import { formatMoneyOrWord } from "../format/moneyword";
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
// their absence means where they do not. The word belongs to the slot, because
// "no deal open" and "none won yet" are different facts about a project and a
// dash states neither.
function dealValue(
  value: components["schemas"]["Money"],
  absent: MessageKey,
  t: Translator,
  locale: Locale,
): string {
  return formatMoneyOrWord(
    value.amount_minor,
    value.currency,
    locale,
    t(absent),
    formatMoney,
  );
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
        // What the plate is waiting for, not what its first slot is called: a
        // reading's LABEL standing in for a loading line said "Open deals" at
        // a reader who was waiting for all four.
        loadingLabel={t("reading.loading")}
      >
        {null}
      </SurfaceState>
    );
  }
  // Every slot declares the NARROW shape, and every slot on the row must:
  // below the strip's two-up width a `row` slot folds to one full-width line
  // (statstrip.css). The fold is the PLATE's — `.stat-strip:has(...)` — so a
  // row where only some cards carry it draws a bordered box among a column of
  // borderless ones.
  return (
    <StatStrip testId="project-rollups">
      <StatCard
        narrow="row"
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
        narrow="row"
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
        narrow="row"
        label={t("project.rollups.openCommitments")}
        value={formatNumber(rollups.open_commitments, locale)}
        onOpen={reveal(PROJECT_COMMITMENTS_ANCHOR)}
      />
      {/* ONE reading of the activity feed, not two. How much is filed and
          when the last of it landed are the same feed answered twice, they
          opened the same anchor, and neither said what the other could not —
          so the count is the reading and the date qualifies it. */}
      <StatCard
        narrow="row"
        label={t("project.rollups.activityCount")}
        value={
          rollups.activity_count > 0
            ? t("project.rollups.activityFiled", {
                count: formatNumber(rollups.activity_count, locale),
              })
            : t("project.rollups.never")
        }
        detail={
          rollups.last_activity_at
            ? t("project.rollups.activityLast", {
                date: formatDateAbbrev(
                  rollups.last_activity_at,
                  locale,
                  recordZone,
                ),
              })
            : undefined
        }
        onOpen={reveal(PROJECT_ACTIVITY_ANCHOR)}
      />
    </StatStrip>
  );
}

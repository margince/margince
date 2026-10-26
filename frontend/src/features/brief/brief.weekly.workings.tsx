// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type Fact, FactList } from "../../design-system/factlist";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  figureValue,
  type WeeklyFigure,
  type WeeklyFigures,
} from "./brief.numeric";
import type { WeeklyReview } from "./brief.queries";

import "./brief.weekly.css";

/**
 * The week's workings, under the figures table: readings that answer "how did
 * the week go" rather than "what did it produce".
 *
 * One row per reading, label left and value right, like the table above. Deals
 * lost is the table's own row, so it is not repeated here.
 */
export function WeeklyWorkings({
  counts,
  figures,
}: Readonly<{ counts: WeeklyReview["counts"]; figures: WeeklyFigures }>) {
  const t = useT();
  const { locale } = useLocale();
  const n = (value: number) => formatNumber(value, locale);
  const of = (figure: WeeklyFigure, value: string) =>
    figureValue(figures[figure], value, t);
  const facts: Fact[] = [
    {
      key: "tasks",
      term: t("brief.weekly.tasksDelivered"),
      value: of(
        "tasks",
        t("brief.weekly.ofDue", {
          done: n(counts.tasks_done),
          due: n(counts.tasks_due),
        }),
      ),
    },
    {
      key: "moved",
      term: t("brief.weekly.dealsMoved"),
      value: of("moved", n(counts.deals_moved)),
    },
    {
      key: "decided",
      term: t("brief.weekly.decided"),
      value: t("brief.weekly.acceptedRejected", {
        accepted: n(counts.proposals_accepted),
        rejected: n(counts.proposals_rejected),
      }),
    },
    {
      key: "queue",
      term: t("brief.weekly.queueWorked"),
      value: t("brief.weekly.actedDismissed", {
        acted: n(counts.brief_items_acted),
        dismissed: n(counts.brief_items_dismissed),
      }),
    },
  ];
  return (
    <Panel>
      <PanelBody>
        <FactList numeric className="brief-weekly-workings" facts={facts} />
      </PanelBody>
    </Panel>
  );
}

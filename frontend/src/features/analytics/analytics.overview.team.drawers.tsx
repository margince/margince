// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { DataTable } from "../../design-system/datatable";
import { QueryGate } from "../../design-system/querystates";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  type HandoffScope,
  MoreRows,
  useHandoffPages,
  WaitingRow,
} from "./analytics.outcomes.sdr";
import { EvidenceFrame } from "./reporting.evidence";
import { reportingPeriodLabel } from "./reporting.model";

type Review = components["schemas"]["SdrTeamReview"];
type Handoffs = components["schemas"]["SdrTeamHandoffs"];
export type Decided = "recycled" | "rejected";

// The reasons given for one outcome's decisions in the period: the same
// population the bottleneck counts, never the rounds submitted in it.
export function ReasonsDrawer({
  outcome,
  review,
  handoffs,
  zone,
  onClose,
}: Readonly<{
  outcome: Decided;
  review: Review;
  handoffs: Handoffs;
  zone: string;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const reasons = handoffs.reasons.filter(
    (reason) => reason.outcome === outcome,
  );
  const total =
    outcome === "recycled"
      ? handoffs.outcomes.recycled
      : handoffs.outcomes.rejected;
  const listed = reasons.reduce((sum, reason) => sum + reason.count, 0);
  return (
    <EvidenceFrame
      title={t(
        outcome === "recycled"
          ? "analytics.team.reasonsReturned"
          : "analytics.team.reasonsRejected",
      )}
      lines={[
        t("analytics.team.reasonsBasis", {
          span: reportingPeriodLabel(
            {
              start_at: review.interval.start_at,
              end_at: review.observed_through,
            },
            zone,
            locale,
          ),
          total: formatNumber(total, locale),
        }),
      ]}
      onClose={onClose}
    >
      <DataTable
        label={t("analytics.team.reasons")}
        rows={reasons}
        rowKey={(reason) => `${reason.given}:${reason.label}`}
        columns={[
          {
            key: "reason",
            header: t("analytics.team.reason"),
            grow: true,
            render: (reason) =>
              reason.given ? reason.label : t("analytics.team.noReason"),
          },
          {
            key: "count",
            header: t("analytics.team.count"),
            align: "end",
            render: (reason) => formatNumber(reason.count, locale),
          },
        ]}
      />
      {listed < total && (
        <p className="t-caption">{t("analytics.team.reasonsCut")}</p>
      )}
    </EvidenceFrame>
  );
}

// The handoffs waiting for a decision now, oldest first. The rows name the
// prospect and who it waits on, never who handed it on.
export function WaitingDrawer({
  scope,
  review,
  onClose,
}: Readonly<{ scope: HandoffScope; review: Review; onClose: () => void }>) {
  const t = useT();
  const { locale } = useLocale();
  const rows = useHandoffPages(scope, "waiting", true);
  const loaded = rows.data?.pages.flatMap((page) => page.rows) ?? [];
  const asOf = Date.parse(rows.data?.pages[0]?.as_of ?? review.as_of);
  return (
    <EvidenceFrame
      title={t("analytics.team.waitingTitle")}
      lines={[t("analytics.team.waitingListBasis")]}
      onClose={onClose}
    >
      <QueryGate query={rows} pendingLabel={t("analytics.team.waitingTitle")}>
        {() => (
          <div>
            {loaded.map((row) => (
              <WaitingRow key={row.id} row={row} asOf={asOf} locale={locale} />
            ))}
            <MoreRows rows={rows} />
          </div>
        )}
      </QueryGate>
    </EvidenceFrame>
  );
}

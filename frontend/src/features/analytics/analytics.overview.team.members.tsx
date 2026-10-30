// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatDuration, formatNumber } from "../../format/format";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";

type Review = components["schemas"]["SdrTeamReview"];
type Handoffs = components["schemas"]["SdrTeamHandoffs"];
type Member = components["schemas"]["SdrTeamMember"];

export function ReasonList({
  reasons,
  locale,
}: Readonly<{ reasons: Handoffs["reasons"]; locale: Locale }>) {
  const t = useT();
  return (
    <DataTable
      label={t("analytics.team.reasons")}
      rows={[...reasons]}
      rowKey={(reason) => `${reason.outcome}:${reason.label}`}
      columns={[
        {
          key: "reason",
          header: t("analytics.team.reason"),
          grow: true,
          render: (reason) => reason.label,
        },
        {
          key: "outcome",
          header: t("analytics.team.outcome"),
          render: (reason) =>
            t(
              reason.outcome === "rejected"
                ? "analytics.team.rejected"
                : "analytics.team.returned",
            ),
        },
        {
          key: "count",
          header: t("analytics.team.count"),
          align: "end",
          render: (reason) => formatNumber(reason.count, locale),
        },
      ]}
    />
  );
}

// One row per member, in name order: what each one's work came to, beside the
// work still waiting on others. The rows are not a ranking.
export function MemberTable({ review }: Readonly<{ review: Review }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (review.members_withheld) {
    return <p className="t-caption">{t("analytics.team.membersWithheld")}</p>;
  }
  if (review.members.length === 0) return null;
  const asOf = Date.parse(review.as_of);
  const count = (value: number) => formatNumber(value, locale);
  const handoffColumns = review.handoffs
    ? [
        {
          key: "submitted",
          header: t("analytics.team.submitted"),
          align: "end" as const,
          render: (row: Member) => count(row.submitted),
        },
        {
          key: "accepted",
          header: t("analytics.team.accepted"),
          align: "end" as const,
          render: (row: Member) => count(row.accepted),
        },
        {
          key: "returned",
          header: t("analytics.team.returned"),
          align: "end" as const,
          render: (row: Member) => count(row.recycled),
        },
        {
          key: "rejected",
          header: t("analytics.team.rejected"),
          align: "end" as const,
          render: (row: Member) => count(row.rejected),
        },
        {
          key: "waiting",
          header: t("analytics.team.waiting"),
          align: "end" as const,
          render: (row: Member) =>
            row.oldest_waiting_since
              ? plural("analytics.team.waitingFor", row.waiting, {
                  count: count(row.waiting),
                  age: formatDuration(
                    asOf - Date.parse(row.oldest_waiting_since),
                    locale,
                  ),
                })
              : count(row.waiting),
        },
      ]
    : [];
  return (
    <Panel title={t("analytics.team.members")}>
      <PanelBody>
        <DataTable
          label={t("analytics.team.members")}
          rows={[...review.members]}
          rowKey={(row) => row.id}
          columns={[
            {
              key: "member",
              header: t("analytics.team.member"),
              grow: true,
              render: (row) => row.label,
            },
            {
              key: "meetings",
              header: t("analytics.team.meetings"),
              align: "end",
              render: (row) => count(row.meetings_held),
            },
            ...handoffColumns,
          ]}
        />
        <p className="t-caption">{t("analytics.team.rowsOverlap")}</p>
        {review.membership_basis === "current" && (
          <p className="t-caption">{t("analytics.team.todaysMembers")}</p>
        )}
        {review.members_truncated && (
          <p className="t-caption">{t("analytics.team.truncated")}</p>
        )}
      </PanelBody>
    </Panel>
  );
}

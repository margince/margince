// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../../design-system/callout";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { identifierNumber, monthName } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { replaceDial } from "../../lib/urlstate";
import type {
  SalesProgress,
  SalesProgressMember,
  SalesProgressMonth,
} from "./progress.queries";
import { amountText, DeltaBadge, goalText } from "./progress.result";

/** Each team member's goal, closed and planned revenue, in name order. */
export function ProgressMembers({
  progress,
}: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const { currency } = progress;
  return (
    <Panel title={t("progress.team")}>
      <PanelBody>
        {progress.members_withheld && (
          <Callout tone="info" title={t("progress.membersWithheld")} />
        )}
        {progress.members.length > 0 && (
          <DataTable<SalesProgressMember>
            label={t("progress.team")}
            fold
            rows={progress.members}
            rowKey={(member) => member.owner_id}
            // The seller's own view, for the same period.
            onRowClick={(member) =>
              replaceDial("scope", `owner:${member.owner_id}`)
            }
            columns={[
              {
                key: "name",
                header: t("progress.member"),
                render: (member) => member.name,
                fold: "title",
                grow: true,
              },
              {
                key: "goal",
                header: t("progress.goal"),
                align: "end",
                render: (member) => goalText(member.goal, currency, locale, t),
              },
              {
                key: "closed",
                header: t("progress.closedRevenue"),
                align: "end",
                render: (member) =>
                  amountText(member.closed, currency, locale, t),
              },
              {
                key: "delta",
                header: t("progress.delta"),
                align: "end",
                render: (member) => (
                  <DeltaBadge delta={member.delta_minor} currency={currency} />
                ),
              },
              {
                key: "planned",
                header: t("progress.planned"),
                align: "end",
                render: (member) =>
                  amountText(member.planned, currency, locale, t),
              },
            ]}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

/** The twelve months of the year: goal, closed revenue and the difference. */
export function ProgressMonths({
  progress,
}: Readonly<{ progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const { currency } = progress;
  return (
    <Panel
      title={t("progress.months", {
        year: identifierNumber(progress.period.year),
      })}
    >
      <PanelBody>
        <DataTable<SalesProgressMonth>
          label={t("progress.months", {
            year: identifierNumber(progress.period.year),
          })}
          rows={progress.months}
          rowKey={(row) => String(row.month)}
          columns={[
            {
              key: "month",
              header: t("progress.month"),
              render: (row) => monthName(row.month, locale),
              grow: true,
            },
            {
              key: "goal",
              header: t("progress.goal"),
              align: "end",
              render: (row) => goalText(row.goal, currency, locale, t),
            },
            {
              key: "closed",
              header: t("progress.closedRevenue"),
              align: "end",
              render: (row) => amountText(row.closed, currency, locale, t),
            },
            {
              key: "delta",
              header: t("progress.delta"),
              align: "end",
              render: (row) =>
                row.state === "not_started" ? (
                  <span className="t-caption">{t("progress.notStarted")}</span>
                ) : (
                  <DeltaBadge delta={row.delta_minor} currency={currency} />
                ),
            },
          ]}
        />
      </PanelBody>
    </Panel>
  );
}

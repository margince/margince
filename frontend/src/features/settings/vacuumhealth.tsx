// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EmptyState } from "../../design-system/atoms";
import { CellStack } from "../../design-system/cellstack";
import { CopyAction } from "../../design-system/clipboardcopy";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Heading } from "../../design-system/heading";
import { PanelBody, PanelIntro } from "../../design-system/panel";
import { formatBytes, formatNumber } from "../../format/format";
import { formatPercent } from "../../format/percent";
import {
  type Locale,
  type Translator,
  useLocale,
  usePlural,
  useT,
} from "../../i18n";
import { useCan } from "../../lib/capability";
import { useHealthScope, useHealthVisible } from "../../lib/healthscope";
import { unwrap } from "../../lib/problem";
import { HealthCard } from "./healthcard";

// GET /admin/vacuum-recommendations: the tables a full vacuum would shrink by
// enough to be worth a maintenance window. Margince only advises; the operator
// runs the statement as the table's owner (compose/vacuumhealth).

type Report = components["schemas"]["VacuumRecommendations"];
type Table = components["schemas"]["VacuumRecommendation"];

function tableColumns(t: Translator, locale: Locale): DataTableColumn<Table>[] {
  return [
    {
      key: "table",
      header: t("vacuumHealth.col.table"),
      render: (row) => `${row.schema}.${row.table}`,
    },
    {
      key: "size",
      header: t("vacuumHealth.col.size"),
      align: "end",
      render: (row) => formatBytes(row.table_bytes, locale),
    },
    {
      key: "waste",
      header: t("vacuumHealth.col.waste"),
      align: "end",
      render: (row) => (
        <CellStack>
          <span>{formatBytes(row.waste_bytes, locale)}</span>
          <span>{formatPercent(row.waste_percent / 100, locale)}</span>
        </CellStack>
      ),
    },
    {
      key: "run",
      header: t("vacuumHealth.col.run"),
      grow: true,
      render: (row) => (
        <CellStack>
          <code>{row.statement}</code>
          <span>
            {t("vacuumHealth.runAs", { owner: row.owner })}{" "}
            <CopyAction
              text={row.statement}
              labels={{
                copy: t("vacuumHealth.copy"),
                copied: t("vacuumHealth.copied"),
                remedy: t("vacuumHealth.copyRemedy"),
              }}
            />
          </span>
        </CellStack>
      ),
    },
  ];
}

function VacuumBody({ report }: Readonly<{ report: Report }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const thresholds = {
    percent: formatPercent(report.min_waste_percent / 100, locale),
    size: formatBytes(report.min_waste_bytes, locale),
  };
  return (
    <>
      <PanelBody>
        <PanelIntro>{t("vacuumHealth.thresholds", thresholds)}</PanelIntro>
        <p>{t("vacuumHealth.scope")}</p>
        {report.unmeasured_tables > 0 && (
          <p>
            {plural("vacuumHealth.unmeasured", report.unmeasured_tables, {
              count: formatNumber(report.unmeasured_tables, locale),
            })}
          </p>
        )}
        {report.tables.length === 0 && (
          <EmptyState>{t("vacuumHealth.empty", thresholds)}</EmptyState>
        )}
      </PanelBody>
      {report.tables.length > 0 && (
        <>
          <DataTable
            bleed
            label={t("settings.vacuumHealth")}
            columns={tableColumns(t, locale)}
            rows={[...report.tables]}
            rowKey={(row) => `${row.schema}.${row.table}`}
          />
          <PanelBody>
            <Heading size="xsmall" as="h3">
              {t("vacuumHealth.howTitle")}
            </Heading>
            <ol>
              <li>{t("vacuumHealth.step.window")}</li>
              <li>{t("vacuumHealth.step.disk")}</li>
              <li>{t("vacuumHealth.step.owner")}</li>
              <li>{t("vacuumHealth.step.run")}</li>
            </ol>
            <p>{t("vacuumHealth.repack")}</p>
          </PanelBody>
        </>
      )}
    </>
  );
}

function isReport(data: Report | undefined): data is Report {
  return (
    typeof data === "object" &&
    data !== null &&
    typeof data.generated_at === "string" &&
    typeof data.min_waste_percent === "number" &&
    typeof data.min_waste_bytes === "number" &&
    typeof data.unmeasured_tables === "number" &&
    Array.isArray(data.tables)
  );
}

// `job_health:read`, what the endpoint asks for: the grant every System
// health card shares.
export function useVacuumHealth() {
  const canSee = useCan("job_health", "read");
  const scope = useHealthScope();
  const visible = useHealthVisible();
  const query = useQuery({
    queryKey: ["vacuum-recommendations", scope],
    enabled: canSee && visible && scope !== undefined,
    queryFn: async ({ signal }) => {
      const data = unwrap(
        await api.GET("/admin/vacuum-recommendations", { signal }),
      );
      // Rejected, not defaulted: an empty list would claim no table needs it.
      if (!isReport(data)) {
        throw new Error("malformed vacuum-recommendations response");
      }
      return data;
    },
  });
  return { canSee, query };
}

export function VacuumHealthCard() {
  const t = useT();
  const { canSee, query } = useVacuumHealth();
  return (
    <HealthCard
      title={t("settings.vacuumHealth")}
      sub={t("settings.vacuumHealthSub")}
      withheld={t("vacuumHealth.adminOnly")}
      canSee={canSee}
      query={query}
    >
      {(report) => <VacuumBody report={report} />}
    </HealthCard>
  );
}

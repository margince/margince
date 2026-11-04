// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { useUrlParams } from "../../lib/urlstate";
import { useAnalyticsContext } from "./analytics.context";
import { OVERVIEW_DIAL } from "./analytics.overview.choice";
import { InitiativeDrawer } from "./reporting.initiativedrawer";
import { reportingAmount } from "./reporting.model";
import { useReportingCatalog } from "./reporting.queries";

export type SalesInitiative = components["schemas"]["SalesInitiative"];

export const INITIATIVES_KEY = "sales-initiatives";

/** The initiatives a reader may open, drafts and live ones, newest first. */
export function useSalesInitiatives(enabled = true) {
  return useQuery({
    queryKey: [INITIATIVES_KEY],
    queryFn: async () => unwrap(await api.GET("/analytics/initiatives")).data,
    enabled,
    staleTime: 30_000,
  });
}

/**
 * Sales initiatives: one entry per effort, each opening Sales review narrowed
 * to it, and the door to defining a new one. A draft opens its own setup.
 */
export function InitiativeReports() {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const [params] = useUrlParams();
  const catalog = useReportingCatalog();
  const context = useAnalyticsContext();
  const initiatives = useSalesInitiatives();
  const canCreate = useCanWrite("sales_initiative", "create");
  const [drawer, setDrawer] = useState<"new" | SalesInitiative | null>(null);
  return (
    <Panel
      title={t("reporting.initiatives")}
      titleAction={
        canCreate ? (
          <Button variant="primary" onClick={() => setDrawer("new")}>
            {t("reporting.newInitiative")}
          </Button>
        ) : undefined
      }
    >
      <PanelBody>
        <p className="t-caption">{t("reporting.initiativesPurpose")}</p>
        <QueryGate
          query={initiatives}
          pendingLabel={t("reporting.initiatives")}
        >
          {(rows) => (
            <>
              {rows.length === 0 && (
                <EmptyState>{t("reporting.noInitiatives")}</EmptyState>
              )}
              {rows.map((initiative) => (
                <PanelRow key={initiative.id}>
                  <div>
                    {initiative.status === "draft" ? (
                      <Button
                        variant="link"
                        onClick={() => setDrawer(initiative)}
                      >
                        {initiative.name}
                      </Button>
                    ) : (
                      <AppLink
                        href={analyticsSectionHref(
                          "overview",
                          initiativeDials(params, initiative.id),
                        )}
                      >
                        {initiative.name}
                      </AppLink>
                    )}
                    <p className="t-caption">
                      {initiativeSummary(
                        initiative,
                        t,
                        locale,
                        zone,
                        catalog.data?.metrics.find(
                          (metric) => metric.id === initiative.objective_metric,
                        )?.unit,
                        context.data?.base_currency,
                      )}
                    </p>
                  </div>
                  <Badge>
                    {t(`reporting.initiativeStatus.${initiative.status}`)}
                  </Badge>
                </PanelRow>
              ))}
            </>
          )}
        </QueryGate>
      </PanelBody>
      {drawer !== null && (
        <InitiativeDrawer
          initiative={drawer === "new" ? undefined : drawer}
          onClose={() => setDrawer(null)}
        />
      )}
    </Panel>
  );
}

function initiativeSummary(
  initiative: SalesInitiative,
  t: ReturnType<typeof useT>,
  locale: ReturnType<typeof useLocale>["locale"],
  zone: ReturnType<typeof useRecordZone>,
  unit: string | undefined,
  currency: string | undefined,
): string {
  const objective =
    initiative.target_value == null
      ? t("reporting.trackingOnly")
      : t("reporting.initiativeTarget", {
          target:
            unit && (unit !== "money" || currency)
              ? reportingAmount(
                  initiative.target_value,
                  unit,
                  currency ?? "",
                  locale,
                )
              : "—",
        });
  return t("reporting.initiativeSummary", {
    objective,
    deadline: formatDate(initiative.deadline, locale, zone),
  });
}

/** Sales review for one initiative: the reader's period and scope stay. */
export function initiativeDials(
  params: ReadonlyMap<string, string>,
  initiativeId: string,
): Map<string, string> {
  const next = new Map(params);
  for (const dial of ["product", "country", "industry", "basis", "view"])
    next.delete(dial);
  next.set("initiative", initiativeId);
  next.set(OVERVIEW_DIAL, "sales");
  return next;
}

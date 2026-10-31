// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { AppLink } from "../../design-system/applink";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCanWrite } from "../../lib/capability";
import { usePipelines } from "../../lib/pipelines.queries";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useUrlParams } from "../../lib/urlstate";
import { ReportingCharts } from "./reporting.charts";
import {
  ReportingEvidenceDrawer,
  useSelectionEvidence,
} from "./reporting.evidence";
import { reportingQuery } from "./reporting.model";
import {
  PREPARED_REPORTS,
  type PreparedReport,
  preparedSelection,
} from "./reporting.prepared";
import { useReportingCatalog } from "./reporting.queries";
import { SaveReportingDialog } from "./reporting.save";

type ReportingSelection = components["schemas"]["ReportingSelection"];

// The reports a team usually wants first, ready to look at and keep. Each is
// asked of the reader's own record scope and their own metric grants.
export function PreparedReports({
  scope,
  ownLens,
}: Readonly<{ scope: ReportingSelection["scope"]; ownLens: boolean }>) {
  const t = useT();
  const catalog = useReportingCatalog();
  const pipelines = usePipelines();
  const [open, setOpen] = useState<string | null>(null);

  return (
    <Panel title={t("reporting.prepared.title")}>
      <PanelBody>
        <QueryGate query={catalog} pendingLabel={t("reporting.prepared.title")}>
          {(metrics) => (
            // The default pipeline is part of a saved copy's question, so
            // nothing is offered before it is known.
            <QueryGate
              query={pipelines}
              pendingLabel={t("reporting.prepared.title")}
            >
              {(loaded) => (
                <>
                  {PREPARED_REPORTS.filter(
                    (report) =>
                      report.kind === "question" ||
                      !report.ownLensOnly ||
                      ownLens,
                  ).map((report) => (
                    <PreparedRow
                      key={report.id}
                      report={report}
                      selection={
                        report.kind === "question"
                          ? preparedSelection(
                              report,
                              metrics,
                              scope,
                              loaded.find((pipeline) => pipeline.is_default)
                                ?.id,
                            )
                          : null
                      }
                      previewing={open === report.id}
                      onPreview={() =>
                        setOpen((current) =>
                          current === report.id ? null : report.id,
                        )
                      }
                    />
                  ))}
                </>
              )}
            </QueryGate>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

function PreparedRow({
  report,
  selection,
  previewing,
  onPreview,
}: Readonly<{
  report: PreparedReport;
  selection: ReportingSelection | null;
  previewing: boolean;
  onPreview: () => void;
}>) {
  const t = useT();
  const [params] = useUrlParams();
  const canCreate = useCanWrite("report_definition", "create");
  const [saving, setSaving] = useState(false);
  // A question none of whose metrics this reader may read has nothing to show.
  if (report.kind === "question" && !selection) {
    return null;
  }
  return (
    <>
      <PanelRow className="analytics-attention-row">
        <div className="analytics-attention-text">
          <p className="analytics-attention-title">{t(report.title)}</p>
          <p className="t-caption">{t(report.purpose)}</p>
        </div>
        {report.kind === "section" ? (
          <AppLink
            className="btn btn-ghost"
            href={analyticsSectionHref(report.section, params)}
          >
            {t("reporting.prepared.open")}
          </AppLink>
        ) : (
          <>
            <Button onClick={onPreview} aria-expanded={previewing}>
              {t("reporting.prepared.preview")}
            </Button>
            {canCreate && (
              <Button onClick={() => setSaving(true)}>
                {t("reporting.prepared.saveCopy")}
              </Button>
            )}
          </>
        )}
      </PanelRow>
      {previewing && selection && <PreparedPreview selection={selection} />}
      {saving && selection && (
        <SaveReportingDialog
          selection={selection}
          onClose={() => setSaving(false)}
          onSaved={(saved) => {
            setSaving(false);
            navigate({ screen: "analytics", id: "reports", id2: saved.id });
          }}
        />
      )}
    </>
  );
}

function PreparedPreview({
  selection,
}: Readonly<{ selection: ReportingSelection }>) {
  const t = useT();
  const { evidence, openEvidence, closeEvidence } = useSelectionEvidence(
    JSON.stringify(selection),
  );
  const evaluation = useQuery({
    queryKey: ["reporting-evaluation", selection],
    queryFn: async () =>
      unwrap(
        await api.GET("/analytics/evaluate", {
          params: { query: reportingQuery(selection) },
        }),
      ),
  });
  return (
    <QueryGate
      query={evaluation}
      pendingLabel={t("reporting.prepared.preview")}
    >
      {(result) => (
        <>
          <ReportingCharts evaluation={result} onEvidence={openEvidence} />
          {evidence && (
            <ReportingEvidenceDrawer
              evaluation={result}
              reference={evidence}
              onClose={closeEvidence}
            />
          )}
        </>
      )}
    </QueryGate>
  );
}

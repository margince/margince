// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { AppLink } from "../../design-system/applink";
import { Badge, Button, EmptyState } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { useViewerId } from "../../lib/session";
import { useUrlParams } from "../../lib/urlstate";
import { ReportingCharts } from "./reporting.charts";
import {
  type CustomReportDraft,
  CustomReportDrawer,
  heldCustomReport,
} from "./reporting.custombuilder";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import { useSelectionEvidence } from "./reporting.evidenceselection";
import { useReportingEvaluation } from "./reporting.queries";

/**
 * A custom report before it is saved: the builder's draft read through the
 * same evaluation a saved report reads, with its cards in their order.
 * Customize returns to the builder; Save writes the definition and opens it.
 */
export function CustomReportPreview() {
  const t = useT();
  const [params] = useUrlParams();
  const reader = useViewerId();
  const draft = heldCustomReport(reader);
  if (!draft)
    return (
      <EmptyState
        action={
          <AppLink
            className="btn btn-ghost"
            href={analyticsSectionHref("reports", params)}
          >
            {t("reporting.backToReports")}
          </AppLink>
        }
      >
        {t("reporting.noPreview")}
      </EmptyState>
    );
  return <HeldPreview initial={draft} reader={reader} />;
}

function HeldPreview({
  initial,
  reader,
}: Readonly<{ initial: CustomReportDraft; reader: string | undefined }>) {
  const t = useT();
  const client = useQueryClient();
  const canSave = useCanWrite("report_definition", "create");
  const [customizing, setCustomizing] = useState(false);
  // The draft on screen; a closed drawer hands back whatever it previewed.
  const [draft, setDraft] = useState(initial);
  const query = useReportingEvaluation(draft.selection);
  const contextKey = JSON.stringify(draft.selection);
  const { evidence, openEvidence, closeEvidence } =
    useSelectionEvidence(contextKey);
  const save = useMutation({
    mutationFn: async (saved: CustomReportDraft) =>
      unwrap(
        await api.POST("/analytics/reports", {
          body: {
            name: saved.name,
            description: saved.description,
            audience: saved.audience,
            audience_team_id:
              saved.audience === "team" ? saved.selection.scope.id : undefined,
            selection: saved.selection,
          },
        }),
      ),
    onSuccess: async (report) => {
      await client.invalidateQueries({ queryKey: ["reporting-reports"] });
      navigate({ screen: "analytics", id: "reports", id2: report.id });
    },
  });
  return (
    <div className="form-stack">
      <Panel
        title={draft.name}
        titleAction={
          <>
            <Button variant="ghost" onClick={() => setCustomizing(true)}>
              {t("reporting.customize")}
            </Button>
            {canSave && (
              <Button
                variant="primary"
                pending={save.isPending}
                onClick={() => save.mutate(draft)}
              >
                {t("reporting.save")}
              </Button>
            )}
          </>
        }
      >
        <PanelBody>
          <Badge tone="warning">{t("reporting.unsavedPreview")}</Badge>
          {draft.description && (
            <p className="t-caption">{draft.description}</p>
          )}
          <ErrorLine error={save.error} />
        </PanelBody>
      </Panel>
      <QueryGate query={query} pendingLabel={draft.name}>
        {(evaluation) => (
          <>
            <ReportingCharts
              // The evaluation answers the blocks; the cards keep the order and
              // widths the builder gave them.
              evaluation={{
                ...evaluation,
                selection: {
                  ...evaluation.selection,
                  layout: draft.selection.layout,
                },
              }}
              onEvidence={openEvidence}
            />
            {evidence && (
              <ReportingEvidenceDrawer
                key={`${contextKey}:${JSON.stringify(evidence)}`}
                evaluation={evaluation}
                reference={evidence}
                onClose={closeEvidence}
              />
            )}
          </>
        )}
      </QueryGate>
      {customizing && (
        <CustomReportDrawer
          initial={draft.selection}
          draft={draft}
          onClose={() => {
            setCustomizing(false);
            setDraft(heldCustomReport(reader) ?? draft);
          }}
        />
      )}
    </div>
  );
}

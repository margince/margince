// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { WrittenBy } from "../../components/citations.list";
import { EntityRef } from "../../components/entityref";
import { AiPending } from "../../design-system/aipending";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatAnalyticsNumber } from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { isVersionSkewOf, problemFieldErrors, unwrap } from "../../lib/problem";
import { reloadReportingFigures } from "../../lib/reportingfigures";
import {
  assessmentText,
  readableQuantities,
} from "./analytics.assessment.text";
import { RecordDecisionDialog } from "./analytics.decisions";
import { metricOf } from "./analytics.overview.readings";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
  ReportingSelection,
} from "./reporting.model";
import { useMetricNames, useReportingCatalog } from "./reporting.queries";

export type SalesAssessment = components["schemas"]["SalesAssessment"];
type View = components["schemas"]["SalesAssessmentView"];
type WorkReading = Readonly<{
  upcoming_meetings?: number;
  upcoming_window?: { start_at: string; end_at: string };
}>;

export function useSalesAssessment(
  selection: ReportingSelection,
  enabled: boolean,
  evaluation?: ReportingEvaluation,
  view: View = "sales",
  work?: WorkReading,
) {
  const [refresh, setRefresh] = useState(0);
  const client = useQueryClient();
  const { locale } = useLocale();
  const catalog = useReportingCatalog();
  const supported = catalog.data?.assessment_views?.includes(view) === true;
  const query = useQuery({
    enabled: enabled && !catalog.isPending && (supported || view === "sales"),
    queryKey: [
      "sales-assessment",
      view,
      selection,
      evaluation?.evaluation_key,
      locale,
      supported,
      work?.upcoming_meetings,
      work?.upcoming_window?.start_at,
      work?.upcoming_window?.end_at,
      refresh,
    ],
    queryFn: async ({ signal }) => {
      const body = {
        selection,
        refresh: refresh > 0,
        ...(supported
          ? {
              view,
              receipt: evaluation
                ? {
                    key: evaluation.evaluation_key,
                    at: evaluation.context.evaluated_at,
                    framework_revision: evaluation.context.framework_revision,
                  }
                : undefined,
            }
          : {}),
      };
      const result = await api.POST("/analytics/assessment", { body, signal });
      if (
        view === "sales" &&
        supported &&
        result.response.status === 422 &&
        legacyAssessmentRefusal(result.error)
      ) {
        const legacy = await api.POST("/analytics/assessment", {
          body: { selection, refresh: refresh > 0 },
          signal,
        });
        return { assessment: unwrap(legacy), bound: false };
      }
      return { assessment: unwrap(result), bound: supported };
    },
    retry: false,
    staleTime: 60_000,
  });
  const response = query.data;
  const assessment =
    response &&
    (!response.bound ||
      !evaluation ||
      response.assessment.evaluation_key === evaluation.evaluation_key)
      ? response.assessment
      : undefined;
  return {
    query,
    assessment,
    supported,
    refresh: () => {
      if (isVersionSkewOf(query.error)) {
        void reloadReportingFigures(client);
        return;
      }
      setRefresh((n) => n + 1);
    },
  };
}

function legacyAssessmentRefusal(problem: unknown): boolean {
  const fields = problemFieldErrors(problem);
  const field = fields[0];
  return (
    fields.length === 1 &&
    field?.field === "body" &&
    field.code === "unknown_field" &&
    /^unknown fields? "(?:receipt|view)"(?:, "(?:receipt|view)")?$/.test(
      field.message,
    )
  );
}

type AssessmentProps = Readonly<{
  selection: ReportingSelection;
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  view?: View;
  work?: WorkReading;
  workPending?: boolean;
}>;

function assessmentParagraphs(assessment: SalesAssessment) {
  return assessment.paragraphs?.length
    ? assessment.paragraphs
    : [assessment.sentences];
}

export function AssessmentPanel({
  selection,
  evaluation,
  onEvidence,
  view = "sales",
  work,
  workPending = false,
}: AssessmentProps) {
  const t = useT();
  const client = useQueryClient();
  const { locale } = useLocale();
  const empty =
    view === "sales" && evaluation.context.scope_holds_deals === false;
  const { query, assessment, supported, refresh } = useSalesAssessment(
    selection,
    !empty && !workPending,
    evaluation,
    view,
    work,
  );
  const written =
    assessment?.generated_by === "model" &&
    assessment.sentences.length > 0 &&
    readableAssessment(assessment, locale);
  const actions = (
    <OverviewActions
      evaluation={evaluation}
      order={written ? assessment.action_ids : undefined}
      onEvidence={onEvidence}
    />
  );
  if (empty || workPending || (view !== "sales" && !supported)) return actions;
  const upcoming = assessment?.cells?.find(
    (cell) => cell.id === "work:upcoming",
  );
  if (
    upcoming &&
    work?.upcoming_meetings != null &&
    Number(upcoming.value.replace(/\s+count$/, "").replace(/[,.]/g, "")) !==
      work.upcoming_meetings
  )
    return (
      <>
        {actions}
        <ErrorLine>{t("reporting.evidenceStale")}</ErrorLine>
        <Button onClick={() => void reloadReportingFigures(client)}>
          {t("common.retry")}
        </Button>
      </>
    );

  if (written)
    return (
      <WrittenAssessment
        assessment={assessment}
        evaluation={evaluation}
        onEvidence={onEvidence}
        view={view}
        refresh={refresh}
        actions={actions}
      />
    );
  if (!query.isFetching && !query.isError) return actions;
  return (
    <>
      <Panel title={t(outlookTitle(view, evaluation))} flatHead>
        <PanelBody>
          {query.isFetching ? (
            <AiPending label={t(outlookTitle(view, evaluation))} lines={3} />
          ) : (
            <>
              <ErrorLine error={query.error} />
              <Button variant="ghost" onClick={refresh}>
                {t("analytics.assessment.retry")}
              </Button>
            </>
          )}
        </PanelBody>
      </Panel>
      {actions}
    </>
  );
}

function outlookTitle(view: View, evaluation: ReportingEvaluation) {
  if (view === "sales")
    return evaluation.selection.period === "this_quarter"
      ? "analytics.outlook.sales"
      : "analytics.outlook.selectedSales";
  if (view === "team") return "analytics.outlook.team";
  return "analytics.outlook.prospecting";
}

function WrittenAssessment({
  assessment,
  evaluation,
  onEvidence,
  view,
  refresh,
  actions,
}: Readonly<{
  assessment: SalesAssessment;
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  view: View;
  refresh: () => void;
  actions: ReactNode;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [reading, setReading] = useState(false);
  const [deciding, setDeciding] = useState(false);
  const canDecide = useCanWrite("activity", "create");
  const proposed = assessment.next_decision
    ? {
        ...assessment.next_decision,
        text: assessmentText(
          {
            text: assessment.next_decision.text,
            cells: assessment.cells.map((cell) => cell.id),
          },
          assessment,
          locale,
        ),
      }
    : undefined;
  const next =
    proposed && readableQuantities(proposed.text) ? proposed : undefined;
  return (
    <>
      <Panel
        title={t(outlookTitle(view, evaluation))}
        tone="ai"
        flatHead
        titleAction={
          <Button variant="ghost" onClick={refresh}>
            {t("analytics.assessment.refresh")}
          </Button>
        }
        footer={<WrittenBy by={assessment.generated_by} />}
      >
        <PanelBody>
          <div className="analytics-outlook-prose">
            {assessmentParagraphs(assessment).map((paragraph) => (
              <p key={paragraph.map((s) => s.text).join(" ")}>
                {paragraph
                  .map((line) => assessmentText(line, assessment, locale))
                  .join(" ")}
              </p>
            ))}
          </div>
          <div className="actions">
            <Button variant="link" onClick={() => setReading(true)}>
              {t("analytics.outlook.records")}
            </Button>
            {next && (
              <p>
                {next.text} <EntityRef kind="deal" id={next.deal_id} />{" "}
                <EntityRef kind="user" id={next.owner_id} />
              </p>
            )}
            {next && canDecide && (
              <Button variant="ghost" onClick={() => setDeciding(true)}>
                {t("analytics.decisions.record")}
              </Button>
            )}
          </div>
        </PanelBody>
      </Panel>
      {actions}
      {reading && (
        <InterpretationDrawer
          assessment={assessment}
          evaluation={evaluation}
          onEvidence={onEvidence}
          onClose={() => setReading(false)}
        />
      )}
      {deciding && next && (
        <RecordDecisionDialog
          initial={{
            decision: next.text,
            dealId: next.deal_id,
            ownerId: next.owner_id,
          }}
          onClose={() => setDeciding(false)}
        />
      )}
    </>
  );
}

function OverviewActions({
  evaluation,
  order,
  onEvidence,
}: Readonly<{
  evaluation: ReportingEvaluation;
  order?: readonly string[];
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const { locale } = useLocale();
  const candidates = evaluation.overview_actions ?? [];
  const actions = order?.length
    ? [...candidates].sort((a, b) => {
        const rank = (id: string) => {
          const at = order.indexOf(id);
          return at < 0 ? order.length : at;
        };
        return rank(a.id) - rank(b.id);
      })
    : candidates;
  if (!actions.length) return null;
  return (
    <div className="actions">
      {actions.map((action) => (
        <Button
          key={action.id}
          variant="ghost"
          onClick={() => onEvidence(action.evidence)}
        >
          {action.label} ({formatAnalyticsNumber(action.count, locale)})
        </Button>
      ))}
    </div>
  );
}

function InterpretationDrawer({
  assessment,
  evaluation,
  onEvidence,
  onClose,
}: Readonly<{
  assessment: SalesAssessment;
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
  onClose: () => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const name = useMetricNames();
  const titleId = useId();
  const paragraphs = assessment.paragraphs?.length
    ? assessment.paragraphs
    : [assessment.sentences];
  const labelOf = (id: string) => {
    if (id.startsWith("metric:")) {
      const metric = evaluation.metrics.find((m) => `metric:${m.id}` === id);
      if (metric) return name(metric.id);
    }
    return assessment.cells?.find((cell) => cell.id === id)?.label ?? "";
  };
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <Heading size="large" id={titleId} className="modal-title">
        {t("analytics.assessment.evidenceTitle")}
      </Heading>
      <div className="analytics-outlook-prose">
        {paragraphs.flat().map((sentence) => (
          <div key={sentence.text}>
            <p>{assessmentText(sentence, assessment, locale)}</p>
            <ul>
              {sentence.cells.map((id) => {
                const reference = evidenceOf(evaluation, id);
                return (
                  <li key={id}>
                    {labelOf(id)}{" "}
                    {reference && (
                      <Button
                        variant="link"
                        onClick={() => {
                          onClose();
                          onEvidence(reference);
                        }}
                      >
                        {t("reporting.viewRecords")}
                      </Button>
                    )}
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </div>
    </Modal>
  );
}

function evidenceOf(
  evaluation: ReportingEvaluation,
  cell: string,
): ReportingEvidenceRef | undefined {
  const metric = evaluation.metrics.find(
    (candidate) => `metric:${candidate.id}` === cell,
  );
  return metric ? metricOf(evaluation, metric.id)?.evidence : undefined;
}

function readableAssessment(
  assessment: SalesAssessment,
  locale: Locale,
): boolean {
  const unsupported =
    /\b(?:commit_evidence|qualified_commit_evidence|no_target|nothing checked|i checked|i verified|i couldn[’']t confirm)\b/i;
  return assessmentParagraphs(assessment)
    .flat()
    .every((line) => {
      const text = assessmentText(line, assessment, locale);
      return !unsupported.test(text) && readableQuantities(text);
    });
}

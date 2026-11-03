// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { WrittenBy } from "../../components/citations.list";
import { EntityRef } from "../../components/entityref";
import { AiPending } from "../../design-system/aipending";
import { Badge, Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { unwrap } from "../../lib/problem";
import { RecordDecisionDialog } from "./analytics.decisions";
import { metricOf } from "./analytics.overview.readings";
import type {
  ReportingEvaluation,
  ReportingEvidenceRef,
  ReportingSelection,
} from "./reporting.model";
import { WrittenSummary } from "./reporting.written";

export type SalesAssessment = components["schemas"]["SalesAssessment"];
type Sentence = components["schemas"]["SalesAssessmentSentence"];

const ASSESSMENT_KEY = "sales-assessment";

/**
 * The assessment of one selection. The key is the selection, so a changed
 * filter never shows the assessment of another one; a refresh asks again.
 */
export function useSalesAssessment(
  selection: ReportingSelection,
  enabled: boolean,
) {
  const [refresh, setRefresh] = useState(0);
  const query = useQuery({
    enabled,
    queryKey: [ASSESSMENT_KEY, selection, refresh],
    queryFn: async () =>
      unwrap(
        await api.POST("/analytics/assessment", {
          body: { selection, refresh: refresh > 0 },
        }),
      ),
    staleTime: 60_000,
  });
  return { query, refresh: () => setRefresh((n) => n + 1) };
}

/**
 * The assessment beside the leading chart: what the figures show, each
 * sentence resting on the cells it cites, and the one decision to make next.
 * Without a model, or when its reply could not be grounded, the panel shows
 * the page's own summary and says so.
 */
export function AssessmentPanel({
  selection,
  evaluation,
  onEvidence,
}: Readonly<{
  selection: ReportingSelection;
  evaluation: ReportingEvaluation;
  onEvidence: (reference: ReportingEvidenceRef) => void;
}>) {
  const t = useT();
  const empty = evaluation.context.scope_holds_deals === false;
  const { query, refresh } = useSalesAssessment(selection, !empty);
  const [reading, setReading] = useState(false);
  const [deciding, setDeciding] = useState(false);
  const canDecide = useCanWrite("activity", "create");
  const written = query.data?.generated_by === "model";
  return (
    <Panel
      title={t("analytics.assessment.title")}
      tone={written ? "ai" : undefined}
      titleAction={
        query.data ? (
          <Button variant="ghost" onClick={refresh}>
            {t("analytics.assessment.refresh")}
          </Button>
        ) : undefined
      }
      footer={query.data ? <WrittenBy by={query.data.generated_by} /> : null}
    >
      <PanelBody>
        {empty && (
          <p className="t-caption">{t("analytics.assessment.noSales")}</p>
        )}
        {!empty && query.isPending && (
          <AiPending label={t("analytics.assessment.pending")} lines={3} />
        )}
        {!empty && query.isError && (
          <>
            <ErrorLine error={query.error} />
            <Button variant="ghost" onClick={() => query.refetch()}>
              {t("analytics.assessment.retry")}
            </Button>
          </>
        )}
        {query.data && !written && (
          <>
            <WrittenSummary evaluation={evaluation} />
            <p className="t-caption">
              {t("analytics.assessment.deterministic")}
            </p>
          </>
        )}
        {query.data && written && (
          <AssessmentBody
            assessment={query.data}
            canDecide={canDecide}
            onRead={() => setReading(true)}
            onDecide={() => setDeciding(true)}
          />
        )}
      </PanelBody>
      {reading && query.data && (
        <InterpretationDrawer
          assessment={query.data}
          evaluation={evaluation}
          onEvidence={onEvidence}
          onClose={() => setReading(false)}
        />
      )}
      {deciding && query.data?.next_decision && (
        <RecordDecisionDialog
          initial={{
            decision: query.data.next_decision.text,
            dealId: query.data.next_decision.deal_id,
            ownerId: query.data.next_decision.owner_id,
          }}
          onClose={() => setDeciding(false)}
        />
      )}
    </Panel>
  );
}

function AssessmentBody({
  assessment,
  canDecide,
  onRead,
  onDecide,
}: Readonly<{
  assessment: SalesAssessment;
  canDecide: boolean;
  onRead: () => void;
  onDecide: () => void;
}>) {
  const t = useT();
  const next = assessment.next_decision;
  return (
    <>
      <ul className="assessment-lines">
        {assessment.sentences.map((sentence) => (
          <li key={sentence.text}>
            <KindBadge kind={sentence.kind} />
            <span>{sentence.text}</span>
          </li>
        ))}
      </ul>
      <p className="t-label">{t("analytics.assessment.nextDecision")}</p>
      {next ? (
        <p>
          {next.text} <EntityRef kind="deal" id={next.deal_id} /> ·{" "}
          <EntityRef kind="user" id={next.owner_id} />
        </p>
      ) : (
        <p className="t-caption">{t("analytics.assessment.noDecision")}</p>
      )}
      <div className="actions">
        <Button variant="ghost" onClick={onRead}>
          {t("analytics.assessment.viewEvidence")}
        </Button>
        {next && canDecide && (
          <Button variant="primary" onClick={onDecide}>
            {t("analytics.decisions.record")}
          </Button>
        )}
      </div>
    </>
  );
}

function KindBadge({ kind }: Readonly<{ kind: Sentence["kind"] }>) {
  const t = useT();
  return <Badge>{t(`analytics.assessment.kind.${kind}`)}</Badge>;
}

/**
 * The interpretation drawer: each sentence with the cells it rests on, and
 * the door to the records behind a cell the evaluation measured.
 */
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
  const titleId = useId();
  const labelOf = (id: string) =>
    assessment.cells.find((cell) => cell.id === id)?.label ?? id;
  return (
    <Modal open onClose={onClose} labelledBy={titleId} intent="drawer">
      <Heading size="large" id={titleId} className="modal-title">
        {t("analytics.assessment.evidenceTitle")}
      </Heading>
      <p className="t-caption">{t("analytics.assessment.evidencePurpose")}</p>
      <ul className="assessment-lines">
        {assessment.sentences.map((sentence) => (
          <li key={sentence.text}>
            <KindBadge kind={sentence.kind} />
            <span>{sentence.text}</span>
            <ul>
              {sentence.cells.map((id) => {
                const reference = evidenceOf(evaluation, id);
                return (
                  <li key={id}>
                    {labelOf(id)}
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
          </li>
        ))}
      </ul>
      {assessment.confidence && (
        <p className="t-caption">
          {t("analytics.assessment.confidence", {
            level: t(`analytics.assessment.level.${assessment.confidence}`),
          })}
        </p>
      )}
    </Modal>
  );
}

/** The records behind a cell: a metric's own evidence, where it has one. */
function evidenceOf(
  evaluation: ReportingEvaluation,
  cell: string,
): ReportingEvidenceRef | undefined {
  if (!cell.startsWith("metric:")) return undefined;
  const id = cell.slice("metric:".length);
  const metric = evaluation.metrics.find((candidate) => candidate.id === id);
  return metric ? metricOf(evaluation, metric.id)?.evidence : undefined;
}

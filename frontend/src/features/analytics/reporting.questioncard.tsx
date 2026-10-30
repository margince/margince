import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EmptyState, PendingBody } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { monthName } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { AnswerTable, QuestionFailure } from "./analytics.questions.answer";
import {
  type AnalyticsQuery,
  customLabels,
  entityLabel,
} from "./analytics.questions.vocab";
import { useAnalyticsSchema } from "./analytics.schema";
import type { ReportingLayout } from "./reporting.layout";
import type {
  ReportingEdition,
  ReportingEvaluation,
  ReportingSelection,
} from "./reporting.model";

type Card = ReportingLayout[number]["cards"][number];
type KeptQuestion = NonNullable<ReportingEdition["questions"]>[number];
type BoundAnswer = components["schemas"]["ReportingQuestionAnswer"];

/** A saved report at the revision a page reads: a draft while previewed. */
export type ReportRevision = Readonly<{ id: string; revision?: number }>;

/** What a sent edition kept of its question cards. */
export type KeptQuestions = Pick<
  ReportingEdition,
  "questions" | "expired" | "redacted"
>;

// The rows a card shows when its question names no limit: the custom report
// page's own default.
const QUESTION_CARD_ROWS = 100;

/**
 * The report's scope as an analytics question names it. Managed teams is the
 * reader's default and is never named, as the questions page leaves it out.
 */
function scopeOf(scope: ReportingSelection["scope"]) {
  if (scope.kind === "managed_teams") return {};
  return { scope_kind: scope.kind, scope_id: scope.id ?? undefined };
}

/**
 * A card holding a custom question, asked for this reader under their own
 * grants and over the report's scope. A sent edition shows the answer it kept,
 * never one asked today.
 */
export function QuestionCard({
  card,
  wide,
  evaluation,
  report,
  mode,
  kept,
}: Readonly<{
  card: Card;
  wide: boolean;
  evaluation: ReportingEvaluation;
  report?: ReportRevision;
  // "frozen" for a sent edition; "preview" while the page shows the report as
  // a role reads it, which this browser's own session cannot ask as.
  mode: "live" | "frozen" | "preview";
  kept?: KeptQuestions;
}>) {
  const t = useT();
  const question = card.question;
  const keptAnswer = kept?.questions?.find(
    (candidate) => candidate.card_id === card.id,
  );
  return (
    <Panel
      className={
        wide ? "reporting-panel reporting-panel-wide" : "reporting-panel"
      }
      title={t("reporting.questionCard", {
        population: entityLabel(t, question?.entity ?? ""),
      })}
    >
      <PanelBody>
        {mode === "frozen" && question && keptAnswer?.answer ? (
          <AnswerTable
            query={question}
            answer={keptAnswer.answer}
            baseCurrency={evaluation.context.currency}
          />
        ) : mode !== "live" || !question ? (
          <EmptyState>
            {t(
              mode === "preview"
                ? "reporting.questionNotPreviewed"
                : notKeptMessage(keptAnswer, kept),
            )}
          </EmptyState>
        ) : (
          <LiveQuestion card={card} evaluation={evaluation} report={report} />
        )}
      </PanelBody>
    </Panel>
  );
}

/** A live card's answer: from the server for a bound card, else asked here. */
function LiveQuestion({
  card,
  evaluation,
  report,
}: Readonly<{
  card: Card;
  evaluation: ReportingEvaluation;
  report?: ReportRevision;
}>) {
  const question = card.question;
  if (!question) return null;
  if (question.binding || question.saved_question_id)
    return (
      <BoundQuestionAnswer
        card={card}
        report={question.binding ? report : undefined}
        baseCurrency={evaluation.context.currency}
      />
    );
  return (
    <QuestionAnswer
      query={{
        ...question,
        ...scopeOf(evaluation.selection.scope),
        // Explicit, so the answer's row count and its table agree.
        limit: question.limit ?? QUESTION_CARD_ROWS,
      }}
      baseCurrency={evaluation.context.currency}
    />
  );
}

// Why a sent edition shows no answer for a card.
function notKeptMessage(
  question: KeptQuestion | undefined,
  edition: KeptQuestions | undefined,
) {
  if (question?.status === "withheld") return "reporting.questionKeptWithheld";
  switch (question?.reason) {
    case "retention_unconfigured":
      return "reporting.questionNotKeptNoRetention";
    case "too_many_records":
      return "reporting.questionNotKeptTooMany";
    case "unanswered":
      return "reporting.questionNotKeptUnanswered";
    case "counts_other_records":
      return "reporting.questionNotKeptOtherRecords";
  }
  if (edition?.expired) return "reporting.questionExpired";
  if (edition?.redacted) return "reporting.questionRedacted";
  return "reporting.questionNotKept";
}

function QuestionAnswer({
  query,
  baseCurrency,
}: Readonly<{ query: AnalyticsQuery; baseCurrency: string }>) {
  const t = useT();
  const schema = useAnalyticsSchema();
  const answer = useQuery({
    queryKey: ["reporting-question-card", query],
    queryFn: async () =>
      unwrap(await api.POST("/analytics/query", { body: query })),
  });
  if (answer.isPending)
    return <PendingBody label={t("reporting.questionLoading")} />;
  if (answer.isError) return <QuestionFailure error={answer.error} />;
  return (
    <AnswerTable
      query={query}
      answer={answer.data}
      baseCurrency={baseCurrency}
      source={{ kind: "query", query }}
      fieldLabels={customLabels(t, schema.data?.entities ?? [])}
    />
  );
}

/**
 * A card bound to a saved question, asked on the server for this reader. Its
 * filters never reach the page, so the answer and its evidence come from the
 * report rather than from the question the card shows.
 */
function BoundQuestionAnswer({
  card,
  report,
  baseCurrency,
}: Readonly<{
  card: Card;
  report: ReportRevision | undefined;
  baseCurrency: string;
}>) {
  const t = useT();
  const schema = useAnalyticsSchema();
  const answer = useQuery({
    queryKey: [
      "reporting-bound-question",
      report?.id,
      report?.revision ?? null,
      card.id,
    ],
    enabled: report !== undefined,
    queryFn: async ({ signal }) =>
      unwrap(
        await api.GET("/analytics/reports/{id}/questions/{card_id}/answer", {
          params: {
            path: { id: report?.id ?? "", card_id: card.id },
            query: { revision: report?.revision },
          },
          signal,
        }),
      ),
  });
  if (report === undefined)
    return <EmptyState>{t("reporting.questionBoundUnsaved")}</EmptyState>;
  if (answer.isPending)
    return <PendingBody label={t("reporting.questionLoading")} />;
  if (answer.isError) return <QuestionFailure error={answer.error} />;
  if (answer.data.status === "restricted" || !answer.data.answer)
    return <EmptyState>{t("reporting.questionRestricted")}</EmptyState>;
  const question = card.question;
  if (!question) return null;
  return (
    <>
      <AnswerTable
        query={question}
        answer={answer.data.answer}
        baseCurrency={baseCurrency}
        fieldLabels={customLabels(t, schema.data?.entities ?? [])}
      />
      <BindingNote binding={answer.data.binding} />
    </>
  );
}

/** What a reader may know of a bound card's question: that it filters, and the calendar its periods keep. */
function BindingNote({
  binding,
}: Readonly<{ binding: BoundAnswer["binding"] }>) {
  const t = useT();
  const { locale } = useLocale();
  if (!binding?.filtered) return null;
  const basis = binding.period_basis;
  return (
    <p className="t-caption">
      {basis
        ? t("reporting.questionBoundPeriod", {
            month: monthName(basis.fiscal_year_start_month, locale),
            zone: basis.timezone,
          })
        : t("reporting.questionBoundFiltered")}
    </p>
  );
}

import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import { EmptyState, PendingBody } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
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
  ReportingEvaluation,
  ReportingSelection,
} from "./reporting.model";

type Card = ReportingLayout[number]["cards"][number];

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
 * grants and over the report's scope. A sent edition keeps no answer, so a
 * frozen report says so rather than answering with today's figures.
 */
export function QuestionCard({
  card,
  wide,
  evaluation,
  mode,
}: Readonly<{
  card: Card;
  wide: boolean;
  evaluation: ReportingEvaluation;
  // "frozen" for a sent edition; "preview" while the page shows the report as
  // a role reads it, which this browser's own session cannot ask as.
  mode: "live" | "frozen" | "preview";
}>) {
  const t = useT();
  const question = card.question;
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
        {mode !== "live" || !question ? (
          <EmptyState>
            {t(
              mode === "preview"
                ? "reporting.questionNotPreviewed"
                : "reporting.questionNotKept",
            )}
          </EmptyState>
        ) : (
          <QuestionAnswer
            query={{
              ...question,
              ...scopeOf(evaluation.selection.scope),
              // Explicit, so the answer's row count and its table agree.
              limit: question.limit ?? QUESTION_CARD_ROWS,
            }}
            baseCurrency={evaluation.context.currency}
          />
        )}
      </PanelBody>
    </Panel>
  );
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

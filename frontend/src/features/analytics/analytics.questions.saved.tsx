// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useInfiniteQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button, EmptyState } from "../../design-system/atoms";
import { DataTable } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatDate } from "../../format/format.dates";
import { type Translator, useLocale, useT } from "../../i18n";
import { openSavedQuestion } from "../../lib/analytics.address";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import {
  type AnalyticsQuery,
  analyticsFieldLabel,
  type CustomLabels,
  customLabels,
  entityLabel,
  measureLabel,
} from "./analytics.questions.vocab";
import { useAnalyticsSchema } from "./analytics.schema";

export const SAVED_QUESTIONS_KEY = ["analytics-saved-questions"] as const;

/**
 * The reader's own saved questions, newest first, a page at a time, about one
 * population when `entity` names it. Only the questions come back: opening one
 * asks it again for whoever opens it. Off until `enabled`, so a picker reads
 * nothing until it is opened.
 */
export function useSavedQuestions(entity?: string, enabled = true) {
  return useInfiniteQuery({
    queryKey: [...SAVED_QUESTIONS_KEY, entity ?? null],
    enabled,
    initialPageParam: null as string | null,
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/analytics/runs", {
          params: { query: { cursor: pageParam ?? undefined, entity } },
          signal,
        }),
      ),
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? null) : null,
  });
}

export type SavedQuestion = components["schemas"]["ReportRunSummary"];

/** The question a card copies from a saved one: its shape, never a filter. */
export function savedQuestionShape(run: SavedQuestion): AnalyticsQuery {
  return {
    entity: run.entity,
    group_by: run.group_by,
    measures: run.measures,
    limit: run.limit,
  };
}

/** A saved question named by what it asks, since it has no name of its own. */
export function savedQuestionLabel(
  t: Translator,
  query: Pick<AnalyticsQuery, "entity" | "group_by" | "measures">,
  fields: CustomLabels,
): string {
  const parts = [entityLabel(t, query.entity)];
  const groups = (query.group_by ?? []).map((field) =>
    analyticsFieldLabel(t, field, fields),
  );
  if (groups.length > 0) parts.push(groups.join(", "));
  parts.push(
    query.measures
      .map((measure) => measureLabel(t, measure, fields))
      .join(", "),
  );
  return parts.join(" · ");
}

export function SavedQuestions() {
  const t = useT();
  const { locale } = useLocale();
  const zone = useRecordZone();
  const saved = useSavedQuestions();
  const schema = useAnalyticsSchema();
  const fields = customLabels(t, schema.data?.entities ?? []);
  return (
    <Panel title={t("analytics.q.savedListTitle")}>
      <PanelBody>
        <QueryGate query={saved} pendingLabel={t("analytics.q.savedLoading")}>
          {(data) => {
            const runs = data.pages.flatMap((page) => page.data);
            if (runs.length === 0)
              return <EmptyState>{t("analytics.q.savedEmpty")}</EmptyState>;
            return (
              <>
                <DataTable
                  label={t("analytics.q.savedListTitle")}
                  rows={runs}
                  rowKey={(run) => run.id}
                  columns={[
                    {
                      key: "question",
                      header: t("analytics.q.savedQuestion"),
                      render: (run) => (
                        <button
                          type="button"
                          className="cell-link"
                          onClick={() => openSavedQuestion(run.id)}
                        >
                          {savedQuestionLabel(t, run, fields)}
                        </button>
                      ),
                    },
                    {
                      key: "asked",
                      header: t("analytics.q.savedAt"),
                      render: (run) => formatDate(run.asked_at, locale, zone),
                    },
                  ]}
                />
                {saved.hasNextPage && (
                  <div>
                    <Button
                      variant="ghost"
                      disabled={saved.isFetchingNextPage}
                      onClick={() => saved.fetchNextPage()}
                    >
                      {t("analytics.q.savedMore")}
                    </Button>
                  </div>
                )}
              </>
            );
          }}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

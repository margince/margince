// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "./analytics.css";
import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { WrittenBy } from "../../components/citations.list";
import { AiPending } from "../../design-system/aipending";
import { Button } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { useLocale, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { OutlookProse, readableAssessment } from "./analytics.assessment";
import type { ProgressAsk, SalesProgress } from "./progress.queries";

// "analytics" heads the key, so turning individual performance off drops the
// summary with the figures it was written from. The figures it rests on are
// in the key, so a reload of the page that changes none of them asks nothing.
function useProgressSummary(ask: ProgressAsk, progress: SalesProgress) {
  const [refresh, setRefresh] = useState(0);
  const { locale } = useLocale();
  const query = useQuery({
    queryKey: [
      "analytics",
      "progress-summary",
      ask,
      progress.closed.amount_minor,
      progress.goal.amount_minor,
      progress.planned.amount_minor,
      locale,
      refresh,
    ],
    queryFn: async ({ signal }) => {
      const result = await api.POST("/analytics/progress/summary", {
        body: {
          period: ask.period,
          year: ask.year,
          month: ask.period === "month" ? ask.month : undefined,
          scope_kind: ask.scope?.kind,
          scope_id: ask.scope?.id,
          refresh: refresh > 0,
        },
        signal,
      });
      // A server from before this route answers 404, and has no summary.
      if (result.response.status === 404) return null;
      return unwrap(result);
    },
    retry: false,
    // The last summary stays while its own reading is asked again, and never
    // stands in for another period, team member or team.
    placeholderData: (previous, last) =>
      JSON.stringify(last?.queryKey[2]) === JSON.stringify(ask)
        ? previous
        : undefined,
    staleTime: 60_000,
  });
  return { query, refresh: () => setRefresh((n) => n + 1) };
}

/**
 * A few written sentences on where the reading stands against its goal and
 * what is still open. Without a model, or with nothing to say, it draws
 * nothing and the page's figures stand alone.
 */
export function ProgressSummary({
  ask,
  progress,
}: Readonly<{ ask: ProgressAsk; progress: SalesProgress }>) {
  const t = useT();
  const { locale } = useLocale();
  const { query, refresh } = useProgressSummary(ask, progress);
  const summary = query.data;
  const written =
    summary?.generated_by === "model" &&
    summary.sentences.length > 0 &&
    readableAssessment(summary, locale);
  if (summary && written)
    return (
      <Panel
        title={t("progress.summary")}
        tone="ai"
        flatHead
        className="analytics-outlook"
        titleAction={
          <Button variant="ghost" onClick={refresh} pending={query.isFetching}>
            {t("progress.summary.refresh")}
          </Button>
        }
        footer={<WrittenBy by={summary.generated_by} />}
      >
        <PanelBody>
          <OutlookProse
            assessment={summary}
            more={t("progress.summary.more")}
          />
        </PanelBody>
      </Panel>
    );
  if (!query.isFetching && !query.isError) return null;
  return (
    <Panel title={t("progress.summary")} flatHead>
      <PanelBody>
        {query.isFetching ? (
          <AiPending label={t("progress.summary.writing")} lines={3} />
        ) : (
          <ErrorLine
            error={query.error}
            actions={
              <Button variant="ghost" onClick={refresh}>
                {t("analytics.assessment.retry")}
              </Button>
            }
          />
        )}
      </PanelBody>
    </Panel>
  );
}

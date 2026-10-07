import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import { useId, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { useCan } from "../app/capability";
import { routeHash } from "../app/router";
import { hashWithParams, useUrlParams } from "../app/urlstate";
import { Badge, Button, EmptyState, TableScroll } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import { tierLabel } from "./ai-decision-labels";
import { CallDetailPanel } from "./aicalls-detail";
import { QueryGate, QueryStates, throwProblem, useMe } from "./common";
import { settingsHref } from "./settingsrouting";
import "./aicalls.css";

// The trace's first page, as one query the card and the page header share.
//
// The header wants a single fact from it — when the runtime last called a model
// — and the unfiltered trace is where that fact lives, so it asks for the same
// window under the same key rather than opening a second read of the same
// endpoint. `task: ""` is the card's own no-filter state, which is why the two
// coincide exactly when the reader has filtered nothing.
function useCallTrace(task: string, filter: CallFilter, enabled: boolean) {
  return useInfiniteQuery({
    enabled,
    queryKey: ["ai-calls", task, filter],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/ai/calls", {
        params: {
          query: {
            cursor: pageParam ?? undefined,
            task: task || undefined,
            ...filter,
          },
        },
      });
      if (error) throwProblem(error);
      return data;
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
}

/**
 * When the runtime last reached a model — and, where there is no instant to
 * give, WHICH silence this is.
 *
 * `never` is a claim about the INSTALLATION: nothing has ever called a model.
 * The other three are claims about the READ — this reader may not make it, it
 * has not answered yet, it failed — and none of them is evidence about the
 * installation at all. One `null` over all four let a caller say "Never called"
 * over a trace that was still arriving, which is the defect this type exists to
 * make unspellable.
 *
 * `failed` is its own arm rather than folded into `unread` for the reason
 * `never` is its own: only one of the two resolves by waiting, and a reading
 * that goes silent on a broken read tells a reader nothing is wrong. Every
 * caller draws it; none may assume another surface will.
 */
export type LastCall =
  | { state: "withheld" }
  | { state: "unread" }
  | { state: "failed" }
  | { state: "never" }
  | { state: "at"; epochMs: number };

/**
 * The newest call, as a state a caller can switch on.
 *
 * Its OWN query rather than the card's. The card pages through a filtered trace
 * on demand; this wants the newest row and wants it to keep up, and the two
 * cannot share a key without one imposing its refetch on the other — a poll
 * over every page the reader had loaded.
 */
export function useLastCallAt(): LastCall {
  const canSee = useCan("ai_diagnostics", "read");
  const query = useQuery({
    enabled: canSee,
    queryKey: ["ai-call-latest"],
    // The header says how long ago the last call was, so the figure has to move
    // while the page is open. A minute is the resolution it reads at.
    refetchInterval: 60_000,
    queryFn: async () => {
      const { data, error } = await api.GET("/ai/calls", {});
      if (error) throwProblem(error);
      return data;
    },
  });
  // Read through the grant, not only around it. A revoked grant disables the
  // query but leaves its last answer in the cache, and answering from it would
  // go on showing a seat the runtime activity it may no longer see.
  if (!canSee) {
    return { state: "withheld" };
  }
  if (query.isError) {
    return { state: "failed" };
  }
  if (query.data === undefined) {
    return { state: "unread" };
  }
  const newest = query.data.data[0];
  return newest
    ? { state: "at", epochMs: Date.parse(newest.occurred_at) }
    : { state: "never" };
}

/** The address dial the trace is narrowed by, so a task row can link to its calls. */
export const CALL_TASK_PARAM = "task";

/**
 * The other dials a figure narrows the trace by — where a call ended — each
 * the `/ai/calls` filter of the same name.
 */
export const CALL_FILTER_PARAMS = [
  "provider",
  "model",
  "served_provider",
  "tier",
] as const;
type CallFilterParam = (typeof CALL_FILTER_PARAMS)[number];
export type CallFilter = Partial<
  Record<CallFilterParam | typeof CALL_TASK_PARAM, string>
>;

/** The trace narrowed to the calls behind one figure. */
export function callsHrefFor(filter: CallFilter): string {
  return hashWithParams(
    routeHash(settingsHref("model-calls")),
    new Map(Object.entries(filter)),
  );
}

export function AiCallsCard() {
  const t = useT();
  const { locale } = useLocale();
  const me = useMe();
  // Same seam as the spend card: `ai_diagnostics:read` (ai/callread.go). The
  // seat ceiling stays out of the question either way (capability.ts) — a read
  // seat may still read a diagnostic.
  const canSee = useCan("ai_diagnostics", "read");
  const zone = viewerZone();
  const [params, setParams] = useUrlParams();
  const task = params.get(CALL_TASK_PARAM) ?? "";
  const setTask = (next: string) => {
    const dials = new Map(params);
    dials.set(CALL_TASK_PARAM, next);
    setParams(dials);
  };
  const [expanded, setExpanded] = useState<string | null>(null);
  const filter: CallFilter = Object.fromEntries(
    CALL_FILTER_PARAMS.flatMap((key) => {
      const value = params.get(key);
      return value ? [[key, value]] : [];
    }),
  );
  const clearFilter = () => {
    const dials = new Map(params);
    for (const key of CALL_FILTER_PARAMS) dials.delete(key);
    setParams(dials);
  };
  const query = useCallTrace(task, filter, canSee);
  const calls = query.data?.pages.flatMap((page) => page.data) ?? [];
  const captureEnabled = query.data?.pages[0]?.payload_capture_enabled ?? false;
  // The filter options are the server's complete task set (carried on every
  // page), NOT the tasks on the loaded rows: deriving them from `calls` would
  // collapse the dropdown to the one selected task once a filter is applied.
  const listed = query.data?.pages[0]?.tasks ?? [];
  // A task reached by link may have no calls yet and so be absent from the
  // server's set; it stays selectable so the select shows what is filtered.
  const tasks = task && !listed.includes(task) ? [task, ...listed] : listed;

  if (!canSee) {
    // Withheld, not absent — the same choice the spend card above it makes. An
    // absent trace reads as "the installation made no model calls", which is a
    // claim about the data rather than about who may read it. No request either:
    // the query is disabled, because a settled denial is not a failure to retry.
    return (
      <Panel title={t("aicalls.title")}>
        <PanelBody>
          <PanelIntro>{t("aicalls.sub")}</PanelIntro>
          <QueryGate query={me} pendingLabel={t("aicalls.title")}>
            {() => <EmptyState>{t("aicalls.withheld")}</EmptyState>}
          </QueryGate>
        </PanelBody>
      </Panel>
    );
  }

  // No bottom margin of its own: `.settings-stack` owns the gap between cards.
  return (
    <Panel title={t("aicalls.title")}>
      <PanelBody>
        <PanelIntro>{t("aicalls.sub")}</PanelIntro>
        <QueryStates query={query} pendingLabel={t("aicalls.title")}>
          <SettingList>
            <SettingRow
              label={t("aicalls.col.task")}
              control={(control) => (
                <Select
                  {...control}
                  className="settingrow-measure"
                  value={task}
                  onChange={setTask}
                  // "All tasks" is a real option, not the select's placeholder: a
                  // reader who filtered to one task has to be able to come back.
                  // A task name is a wire value the server owns, so it is its own
                  // label — there is nothing to translate.
                  options={[
                    { value: "", label: t("aicalls.filter.all") },
                    ...tasks.map((value) => ({ value, label: value })),
                  ]}
                />
              )}
            />
            {Object.keys(filter).length > 0 ? (
              <Callout
                tone="info"
                title={t("aicalls.filtered", {
                  filter: Object.entries(filter)
                    .map(([key, value]) => `${key}: ${value}`)
                    .join(" · "),
                })}
              >
                <Button variant="link" onClick={clearFilter}>
                  {t("aicalls.filtered.clear")}
                </Button>
              </Callout>
            ) : null}
            <SettingRow
              label={t("aicalls.callsLabel")}
              layout="stack"
              control={
                // A column, because the page that follows the trace is under it
                // rather than beside it; `.settingrow-control` is a flex ROW, so
                // the two would otherwise sit shoulder to shoulder.
                <div className="form-stack settingrow-measure">
                  {calls.length === 0 ? (
                    <EmptyState>{t("aicalls.empty")}</EmptyState>
                  ) : (
                    // Every figure of a call stays — a call is only diagnosable
                    // with its model, its tokens and its latency side by side —
                    // but they share three columns, not six: the moment and the
                    // latency ride under the task and the tokens, and the model
                    // wraps. A table wider than its card scrolls, and an overlay
                    // scrollbar draws nothing, so it just looks cut off.
                    // `TableScroll` stays as the containment for a viewport too
                    // narrow for even this.
                    <TableScroll label={t("aicalls.callsLabel")}>
                      <table className="table aicalls-table">
                        <thead>
                          <tr>
                            {/* The disclosure column. Named rather than left
                              blank: a table that announces five headers for six
                              cells makes the reader count. */}
                            <th className="sr-only">
                              {t("aicalls.col.detail")}
                            </th>
                            <th>
                              {t("aicalls.col.task")} / {t("aicalls.col.when")}
                            </th>
                            <th>{t("aicalls.col.model")}</th>
                            <th>
                              {t("aicalls.col.tokens")} /{" "}
                              {t("aicalls.col.latency")}
                            </th>
                          </tr>
                        </thead>
                        <tbody>
                          {calls.map((call) => (
                            <FragmentRow
                              key={call.id}
                              call={call}
                              expanded={expanded === call.id}
                              captureEnabled={captureEnabled}
                              onToggle={() =>
                                setExpanded(
                                  expanded === call.id ? null : call.id,
                                )
                              }
                              when={formatDateTime(
                                call.occurred_at,
                                locale,
                                zone,
                              )}
                              tokens={`${formatNumber(call.tokens_in, locale)} / ${formatNumber(call.tokens_out, locale)}`}
                            />
                          ))}
                        </tbody>
                      </table>
                    </TableScroll>
                  )}
                  {query.hasNextPage && (
                    <div>
                      <Button
                        disabled={query.isFetchingNextPage}
                        onClick={() => void query.fetchNextPage()}
                      >
                        {t("aicalls.loadMore")}
                      </Button>
                    </div>
                  )}
                </div>
              }
            />
          </SettingList>
        </QueryStates>
      </PanelBody>
    </Panel>
  );
}

function FragmentRow({
  call,
  expanded,
  captureEnabled,
  onToggle,
  when,
  tokens,
}: Readonly<{
  call: components["schemas"]["AiCallSummary"];
  expanded: boolean;
  captureEnabled: boolean;
  onToggle: () => void;
  when: string;
  tokens: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const panelId = useId();
  // The attempts the ladder itself made. A decision model that fell back is
  // the first of `calls_attempted`, and the rung after it is a second model
  // asked, not a retry of the first — so it does not count toward the badge.
  const ladderAttempts = call.decision_attempted
    ? call.calls_attempted - 1
    : call.calls_attempted;
  return (
    <>
      {/* The disclosure is a real button in the first cell, not a click handler on
          the row. A `<tr onClick>` is reachable by pointer alone: it takes no
          focus, answers no key, and announces no state — so the attempt trail
          behind it, which is the whole reason this table expands, was unreachable
          by keyboard and to a screen reader. The subject-request queue
          (screens/privacy.tsx) is the shape this follows. */}
      <tr>
        <td>
          {/* NOT a `Disclosure`: that primitive is a `<details>` element, and
              what opens here is the NEXT table row, which no element can
              contain from inside a cell of the row above it. So the trigger
              stays a real button carrying the expanded state in ARIA, and the
              chevron is turned by that same attribute, by the catalog's own
              `.expander-chevron` — `iconOnly` because the glyph is its whole
              label, which is what makes the control square instead of a pill
              around 14px. */}
          <Button
            iconOnly
            variant="ghost"
            aria-expanded={expanded}
            aria-controls={expanded ? panelId : undefined}
            // Named by the call it opens, not "Show detail": a page of twenty
            // rows would otherwise offer twenty identically-named buttons.
            aria-label={t("aicalls.expandCall", { task: call.task, when })}
            onClick={onToggle}
          >
            {/* No `size=`: `.btn svg` already sizes a button's icon child
                (base.css), and a size at the call site is the drift that rule
                exists to stop. */}
            <ChevronDown className="expander-chevron" aria-hidden />
          </Button>
        </td>
        <td>
          {call.task}
          <div className="t-caption">{when}</div>
          <div className="aicalls-badges">
            {/* The logical call's flag, not this row's kind: a fallback's
                terminal row is the completion that answered after the
                decision model was asked. */}
            {call.decision_attempted && (
              <Badge>{t("aicalls.badge.decision")}</Badge>
            )}
            {call.cache_hit && <Badge>{t("aicalls.badge.cacheHit")}</Badge>}
            {call.degraded && (
              <Badge tone="warning">{t("aicalls.badge.degraded")}</Badge>
            )}
            {call.error_sentinel && (
              <Badge tone="danger">{call.error_sentinel}</Badge>
            )}
            {ladderAttempts > 1 && (
              <Badge>
                {t("aicalls.badge.retries", {
                  count: formatNumber(ladderAttempts, locale),
                })}
              </Badge>
            )}
          </div>
        </td>
        <td>
          {tierLabel(call.tier, t)} · {call.provider}/{call.served_model}
        </td>
        <td>
          {tokens}
          <div className="t-caption">
            {t("aicalls.ms", { value: formatNumber(call.latency_ms, locale) })}
          </div>
        </td>
      </tr>
      {expanded && (
        <tr>
          <td colSpan={4} id={panelId}>
            <CallDetailPanel id={call.id} captureEnabled={captureEnabled} />
          </td>
        </tr>
      )}
    </>
  );
}

import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { api, FIRST_PAGE } from "../api/client";
import { useCan } from "../app/capability";
import { routeHash } from "../app/router";
import { hashWithParams, useUrlParams } from "../app/urlstate";
import { Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { tierLabel } from "./ai-decision-labels";
import { providerName } from "./ai-provider-names";
import { CallTable } from "./aicalls-table";
import {
  LoadMoreButton,
  QueryGate,
  QueryStates,
  unwrap,
  useMe,
} from "./common";
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
      return unwrap(
        await api.GET("/ai/calls", {
          params: {
            query: {
              cursor: pageParam ?? undefined,
              task: task || undefined,
              ...filter,
            },
          },
        }),
      );
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
      return unwrap(await api.GET("/ai/calls", {}));
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
  const me = useMe();
  // Same seam as the spend card: `ai_diagnostics:read` (ai/callread.go). The
  // seat ceiling stays out of the question either way (capability.ts) — a read
  // seat may still read a diagnostic.
  const canSee = useCan("ai_diagnostics", "read");
  const [params, setParams] = useUrlParams();
  const task = params.get(CALL_TASK_PARAM) ?? "";
  const setTask = (next: string) => {
    const dials = new Map(params);
    dials.set(CALL_TASK_PARAM, next);
    setParams(dials);
  };
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
  // The server's complete task set, carried on every page: the tasks on the
  // loaded rows collapse to the one selected once a filter applies.
  const listed = query.data?.pages[0]?.task_options ?? [];
  // A task reached by link may have no calls yet and so be absent from the
  // server's set; it stays selectable so the select shows what is filtered.
  const options =
    task && !listed.some((option) => option.task === task)
      ? [{ task }, ...listed]
      : listed;

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
    <Panel title={t("aicalls.title")} className="aicalls-card">
      <PanelBody>
        <PanelIntro>{t("aicalls.sub")}</PanelIntro>
        {query.data === undefined && (
          <QueryStates query={query} pendingLabel={t("aicalls.title")}>
            {null}
          </QueryStates>
        )}
      </PanelBody>
      {query.data !== undefined && (
        <>
          <SettingList bleed="settings">
            <SettingRow
              label={t("aicalls.col.task")}
              control={(control) => (
                <Select
                  {...control}
                  className="settingrow-measure"
                  value={task}
                  onChange={setTask}
                  // "All tasks" is a real option, not the placeholder: a reader
                  // who filtered to one task has to be able to come back.
                  options={[
                    { value: "", label: t("aicalls.filter.all") },
                    ...options.map((option) => ({
                      value: option.task,
                      label: option.display_name ?? option.task,
                    })),
                  ]}
                />
              )}
            />
          </SettingList>
          {Object.keys(filter).length > 0 && (
            <PanelBody>
              <Callout
                tone="info"
                title={t("aicalls.filtered", {
                  filter: filterWords(filter, t),
                })}
              >
                <Button variant="link" onClick={clearFilter}>
                  {t("aicalls.filtered.clear")}
                </Button>
              </Callout>
            </PanelBody>
          )}
          <PanelGroupHead title={t("aicalls.callsLabel")} level="h3" />
          {calls.length === 0 ? (
            <PanelBody>
              <EmptyState>{t("aicalls.empty")}</EmptyState>
            </PanelBody>
          ) : (
            <CallTable calls={calls} captureEnabled={captureEnabled} />
          )}
          {query.hasNextPage && (
            <PanelBody>
              <LoadMoreButton query={query} />
            </PanelBody>
          )}
        </>
      )}
    </Panel>
  );
}

const FILTER_TERM = {
  provider: "aiTerms.provider",
  model: "aicalls.col.model",
  served_provider: "aicalls.filter.servedProvider",
  tier: "aiTerms.tier",
} as const satisfies Record<CallFilterParam, MessageKey>;

// Each dial under its own name, with a vendor or a tier named as the rest of
// this page names them. A model id is its own name.
function filterWords(filter: CallFilter, t: Translator): string {
  return CALL_FILTER_PARAMS.flatMap((key) => {
    const value = filter[key];
    if (!value) return [];
    const shown =
      key === "provider"
        ? providerName(value, t)
        : key === "tier"
          ? tierLabel(value, t)
          : value;
    return [`${t(FILTER_TERM[key])}: ${shown}`];
  }).join(" · ");
}

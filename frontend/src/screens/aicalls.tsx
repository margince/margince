import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import { Button, EmptyState } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
} from "../design-system/panel";
import {
  LoadMoreButton,
  QueryGate,
  QueryStates,
} from "../design-system/querystates";
import { type Translator, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useCan } from "../lib/capability";
import { unwrap } from "../lib/problem";
import { routeHash } from "../lib/router";
import { useMe } from "../lib/session";
import { hashWithParams, type UrlParams, useUrlParams } from "../lib/urlstate";
import { tierLabel } from "./ai-decision-labels";
import { providerName } from "./ai-provider-names";
import { type AiCallRecord, CallDrawer } from "./aicalls-detail";
import {
  CALL_PARAMS,
  type CallFilters,
  CallFiltersBar,
  callFiltersOf,
  callQueryOf,
  isNarrowed,
  UNFILTERED_CALLS,
  withCallFilters,
} from "./aicalls-filters";
import { CallTable } from "./aicalls-table";
import { ExportScenarioDialog } from "./aiexport";
import { SEARCH_DEBOUNCE_MS } from "./listquery";
import { settingsHref } from "./settingsrouting";
import "./aicalls.css";

/** The address dial the trace is narrowed by, so a task row can link to its calls. */
export const CALL_TASK_PARAM = CALL_PARAMS.task;

/**
 * The dials a figure elsewhere narrows the trace by, each the `/ai/calls`
 * filter of the same name. `model` is also the toolbar's own model dial.
 */
const CALL_FILTER_PARAMS = [
  "provider",
  "model",
  "served_provider",
  "tier",
] as const;
type CallFilterParam = (typeof CALL_FILTER_PARAMS)[number];
export type CallFilter = Partial<
  Record<CallFilterParam | typeof CALL_TASK_PARAM, string>
>;

// The figure dials the toolbar has no control for, so they are named in a
// callout with their own way back.
const FIGURE_PARAMS = ["provider", "served_provider", "tier"] as const;
type FigureParam = (typeof FIGURE_PARAMS)[number];
type FigureFilter = Partial<Record<FigureParam, string>>;

/** The trace narrowed to the calls behind one figure. */
export function callsHrefFor(filter: CallFilter): string {
  return hashWithParams(
    routeHash(settingsHref("model-calls")),
    new Map(Object.entries(filter)),
  );
}

function figureFilterOf(params: UrlParams): FigureFilter {
  return Object.fromEntries(
    FIGURE_PARAMS.flatMap((key) => {
      const value = params.get(key);
      return value ? [[key, value]] : [];
    }),
  );
}

// The trace as the server narrows it. Every dial is in the key, so a changed
// filter is a fresh walk from the newest call rather than a cursor minted under
// another question.
function useCallTrace(
  filters: CallFilters,
  figure: FigureFilter,
  enabled: boolean,
) {
  return useInfiniteQuery({
    enabled,
    queryKey: ["ai-calls", filters, figure],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/ai/calls", {
          params: {
            query: {
              ...callQueryOf(filters, Date.now()),
              ...figure,
              cursor: pageParam ?? undefined,
            },
          },
        }),
      ),
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });
}

/**
 * When the runtime last reached a model and, where there is no instant to
 * give, WHICH silence this is.
 *
 * `never` is a claim about the INSTALLATION: nothing has ever called a model.
 * The other three are claims about the READ (this reader may not make it, it
 * has not answered yet, it failed), and none of them is evidence about the
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
 * cannot share a key without one imposing its refetch on the other: a poll
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
    queryFn: async () => unwrap(await api.GET("/ai/calls", {})),
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

function CallTrace({
  query,
  narrowed,
  taskOnly,
  onClear,
}: Readonly<{
  query: ReturnType<typeof useCallTrace>;
  narrowed: boolean;
  taskOnly: boolean;
  onClear: () => void;
}>) {
  const t = useT();
  const [openId, setOpenId] = useState<string | null>(null);
  const [exporting, setExporting] = useState<AiCallRecord | null>(null);
  const rows = query.data?.pages.flatMap((page) => page.data) ?? [];
  const captureEnabled = query.data?.pages[0]?.payload_capture_enabled ?? false;
  const at = rows.findIndex((call) => call.id === openId);
  const open = at >= 0 ? rows[at] : undefined;
  const step = (offset: number) => {
    const next = rows[at + offset];
    return next ? () => setOpenId(next.id) : undefined;
  };

  if (rows.length === 0) {
    // Under a filter the empty trace is a fact about the filter, not about the
    // installation, and the way back is beside it.
    return (
      <PanelBody>
        {narrowed ? (
          <EmptyState>
            <p>{t(taskOnly ? "aicalls.taskEmpty" : "aicalls.filteredEmpty")}</p>
            <Button onClick={onClear}>{t("aicalls.clear")}</Button>
          </EmptyState>
        ) : (
          <EmptyState>{t("aicalls.empty")}</EmptyState>
        )}
      </PanelBody>
    );
  }
  return (
    <>
      {narrowed && (
        <PanelBody>
          <div className="aicalls-countline">
            <Button variant="link" onClick={onClear}>
              {t("aicalls.clear")}
            </Button>
          </div>
        </PanelBody>
      )}
      <PanelGroupHead title={t("aicalls.callsLabel")} level="h3" />
      <CallTable calls={rows} onOpen={(call) => setOpenId(call.id)} />
      {query.hasNextPage && (
        <PanelBody>
          <LoadMoreButton query={query} />
        </PanelBody>
      )}
      {open && (
        <CallDrawer
          call={open}
          captureEnabled={captureEnabled}
          onClose={() => setOpenId(null)}
          onPrev={step(-1)}
          onNext={step(1)}
          onExport={(detail) => {
            // One dialog at a time: the export opens in place of the drawer
            // rather than over it, which would stack two Escape handlers.
            setOpenId(null);
            setExporting(detail);
          }}
        />
      )}
      {exporting && (
        <ExportScenarioDialog
          call={exporting}
          onClose={() => {
            setExporting(null);
            setOpenId(exporting.id);
          }}
        />
      )}
    </>
  );
}

function withSelected(listed: readonly string[], selected: string) {
  return selected && !listed.includes(selected)
    ? [selected, ...listed]
    : listed;
}

/**
 * The search box as typed, written to the address once typing settles. The
 * address is the query key, so writing every keystroke would be one request
 * per character with answers free to land out of order. An address that moves
 * on its own (Back, a cleared filter) moves the box with it; trimmed equality
 * keeps the box from eating a space the reader is still typing past.
 */
function useSettledSearch(
  settled: string,
  onSettle: (search: string) => void,
): [string, (typed: string) => void] {
  const [typed, setTyped] = useState(settled);
  const [seen, setSeen] = useState(settled);
  if (settled !== seen) {
    setSeen(settled);
    if (settled.trim() !== typed.trim()) setTyped(settled);
  }
  useEffect(() => {
    if (typed.trim() === settled.trim()) return;
    const timer = setTimeout(() => onSettle(typed), SEARCH_DEBOUNCE_MS);
    return () => clearTimeout(timer);
  }, [typed, settled, onSettle]);
  return [typed, setTyped];
}

export function AiCallsCard() {
  const t = useT();
  const me = useMe();
  // `ai_diagnostics:read` is what `GET /ai/calls` asks for (ai/callread.go).
  // The seat ceiling stays out of it: a read seat may read a diagnostic.
  const canSee = useCan("ai_diagnostics", "read");
  const [params, setParams] = useUrlParams();
  const filters = useMemo(() => callFiltersOf(params), [params]);
  const figure = useMemo(() => figureFilterOf(params), [params]);
  const setFilters = useCallback(
    (next: CallFilters) => setParams(withCallFilters(params, next)),
    [params, setParams],
  );
  const clearFigure = () => {
    const dials = new Map(params);
    for (const key of FIGURE_PARAMS) dials.delete(key);
    setParams(dials);
  };
  const clearAll = () => {
    const dials = new Map(withCallFilters(params, UNFILTERED_CALLS));
    for (const key of FIGURE_PARAMS) dials.delete(key);
    setParams(dials);
  };
  const [search, setSearch] = useSettledSearch(
    filters.search,
    useCallback(
      (typed) => setFilters({ ...filters, search: typed }),
      [filters, setFilters],
    ),
  );
  const query = useCallTrace(filters, figure, canSee);
  // The server's complete sets, carried on every page, NOT the values on the
  // loaded rows: those collapse to the one selected once a dial is set.
  const first = query.data?.pages[0];
  // A value reached by link may have no calls yet and so be absent from the
  // server's set; it stays selectable so the select shows what is filtered.
  const listedTasks = first?.task_options ?? [];
  const tasks =
    filters.task && !listedTasks.some((option) => option.task === filters.task)
      ? [{ task: filters.task }, ...listedTasks]
      : listedTasks;
  const models = withSelected(first?.models ?? [], filters.model);
  const hasFigure = Object.keys(figure).length > 0;
  const narrowed = isNarrowed(filters) || hasFigure;
  const taskOnly =
    !hasFigure && !isNarrowed({ ...filters, task: "" }) && filters.task !== "";

  if (!canSee) {
    // Withheld, not absent: an absent trace reads as "the installation made no
    // model calls". No request either, because a settled denial is no failure.
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
        <CallFiltersBar
          tasks={tasks}
          models={models}
          filters={filters}
          search={search}
          onSearch={setSearch}
          onChange={setFilters}
        />
        {hasFigure && (
          <Callout
            tone="info"
            title={t("aicalls.filtered", { filter: figureWords(figure, t) })}
          >
            <Button variant="link" onClick={clearFigure}>
              {t("aicalls.filtered.clear")}
            </Button>
          </Callout>
        )}
        {query.data === undefined && (
          <QueryStates
            query={query}
            pendingLabel={t("aicalls.title")}
            pendingLines={6}
          >
            {null}
          </QueryStates>
        )}
      </PanelBody>
      {query.data !== undefined && (
        <CallTrace
          query={query}
          narrowed={narrowed}
          taskOnly={taskOnly}
          onClear={clearAll}
        />
      )}
    </Panel>
  );
}

const FIGURE_TERM = {
  provider: "aiTerms.provider",
  served_provider: "aicalls.filter.servedProvider",
  tier: "aiTerms.tier",
} as const satisfies Record<FigureParam, MessageKey>;

// Each dial under its own name, with a vendor or a tier named as the rest of
// this page names them. An upstream host is its own name.
function figureWords(figure: FigureFilter, t: Translator): string {
  return FIGURE_PARAMS.flatMap((key) => {
    const value = figure[key];
    if (!value) return [];
    const shown =
      key === "provider"
        ? providerName(value, t)
        : key === "tier"
          ? tierLabel(value, t)
          : value;
    return [`${t(FIGURE_TERM[key])}: ${shown}`];
  }).join(" · ");
}

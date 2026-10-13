import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useCallback, useEffect, useMemo, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import { useCan } from "../app/capability";
import { useUrlParams } from "../app/urlstate";
import { Badge, Button, EmptyState } from "../design-system/atoms";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { formatDateTime, formatNumber } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
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
import {
  type AiCallSummary,
  costLabel,
  modelLabel,
  OUTCOME_TONE,
  outcomeLabel,
  taskLabel,
} from "./aicalls-vocab";
import { ExportScenarioDialog } from "./aiexport";
import {
  LoadMoreButton,
  QueryGate,
  QueryStates,
  throwProblem,
  useMe,
} from "./common";
import { SEARCH_DEBOUNCE_MS } from "./listquery";
import "./aicalls.css";

// The trace as the server narrows it. Every dial is in the key, so a changed
// filter is a fresh walk from the newest call rather than a cursor minted under
// another question.
function useCallTrace(filters: CallFilters, enabled: boolean) {
  return useInfiniteQuery({
    enabled,
    queryKey: ["ai-calls", filters],
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/ai/calls", {
        params: {
          query: {
            ...callQueryOf(filters, Date.now()),
            cursor: pageParam ?? undefined,
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
export const CALL_TASK_PARAM = CALL_PARAMS.task;

function useCallColumns(
  onOpen: (call: AiCallSummary) => void,
): DataTableColumn<AiCallSummary>[] {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const n = (value: number) => formatNumber(value, locale);
  return [
    {
      key: "when",
      header: t("aicalls.col.when"),
      render: (call) => (
        <time className="aicalls-when" dateTime={call.occurred_at}>
          {formatDateTime(call.occurred_at, locale, zone)}
        </time>
      ),
    },
    {
      key: "feature",
      header: t("aicalls.col.feature"),
      // The row answers a pointer too, but a `<tr>` takes no focus and
      // announces nothing, so the feature is the button a keyboard opens by.
      render: (call) => {
        const when = formatDateTime(call.occurred_at, locale, zone);
        return (
          <CellStack>
            <Button
              variant="link"
              className="aicalls-feature"
              aria-label={t("aicalls.open", {
                feature: taskLabel(call.task),
                when,
              })}
              onClick={(event) => {
                event.stopPropagation();
                onOpen(call);
              }}
            >
              {taskLabel(call.task)}
            </Button>
            <time
              className="t-caption aicalls-when-inline"
              dateTime={call.occurred_at}
            >
              {when}
            </time>
          </CellStack>
        );
      },
    },
    {
      key: "model",
      header: t("aicalls.col.model"),
      render: (call) => (
        <CellStack>
          <span className="aicalls-clip" title={modelLabel(call)}>
            {modelLabel(call)}
          </span>
          <span className="t-caption aicalls-clip">{call.provider}</span>
        </CellStack>
      ),
    },
    {
      key: "outcome",
      header: t("aicalls.col.outcome"),
      render: (call) => (
        <Badge tone={OUTCOME_TONE[call.outcome]}>
          {outcomeLabel(call.outcome, t)}
        </Badge>
      ),
    },
    {
      key: "tokens",
      header: t("aicalls.col.tokens"),
      align: "end",
      render: (call) => (
        <span
          title={t("aicalls.fact.tokensInOut", {
            in: n(call.tokens_in),
            out: n(call.tokens_out),
          })}
        >
          {n(call.tokens_in + call.tokens_out)}
        </span>
      ),
    },
    {
      key: "cost",
      header: t("aicalls.col.cost"),
      align: "end",
      render: (call) =>
        call.estimated_cost_microusd === undefined ? (
          <span className="t-caption" title={t("aicalls.cost.unpricedHint")}>
            {costLabel(call, locale, t)}
          </span>
        ) : (
          costLabel(call, locale, t)
        ),
    },
    {
      key: "latency",
      header: t("aicalls.col.latency"),
      align: "end",
      render: (call) => t("aicalls.ms", { value: n(call.latency_ms) }),
    },
  ];
}

function CallTrace({
  query,
  filters,
  onClear,
}: Readonly<{
  query: ReturnType<typeof useCallTrace>;
  filters: CallFilters;
  onClear: () => void;
}>) {
  const t = useT();
  const [openId, setOpenId] = useState<string | null>(null);
  const [exporting, setExporting] = useState<AiCallRecord | null>(null);
  const calls = query.data?.pages.flatMap((page) => page.data) ?? [];
  const captureEnabled = query.data?.pages[0]?.payload_capture_enabled ?? false;
  const rows = calls;
  const columns = useCallColumns((call) => setOpenId(call.id));
  const narrowed = isNarrowed(filters);
  const at = rows.findIndex((call) => call.id === openId);
  const open = at >= 0 ? rows[at] : undefined;
  const step = (offset: number) => {
    const next = rows[at + offset];
    return next ? () => setOpenId(next.id) : undefined;
  };

  if (rows.length === 0) {
    // Under a filter the empty trace is a fact about the filter, not about the
    // installation, and the way back is beside it.
    if (!narrowed) {
      return <EmptyState>{t("aicalls.empty")}</EmptyState>;
    }
    const taskOnly = isNarrowed({ ...filters, task: "" }) === false;
    return (
      <EmptyState>
        <p>{t(taskOnly ? "aicalls.taskEmpty" : "aicalls.filteredEmpty")}</p>
        <Button onClick={onClear}>{t("aicalls.clear")}</Button>
      </EmptyState>
    );
  }
  return (
    <>
      {narrowed && (
        <div className="aicalls-countline">
          <Button variant="link" onClick={onClear}>
            {t("aicalls.clear")}
          </Button>
        </div>
      )}
      <div className="aicalls-table">
        <DataTable
          label={t("aicalls.callsLabel")}
          columns={columns}
          rows={rows}
          rowKey={(call) => call.id}
          onRowClick={(call) => setOpenId(call.id)}
        />
      </div>
      <div className="aicalls-more">
        <LoadMoreButton query={query} />
      </div>
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
  const setFilters = useCallback(
    (next: CallFilters) => setParams(withCallFilters(params, next)),
    [params, setParams],
  );
  const [search, setSearch] = useSettledSearch(
    filters.search,
    useCallback(
      (typed) => setFilters({ ...filters, search: typed }),
      [filters, setFilters],
    ),
  );
  const query = useCallTrace(filters, canSee);
  // The server's complete sets, carried on every page, NOT the values on the
  // loaded rows: those collapse to the one selected once a dial is set.
  const first = query.data?.pages[0];
  // A value reached by link may have no calls yet and so be absent from the
  // server's set; it stays selectable so the select shows what is filtered.
  const tasks = withSelected(first?.tasks ?? [], filters.task);
  const models = withSelected(first?.models ?? [], filters.model);

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

  return (
    <Panel title={t("aicalls.title")}>
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
        <QueryStates
          query={query}
          pendingLabel={t("aicalls.title")}
          pendingLines={6}
        >
          <CallTrace
            query={query}
            filters={filters}
            onClear={() => setFilters(UNFILTERED_CALLS)}
          />
        </QueryStates>
      </PanelBody>
    </Panel>
  );
}

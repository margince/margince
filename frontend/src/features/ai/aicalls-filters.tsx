import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Field, SearchField } from "../../design-system/atoms";
import { FilterBar } from "../../design-system/filterbar";
import { Select } from "../../design-system/select";
import { stable } from "../../format/collate";
import { useT } from "../../i18n";
import { aiTaskLabel } from "../../lib/ai-task-label";
import type { UrlParams } from "../../lib/urlstate";
import { CALL_OUTCOMES, type CallOutcome, outcomeLabel } from "./aicalls-vocab";

type TaskOption = components["schemas"]["AiCallTaskOption"];

const WINDOWS = { any: 0, hour: 3_600_000, day: 86_400_000, week: 604_800_000 };
type CallWindow = keyof typeof WINDOWS;

function isWindow(value: string): value is CallWindow {
  return value in WINDOWS;
}

function isOutcome(value: string): value is CallOutcome {
  return CALL_OUTCOMES.some((outcome) => outcome === value);
}

/**
 * The dials `GET /ai/calls` narrows by, all held in the address so a filtered
 * trace is a link. The window is kept as its name, not an instant. The instant
 * is cut when the request runs, so the query key does not move every render.
 */
export type CallFilters = Readonly<{
  task: string;
  window: CallWindow;
  outcome: CallOutcome | "";
  model: string;
  search: string;
}>;

export const UNFILTERED_CALLS: CallFilters = {
  task: "",
  window: "any",
  outcome: "",
  model: "",
  search: "",
};

/** The address keys, so a task row elsewhere can link to its calls. */
export const CALL_PARAMS = {
  task: "task",
  window: "window",
  outcome: "outcome",
  model: "model",
  search: "q",
} as const satisfies Record<keyof CallFilters, string>;

export function callFiltersOf(params: UrlParams): CallFilters {
  const window = params.get(CALL_PARAMS.window) ?? "";
  const outcome = params.get(CALL_PARAMS.outcome) ?? "";
  return {
    task: params.get(CALL_PARAMS.task) ?? "",
    window: isWindow(window) ? window : "any",
    outcome: isOutcome(outcome) ? outcome : "",
    model: params.get(CALL_PARAMS.model) ?? "",
    search: params.get(CALL_PARAMS.search) ?? "",
  };
}

/** `params` with the trace's dials replaced and every other dial kept. */
export function withCallFilters(
  params: UrlParams,
  filters: CallFilters,
): UrlParams {
  const next = new Map(params);
  next.set(CALL_PARAMS.task, filters.task);
  next.set(CALL_PARAMS.window, filters.window === "any" ? "" : filters.window);
  next.set(CALL_PARAMS.outcome, filters.outcome);
  next.set(CALL_PARAMS.model, filters.model);
  next.set(CALL_PARAMS.search, filters.search.trim());
  return next;
}

export function isNarrowed(filters: CallFilters): boolean {
  return (
    filters.task !== "" ||
    filters.window !== "any" ||
    filters.outcome !== "" ||
    filters.model !== "" ||
    filters.search.trim() !== ""
  );
}

/** The filters as the request's query, with the window cut at `now`. */
export function callQueryOf(filters: CallFilters, now: number) {
  const span = WINDOWS[filters.window];
  const search = filters.search.trim();
  return {
    ...(filters.task ? { task: filters.task } : {}),
    ...(span ? { from: new Date(now - span).toISOString() } : {}),
    ...(filters.outcome ? { outcome: filters.outcome } : {}),
    ...(filters.model ? { model: filters.model } : {}),
    ...(search ? { q: search } : {}),
  };
}

export function CallFiltersBar({
  tasks,
  models,
  filters,
  search,
  onSearch,
  onChange,
  actions,
  caption,
}: Readonly<{
  tasks: readonly TaskOption[];
  models: readonly string[];
  filters: CallFilters;
  search: string;
  onSearch: (search: string) => void;
  onChange: (filters: CallFilters) => void;
  /** Clear, once a dial is set. */
  actions?: ReactNode;
  /** What the dials selected: the count, and any figure the trace is cut to. */
  caption?: ReactNode;
}>) {
  const t = useT();
  return (
    <FilterBar
      label={t("aicalls.filtersLabel")}
      actions={actions}
      caption={caption}
    >
      <Field label={t("aicalls.filter.search")}>
        {(control) => (
          <SearchField
            {...control}
            value={search}
            placeholder={t("aicalls.filter.searchPlaceholder")}
            onChange={(event) => onSearch(event.target.value)}
          />
        )}
      </Field>
      <Field label={t("aicalls.col.feature")}>
        {(control) => (
          <Select
            {...control}
            value={filters.task}
            onChange={(task) => onChange({ ...filters, task })}
            // "All features" is a real option, so a reader who narrowed to one
            // can come back. A task with no name of its own shows its key.
            options={[
              { value: "", label: t("aicalls.filter.all") },
              ...tasks.map((option) => ({
                value: option.task,
                label: aiTaskLabel(option.task, t, option.display_name),
              })),
            ]}
          />
        )}
      </Field>
      <Field label={t("aicalls.filter.window")}>
        {(control) => (
          <Select
            {...control}
            value={filters.window}
            onChange={(value) =>
              onChange({
                ...filters,
                window: isWindow(value) ? value : "any",
              })
            }
            options={[
              { value: "any", label: t("aicalls.window.any") },
              { value: "hour", label: t("aicalls.window.hour") },
              { value: "day", label: t("aicalls.window.day") },
              { value: "week", label: t("aicalls.window.week") },
            ]}
          />
        )}
      </Field>
      <Field label={t("aicalls.col.outcome")}>
        {(control) => (
          <Select
            {...control}
            value={filters.outcome}
            onChange={(value) =>
              onChange({ ...filters, outcome: isOutcome(value) ? value : "" })
            }
            options={[
              { value: "", label: t("aicalls.filter.anyOutcome") },
              ...CALL_OUTCOMES.map((value) => ({
                value,
                label: outcomeLabel(value, t),
              })),
            ]}
          />
        )}
      </Field>
      <Field label={t("aicalls.col.model")}>
        {(control) => (
          <Select
            {...control}
            value={filters.model}
            onChange={(model) => onChange({ ...filters, model })}
            options={[
              { value: "", label: t("aicalls.filter.anyModel") },
              ...[...models]
                .sort(stable)
                .map((value) => ({ value, label: value })),
            ]}
          />
        )}
      </Field>
    </FilterBar>
  );
}

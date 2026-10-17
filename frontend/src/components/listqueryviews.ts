import { useState } from "react";
import type { ListChip, ListView } from "../design-system/listtable";
import type { useT } from "../i18n";
import { withoutStrandedTagMode } from "../lib/tagfilter";
import type { FilterSpec, RailView, ViewSpec } from "./listquery";
import type { ListQuery } from "./listqueryparams";

type Translate = ReturnType<typeof useT>;

/**
 * The filters after the reader picks `value` on the chip `key` ("" clears it).
 *
 * A chip whose options name different query parameters carries the parameter
 * in the value, as `param:value` (the owner dial). Its key names no parameter,
 * so clearing it drops whichever of its own parameters is set. Clearing every
 * composite parameter would let picking a lifecycle drop the owner filter.
 */
export function pickedFilters(
  chips: readonly ListChip[],
  current: Readonly<Record<string, string>>,
  key: string,
  value: string,
): Record<string, string> {
  const filters = { ...current };
  const owned = chips.filter((chip) => chip.key === key);
  const mine = owned
    .flatMap((chip) => chip.options.map((option) => option.value))
    .filter((candidate) => candidate.includes(":"))
    .map((candidate) => candidate.slice(0, candidate.indexOf(":")));
  for (const param of mine) {
    delete filters[param];
  }
  if (!value) {
    delete filters[key];
    return withoutStrandedTagMode(filters);
  }
  // A dial the server refuses beside this one gives way to it, so picking a
  // team after an owner swaps the narrowing instead of answering 422.
  for (const param of owned.flatMap((chip) => chip.excludes ?? [])) {
    delete filters[param];
  }
  const split = value.indexOf(":");
  if (split > 0 && mine.includes(value.slice(0, split))) {
    filters[value.slice(0, split)] = value.slice(split + 1);
  } else {
    filters[key] = value;
  }
  // A mode with no tag left to combine goes with it. Nothing draws it, so it
  // would sit in the address unseen and narrow the next tag the reader picks.
  return withoutStrandedTagMode(filters);
}

export function translateChip(chip: FilterSpec, t: Translate): ListChip {
  return {
    key: chip.key,
    label: t(chip.label),
    allLabel: t(chip.allLabel),
    options: chip.options.map((option) => ({
      value: option.value,
      label: "text" in option ? option.text : t(option.label),
    })),
  };
}

export function translateView(spec: ViewSpec, t: Translate): ListView {
  return { label: t(spec.label), sort: spec.sort, filters: spec.filters };
}

/**
 * Which view tab is lit, and the setter the table reports a press to.
 *
 * The highlight is derived from the query. A reader who edits a filter is then
 * no longer claimed to be on the preset they started from. But two views can
 * ask for the same sort and filters. Say a saved "German customers" view
 * narrows the list the same way the built-in Customers preset does. A purely
 * derived highlight lights the first match, so the reader's own view never
 * lights when they pick it.
 *
 * The pressed tab therefore wins, but only while it still describes the list.
 * The moment the query moves away it stops matching and the highlight falls
 * back to whatever the query now describes.
 *
 * What is pinned is the view's id, never its position. `/views` answers ordered
 * by name, so creating or renaming a view re-orders the rail under the reader.
 * A stored index then points at whichever view moved into that slot. The
 * highlight would then silently claim a view nobody picked.
 */
export function useActiveView(
  views: readonly RailView[],
  query: ListQuery,
): [number, (index: number) => void] {
  const [pickedId, setPickedId] = useState<string | null>(null);
  const matched = views.findIndex((spec) => matchesView(spec, query));
  const pinned = views.findIndex((spec) => spec.id === pickedId);
  const pinnedView = views[pinned];
  const view =
    pinnedView !== undefined && matchesView(pinnedView, query)
      ? pinned
      : matched;
  // The press reports an index into the rail just rendered, which is where the
  // id it names comes from. A press on nothing (the surface's own "clear
  // everything" reports index 0 against an empty rail) unpins rather than
  // pinning a view that is not there.
  return [view, (index: number) => setPickedId(views[index]?.id ?? null)];
}

/**
 * Is the list showing what this view asks for, nothing added or left out?
 *
 * Takes the state a view describes rather than its name. So it reads a screen's
 * built-in preset and a reader's saved view the same way. The two differ only
 * in where the label came from, and the highlight is about the query.
 *
 * Every dial that decides which rows the list holds counts. Two are easy to
 * leave out, and they are the reason this is spelled out. The archived toggle
 * widens the list as a filter narrows it. So a view saved with "Show archived"
 * on is a different view from the default one. The search narrows it the same
 * way: a view saved over "acme" describes the acme rows. A tab that ignored the
 * box would light over the whole list and name it acme.
 *
 * The invariant, then: a tab is lit only while the list on screen is the one it
 * names, down to the search box. Page size is the one allowed exception. It
 * changes how many of those rows are drawn at once, not which rows they are.
 */
function matchesView(
  spec: Readonly<{
    q?: string;
    sort?: string;
    filters?: Readonly<Record<string, string>>;
    includeArchived?: boolean;
  }>,
  query: ListQuery,
): boolean {
  if (query.q !== (spec.q ?? "")) {
    return false;
  }
  if (query.sort !== (spec.sort ?? "")) {
    return false;
  }
  if (query.includeArchived !== (spec.includeArchived ?? false)) {
    return false;
  }
  const wanted = Object.entries(spec.filters ?? {});
  const applied = Object.entries(query.filters).filter(([, value]) => value);
  return (
    wanted.length === applied.length &&
    wanted.every(([key, value]) => query.filters[key] === value)
  );
}

/**
 * What each chip currently shows, given the filters actually in the query.
 *
 * A normal chip stores its value under its own key and needs no translation.
 * Some chips have options that each name a different query parameter
 * (`param:value`, as in the owner dial). Such a chip stores the value under the
 * parameter. So its selected option has to be read back from whichever
 * parameter is set. Without this the dial narrows the list correctly and then
 * renders as "Any owner", which reads as a filter that did not take.
 */
export function chosenFor(
  // Takes the option values, so it reads a declared chip and a data-driven one
  // the same way. The two differ only in where their labels came from, and a
  // reader of this function never looks at a label.
  chips: readonly ListChip[],
  filters: Readonly<Record<string, string>>,
): Record<string, string> {
  const chosen = { ...filters };
  for (const chip of chips) {
    for (const option of chip.options) {
      const split = option.value.indexOf(":");
      if (split <= 0) {
        continue;
      }
      const param = option.value.slice(0, split);
      if (filters[param] === option.value.slice(split + 1)) {
        chosen[chip.key] = option.value;
      }
    }
  }
  return chosen;
}

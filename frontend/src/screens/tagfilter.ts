// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/**
 * The tag filter, on the wire and in the address, spelled ONCE.
 *
 * Three lists and a board carry this filter, and the address is the one place
 * where a multi-value filter cannot travel as itself: `ListQuery.filters` is a
 * flat `Record<string, string>` whose keys are wire param names spread straight
 * onto the request, so several tag ids have to survive a round trip through one
 * string. Encoding that per screen is how one list would start selecting a
 * different slice than another from the same address.
 *
 * Commas, not repetition: an address a person can read and edit, and the ids
 * are UUIDs so no value can contain the separator.
 */

/** How several tag ids combine. Mirrors the contract's `tag_mode` enum. */
export type TagMode = "any" | "all" | "none";

/**
 * parseTagMode narrows a mode off the address.
 *
 * An address is text a person can edit, so anything at all can arrive here.
 * An unknown mode is DROPPED rather than defaulted, and the caller then sends
 * no mode at all — which is what the endpoint reads as `any`. Mapping a typo
 * onto `any` here would be this tier quietly widening a filter the reader
 * wrote, and the server refuses an unknown mode for exactly that reason:
 * `any` is not a superset of `none`, so a garbage mode silently answered as
 * `any` returns a different slice, not a bigger one.
 */
export function parseTagMode(value: string | undefined): TagMode | undefined {
  switch (value) {
    case "any":
    case "all":
    case "none":
      return value;
    default:
      return undefined;
  }
}

/** The tag ids an address carries, in order, with blanks dropped. */
export function parseTagIDs(value: string | undefined): string[] {
  if (!value) {
    return [];
  }
  return value
    .split(",")
    .map((id) => id.trim())
    .filter((id) => id !== "");
}

/**
 * The filter entries for a request's query, from what the address holds.
 *
 * Empty when no tag is selected: `tag_mode` alone is not a filter — the
 * contract says a mode with nothing to combine is ignored — and sending it
 * would put a dial in the address that changes nothing.
 */
export function tagQueryParams(
  ids: readonly string[],
  mode: TagMode | undefined,
): { tag_id?: string[]; tag_mode?: TagMode } {
  if (ids.length === 0) {
    return {};
  }
  // No mode when the address named none, or named one this tier does not
  // recognise: the endpoint's own default is `any`, so omitting it says the
  // same thing without this tier deciding what a typo meant.
  return mode === undefined
    ? { tag_id: [...ids] }
    : { tag_id: [...ids], tag_mode: mode };
}

/**
 * The filter map as WIRE params, with the tag filter expanded.
 *
 * `ListQuery.filters` is spread straight onto a request, so every key in it is
 * a param name already. `tag_id` is the one key that cannot travel that way: it
 * holds several ids in one comma-joined string for the address, and the
 * endpoint wants an array. Every list that carries tags calls this instead of
 * spreading the map, so the three cannot disagree about what one address means.
 */
export function listQueryParams(
  filters: Readonly<Record<string, string>>,
): Record<string, unknown> {
  const { tag_id: joined, tag_mode: mode, ...rest } = filters;
  return {
    ...rest,
    ...tagQueryParams(parseTagIDs(joined), parseTagMode(mode)),
  };
}

/**
 * The filter map with a mode that has nothing left to combine removed.
 *
 * Clearing the tag dial drops `tag_id` and would leave `tag_mode` sitting in
 * the address, where nothing draws it and nothing sends it: an invisible dial
 * the reader cannot see to clear, which then narrows the list the moment they
 * pick a word again in a way they never asked for.
 */
export function withoutStrandedTagMode(
  filters: Readonly<Record<string, string>>,
): Record<string, string> {
  if (parseTagIDs(filters.tag_id).length > 0) {
    return { ...filters };
  }
  const { tag_mode: _strandedMode, ...rest } = filters;
  return { ...rest };
}

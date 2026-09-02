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

const MODES: readonly string[] = ["any", "all", "none"];

/**
 * parseTagMode narrows a mode off the address.
 *
 * An address is text a person can edit, so anything at all can arrive here.
 * Anything but the three is `any` — the contract's own default, and the
 * widest of the three, so a typo shows MORE rows rather than silently hiding
 * some behind a filter the reader never set.
 */
export function parseTagMode(value: string | undefined): TagMode {
  return value !== undefined && MODES.includes(value)
    ? (value as TagMode)
    : "any";
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
  mode: TagMode,
): { tag_id?: string[]; tag_mode?: TagMode } {
  if (ids.length === 0) {
    return {};
  }
  return { tag_id: [...ids], tag_mode: mode };
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

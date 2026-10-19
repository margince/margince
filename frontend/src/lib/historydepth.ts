// How far into this app's own run of history entries the reader is.
//
// The app's Back must know whether `history.back()` stays in the app. The
// browser does not say, so each entry carries its depth in `history.state`.
// Depth 0 is an entry the app did not push; an in-app move adds one.
// A stamp outlives a reload, and the entries below it are still ours.

const DEPTH_KEY = "marginceDepth";

// The address at each depth, for the entries seen in this page load. A gap is
// an entry from before the load, whose address is unknown.
const seenAt: string[] = [];
let depth: number | undefined;

type Stamp = Readonly<{ depth: number; at: string }>;

function stampOf(state: unknown): Stamp | undefined {
  if (typeof state !== "object" || state === null) {
    return undefined;
  }
  const carried: unknown = Reflect.get(state, DEPTH_KEY);
  if (typeof carried !== "object" || carried === null) {
    return undefined;
  }
  const stampDepth: unknown = Reflect.get(carried, "depth");
  const at: unknown = Reflect.get(carried, "at");
  return typeof stampDepth === "number" && typeof at === "string"
    ? { depth: stampDepth, at }
    : undefined;
}

// Merged into what the entry already carries: lib/scrollmemory.ts keeps the
// entry's identity in the same object.
function stamp(value: number): void {
  const carried = globalThis.history.state;
  const mark: Stamp = { depth: value, at: globalThis.location.hash };
  const state =
    typeof carried === "object" && carried !== null
      ? { ...carried, [DEPTH_KEY]: mark }
      : { [DEPTH_KEY]: mark };
  globalThis.history.replaceState(state, "");
}

/**
 * Note the entry the reader is on.
 *
 * `moved`: the app just moved by `hashchange`. `replaced`: the app rewrote the
 * address of the entry it is on. `start`: the app begins watching the address.
 * A stamp written at another address is trusted only after a replace.
 * Otherwise it was copied from the previous entry (happy-dom does this), so
 * the entry is new. A new entry drops every forward one, as the browser does.
 */
export function noteHistoryEntry(how: "moved" | "replaced" | "start"): void {
  const hash = globalThis.location.hash;
  const found = stampOf(globalThis.history.state);
  const known =
    how !== "replaced" && found?.at !== hash ? undefined : found?.depth;
  if (known === undefined) {
    depth = how === "moved" && depth !== undefined ? depth + 1 : 0;
    seenAt.length = depth;
  } else {
    depth = known;
  }
  if (found?.depth !== depth || found.at !== hash) {
    stamp(depth);
  }
  seenAt[depth] = hash;
}

/** Whether `history.back()` lands on an earlier screen of this app. */
export const backStaysInApp = (): boolean => (depth ?? 0) > 0;

/**
 * Steps back to the nearest earlier entry that is not `here`, or 0 when every
 * earlier in-app entry is. An entry of unknown address counts as elsewhere:
 * it is ours, and going there keeps the reader in the app.
 */
export function stepsBackOutOf(here: (hash: string) => boolean): number {
  if (depth === undefined) {
    return 0;
  }
  for (let at = depth - 1; at >= 0; at -= 1) {
    const hash = seenAt[at];
    if (hash === undefined || !here(hash)) {
      return depth - at;
    }
  }
  return 0;
}

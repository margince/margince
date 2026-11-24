// How far into this app's own run of history entries the reader is.
//
// The app's Back must know whether `history.back()` stays in the app. The
// browser does not say, so each entry carries its depth in `history.state`.
// Depth 0 is where the app's run of entries begins; each entry after it adds one.
// A stamp outlives a reload, and the entries below it are still ours.

const DEPTH_KEY = "marginceDepth";

// A traversal the browser never carries out fires nothing: a `go` past either
// end, or a leave refused at the unload prompt. One that loads a document is held.
const UNANSWERED_MS = 1000;

// The address at each depth, for the entries seen in this page load. A gap is
// an entry from before the load, whose address is unknown.
const seenAt: string[] = [];
let depth: number | undefined;
// The browser's own index of the entry at `depth`, where it reports one.
let indexAtDepth: number | undefined;
let traversing = false;
let unanswered: ReturnType<typeof setTimeout> | undefined;

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

// The Navigation API's list of this tab's entries, where the browser has one.
// It knows which way a move went, and addresses this page load never saw.
type BrowserEntries = Readonly<{
  currentEntry: NavigationHistoryEntry | null;
  entries: () => NavigationHistoryEntry[];
  addEventListener: EventTarget["addEventListener"];
}>;

function isBrowserEntries(value: unknown): value is BrowserEntries {
  return (
    typeof value === "object" &&
    value !== null &&
    "currentEntry" in value &&
    typeof Reflect.get(value, "entries") === "function" &&
    typeof Reflect.get(value, "addEventListener") === "function"
  );
}

function browserEntries(): BrowserEntries | undefined {
  const navigation: unknown = Reflect.get(globalThis, "navigation");
  return isBrowserEntries(navigation) ? navigation : undefined;
}

function browserIndex(): number | undefined {
  const index = browserEntries()?.currentEntry?.index;
  return index !== undefined && index >= 0 ? index : undefined;
}

// The entries this document made before the app began watching, which a move
// made while the shell mounted leaves. Back reaches them without leaving.
function sameDocumentEntriesBefore(index: number | undefined): number {
  const entries = browserEntries()?.entries();
  if (index === undefined || entries === undefined) {
    return 0;
  }
  let first = index;
  while (first > 0 && entries[first - 1]?.sameDocument === true) {
    first -= 1;
  }
  return index - first;
}

// An entry without a stamp of its own. A replace keeps the entry, so its depth.
// A move goes as far as the browser's index went, or else is one new entry.
function unstampedDepth(
  how: "moved" | "replaced" | "start",
  index: number | undefined,
): number {
  if (how === "start" || depth === undefined) {
    seenAt.length = 0;
    return sameDocumentEntriesBefore(index);
  }
  if (how === "replaced") {
    return depth;
  }
  const moved =
    index !== undefined && indexAtDepth !== undefined
      ? Math.max(0, depth + index - indexAtDepth)
      : depth + 1;
  // A new entry drops every forward one, as the browser does.
  if (moved > depth) {
    seenAt.length = depth + 1;
  }
  return moved;
}

/**
 * Note the entry the reader is on.
 *
 * `moved`: the browser moved to another entry (`popstate` or `hashchange`,
 * counted once per move by lib/router.tsx). `replaced`: the app rewrote the
 * address of the entry it is on. `start`: the app begins watching the address.
 * A stamp written at another address is trusted only after a replace: on a
 * move it was copied from the previous entry, as happy-dom does.
 */
export function noteHistoryEntry(how: "moved" | "replaced" | "start"): void {
  const hash = globalThis.location.hash;
  const found = stampOf(globalThis.history.state);
  const index = browserIndex();
  depth =
    found && (how === "replaced" || found.at === hash)
      ? found.depth
      : unstampedDepth(how, index);
  indexAtDepth = index;
  if (how === "moved") {
    settleTraversal();
  }
  if (found?.depth !== depth || found.at !== hash) {
    stamp(depth);
  }
  seenAt[depth] = hash;
}

/** The depth stamped on the entry the browser is on, if it carries one. */
export const entryDepth = (): number | undefined =>
  stampOf(globalThis.history.state)?.depth;

/** The address of this app's entry at `atDepth`, when it is known. */
export function hashAt(atDepth: number): string | undefined {
  const seen = seenAt[atDepth];
  if (seen !== undefined || depth === undefined || atDepth < 0) {
    return seen;
  }
  if (indexAtDepth === undefined) {
    return undefined;
  }
  const url =
    browserEntries()?.entries()[indexAtDepth - (depth - atDepth)]?.url;
  return url ? new URL(url).hash : undefined;
}

/**
 * The depth of the nearest earlier entry that is not `here`, if there is one.
 * An entry of unknown address counts as elsewhere: it is ours, and going there
 * keeps the reader in the app.
 */
export function depthBackOutOf(
  here: (hash: string) => boolean,
): number | undefined {
  if (depth === undefined) {
    return undefined;
  }
  for (let at = depth - 1; at >= 0; at -= 1) {
    const hash = hashAt(at);
    if (hash === undefined || !here(hash)) {
      return at;
    }
  }
  return undefined;
}

/** Whether the entry just ahead of this one is `here`, as after a step back out of it. */
export function steppedBackOutOf(here: (hash: string) => boolean): boolean {
  const ahead = depth === undefined ? undefined : hashAt(depth + 1);
  return ahead !== undefined && here(ahead);
}

/**
 * Move to the entry at `targetDepth`, one traversal at a time.
 *
 * `history.go` lands a task later or more. A second press before then would
 * start a second traversal from the same depth, past the entry it means.
 * False when no traversal started, or the browser has no entry there.
 */
export function traverseTo(targetDepth: number): boolean {
  if (
    depth === undefined ||
    targetDepth < 0 ||
    targetDepth === depth ||
    traversing ||
    !browserHasEntry(targetDepth - depth)
  ) {
    return false;
  }
  traversing = true;
  unanswered = globalThis.setTimeout(settleTraversal, UNANSWERED_MS);
  browserEntries()?.addEventListener("navigate", holdForDocumentLoad);
  globalThis.addEventListener("pageshow", settleTraversal);
  globalThis.history.go(targetDepth - depth);
  return true;
}

/** Whether a traversal `traverseTo` started has yet to land or be given up. */
export const traversalInFlight = (): boolean => traversing;

/**
 * Step back to the nearest earlier entry of ours that is not `here`, or call
 * `elsewhere` when there is none. A press already off `here` is a repeat.
 */
export function stepBackOutOf(
  here: (hash: string) => boolean,
  elsewhere: () => void,
): void {
  if (!here(globalThis.location.hash)) {
    return;
  }
  const before = depthBackOutOf(here);
  if (before === undefined) {
    elsewhere();
  } else {
    traverseTo(before);
  }
}

// Without the Navigation API, a `go` past either end is sent and does nothing.
function browserHasEntry(delta: number): boolean {
  const index = browserIndex();
  const count = browserEntries()?.entries().length;
  return (
    index === undefined ||
    count === undefined ||
    (index + delta >= 0 && index + delta < count)
  );
}

// Fires once the unload prompt has let a document load go. Held until the page
// leaves or comes back from the cache (`pageshow`); a press meanwhile overshoots.
function holdForDocumentLoad(event: Event): void {
  const destination: unknown = Reflect.get(event, "destination");
  if (
    traversing &&
    typeof destination === "object" &&
    destination !== null &&
    Reflect.get(destination, "sameDocument") === false
  ) {
    globalThis.clearTimeout(unanswered);
  }
}

function settleTraversal(): void {
  globalThis.clearTimeout(unanswered);
  traversing = false;
}

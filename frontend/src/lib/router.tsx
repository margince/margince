import { useEffect, useState, useSyncExternalStore } from "react";
import { noteHistoryEntry } from "./historydepth";
import { publishRecordAddress, restoreRecordAddress } from "./recordaddress";
import { hashWithParams, type UrlParams } from "./urlstate";
// The screen union binds navigation and dispatch to the same route names.
export const SCREENS = [
  "home",
  "contacts",
  "companies",
  "partners",
  "leads",
  "deals",
  "projects",
  "worklist",
  "analytics",
  "settings",
  "filters",
  "scheduled",
  "offers",
  "legal-reviews",
  "search",
  "share",
  // A tag has a page of its own: the records carrying one word, grouped by
  // type. Every tag pill in the product links here.
  "tags",
  // One Live List or Shortlist, opened from the lists library.
  "lists",
  "onboarding",
  // reach:external the extension's own chrome, opened by the client rather than
  // from anywhere in this app.
  "client",
  "book",
  "preferences",
  // reach:external the address the visible unsubscribe footer of an outbound
  // message carries; this app never links to it.
  "unsubscribe",
  // reach:external a double opt-in link, followed out of the reader's mailbox.
  "confirm",
  "room",
  // reach:external where a provider returns a human who has just lent an agent
  // their authority; it is a redirect target, never a destination.
  "oauth-consent",
  // reach:external the address a password-reset mail carries. A signed-out
  // reader has no nav to reach it from.
  "reset-password",
  "ext",
  "x",
  // reach:external what `parseRoute` answers for an address this app does not
  // have; a link to it would be a link to the fact that a link was wrong.
  "not-found",
] as const;

/**
 * The screen half of an address.
 *
 * Call sites name this alias and never the list above it, so the set of
 * addresses widens in one place.
 */
export type Screen = (typeof SCREENS)[number];

export type Route = {
  screen: Screen;
  id?: string;
  id2?: string;
  id3?: string;
};

const SCREEN_NAMES: ReadonlySet<string> = new Set(SCREENS);

// A type predicate, not a cast. The runtime test and the compile-time claim are
// the same expression, so they cannot drift. lib/extensions.ts uses this shape
// for unit RBAC objects.
export function isScreen(value: string): value is Screen {
  return SCREEN_NAMES.has(value);
}

export function parseHash(hash: string): Route {
  // A hash may carry a query of its own ("#/onboarding?utm=x"); the query is
  // not part of the route and must never leak into a screen name.
  const [path = ""] = hash.replace(/^#\/?/, "").split("?");
  const [screen, id, id2, id3] = path.split("/").filter(Boolean);
  if (screen === undefined) {
    return { screen: "home" };
  }
  // The day's surface moved from `today` to `worklist`, and a rep's bookmark
  // outlives the rename. Answering the old address with the page that replaced
  // it costs one line; answering it with Not Found teaches them the product
  // lost their page.
  if (screen === "today") {
    return { screen: "worklist" };
  }
  // Reports became Analytics, and the same argument applies: a bookmark and
  // every link already sent outlive the rename. The segment rides along, so
  // #/reports/forecast lands on the forecast section rather than the default.
  if (screen === "reports") {
    return { screen: "analytics", id, id2, id3 };
  }
  // Home is language-neutral; previously shared Brief links keep their state.
  if (screen === "brief") {
    return { screen: "home", id, id2, id3 };
  }
  // The account's contact tab is addressed `contacts`, and a link already sent
  // to a colleague names it `contacts`. The account and anything below the tab
  // ride along, so an old address opens the same roster the new one does rather
  // than dropping the reader on Overview.
  if (screen === "companies" && id2 === "contacts") {
    return { screen, id, id2: "contacts", id3 };
  }
  if (!isScreen(screen)) {
    // A hash comes out of the URL bar, so its first segment is text a human
    // typed, not a Screen. An address this app does not answer is a page — the
    // not-found one — rather than a parse failure, and it carries no segments
    // below it: they addressed arguments of a screen that isn't there.
    return { screen: "not-found" };
  }
  return { screen, id, id2, id3 };
}

export function routeHash(route: Route): string {
  // The segments are positional and have no placeholder, so a gap ends the
  // path: an id3 with no id2 cannot be serialized without inventing one.
  const path: string[] = [];
  for (const segment of [route.screen, route.id, route.id2, route.id3]) {
    if (!segment) {
      break;
    }
    path.push(segment);
  }
  return `#/${path.join("/")}`;
}

/**
 * Go to an address.
 *
 * `dials` is the QUERY half — which view of the screen, not which screen. It is
 * optional because most navigations name a page and nothing about how it is
 * narrowed; it exists because some do, and there was no spelling for it:
 * `useUrlParams` writes the query of the screen the reader is ALREADY on, so a
 * figure on Brief offering to open one lane of the Worklist had nowhere to say
 * which lane. Serialised by `hashWithParams`, so an address written here and one
 * written by a dial on the screen itself are the same address.
 */
export function navigate(route: Route, dials?: UrlParams): void {
  const path = routeHash(route);
  globalThis.location.hash = dials ? hashWithParams(path, dials) : path;
}

/**
 * Go to an address WITHOUT leaving the current one in history.
 *
 * For a redirect, and only for a redirect: an address the product answers by
 * sending the reader somewhere else must not be a history entry, or Back lands
 * on it, it redirects again, and the reader cannot get out of the loop with the
 * one key that exists for getting out of things.
 *
 * `history.replaceState` and not `location.replace`, which is the obvious
 * spelling and the wrong one: `location.replace` discards the entry's state
 * object. lib/scrollmemory.ts stamps the entry's identity in there. A redirect,
 * or a screen that normalises its own address on arrival, would wipe that name,
 * and the offset saved under it would be lost. Carrying the current
 * state through keeps the entry the same entry.
 *
 * It fires no event, so the store has to be told; lib/urlstate.ts's
 * `replaceParams` is the same write for the QUERY half and says the same thing.
 */
export function navigateReplacing(route: Route, dials?: UrlParams): void {
  globalThis.history.replaceState(
    globalThis.history.state,
    "",
    dials ? hashWithParams(routeHash(route), dials) : routeHash(route),
  );
  announce();
}

// The one-time credentials an address may carry, and why they are taken HERE.
//
// Two emailed links land as a hash query: the Deal Room's buyer invitation and
// the password reset. Each query is a bearer capability — one is worth a room,
// the other a password — so it has to leave the address bar and the history
// entry immediately, and neither screen can be what takes it out. App renders a
// gate ahead of every route (a release skew, the session probe, the onboarding
// redirect that rewrites the hash), and while such a gate holds, the screen that
// would have scrubbed the credential never mounts. It then sits in the reader's
// history for as long as the gate does, which on an installation mid-upgrade is
// until somebody finishes the upgrade.
//
// Reading the address is the one thing every one of those gates is downstream
// of, so taking it here makes "before the gate" a fact about the code rather
// than an ordering somebody has to keep true in App.
//
// ONE table and one memory, because these were two copies of the scrub and the
// second copy is how the first one's fix missed a password-reset token.
//
// What makes an address a carrier: its query IS a credential, so the screen can
// be handed it in memory and still do its whole job. An address whose query is
// the REQUEST — the authorize parameters a consent screen posts back — is not
// one, and taking it would leave that screen with nothing to render.
const HASH_CREDENTIALS: ReadonlyArray<{ screen: Screen; param: string }> = [
  // The Deal Room's buyer invitation: `#/room?c=<credential>`.
  { screen: "room", param: "c" },
  // The emailed password reset: `#/reset-password?token=<token>`. The screen
  // name is RESET_ROUTE in features/sign-in/auth.tsx, spelled as a literal here
  // as SCREENS above spells it. Importing a screen from the router would be a
  // cycle, and the `Screen` type still fails a rename.
  { screen: "reset-password", param: "token" },
];

const held = new Map<Screen, string>();

// Moves whatever credential the CURRENT address carries into memory and
// rewrites the address without it. Idempotent, because the address no longer
// carries one afterwards.
function takeFromHash(): void {
  const hash = globalThis.location.hash;
  const query = hash.indexOf("?");
  if (query < 0) {
    return;
  }
  const { screen } = parseHash(hash);
  const carrier = HASH_CREDENTIALS.find((entry) => entry.screen === screen);
  if (!carrier) {
    return;
  }
  const carried = new URLSearchParams(hash.slice(query + 1)).get(carrier.param);
  if (carried === null) {
    return;
  }
  held.set(screen, carried);
  // replaceState, not an assignment to location.hash: an assignment adds a
  // history entry and leaves the credential in the one behind it, so the leak
  // would survive a Back press. It fires no hashchange either, which is what
  // lets the subscription below take the credential before it tells React the
  // address moved — the snapshot React then reads is already the scrubbed one.
  //
  // The document's own query is kept: the credential is in the fragment, so
  // nothing here needs `?…` gone, and rewriting it away discarded whatever else
  // the URL was carrying (Storybook's `?id=<story>`, which left a reload of the
  // canvas on no story at all).
  const { pathname, search } = globalThis.location;
  globalThis.history.replaceState(
    null,
    "",
    `${pathname}${search}${hash.slice(0, query)}`,
  );
}

/**
 * The credential `screen`'s address carried, out of the address.
 *
 * Reading is what takes it: the first call that finds one moves it into memory
 * and rewrites the address without it, and every call after that answers from
 * memory. Non-destructive on the way out, so a replayed render (StrictMode) or
 * one React discards cannot drop the credential on the floor in the gap between
 * the router taking it and the screen that spends it asking for it.
 */
export function takeHashCredential(screen: Screen): string | null {
  takeFromHash();
  return held.get(screen) ?? null;
}

/**
 * Forget a credential the screen has taken in hand.
 *
 * Memory is where the address used to hold it, and it has to empty the way the
 * address did — the moment the screen has it — because memory outlives a mount
 * and the address did not. A remount that found the credential still there
 * would spend a single-use link twice, or put the reset form back over a reader
 * who had gone back to login. Only the screen knows when it has it.
 */
export function forgetHashCredential(screen: Screen, credential: string): void {
  if (held.get(screen) === credential) {
    held.delete(screen);
  }
}

// Notify subscribers synchronously so an app navigation cannot fetch the previous route.
const watchers = new Set<() => void>();

// The whole address (path, query and fragment) as of the last announcement.
// The browser tells this module about a Back or Forward with two events, and
// it fires both for a fragment-only move: `popstate` and then `hashchange`.
// The second one finds the address it already announced and does nothing, so
// one move is one announcement and one history entry counted.
let lastAddress: string | undefined;

function liveAddress(): string {
  const { pathname, search, hash } = globalThis.location;
  return `${pathname}${search}${hash}`;
}

function announce(): void {
  publishRecordAddress();
  // A replace keeps the entry, and the address noted for it must follow. With
  // nobody watching, no entry is being counted.
  if (watchers.size > 0) {
    noteHistoryEntry("replaced");
  }
  // Read after publishRecordAddress has rewritten the path, so the event that
  // trails this move finds the rewritten address and counts as the same move.
  lastAddress = liveAddress();
  // A copy, so a watcher that unsubscribes while being told does not change the
  // set underneath the loop.
  for (const watcher of [...watchers]) {
    watcher();
  }
}

function addressChanged(): void {
  if (liveAddress() === lastAddress) {
    return;
  }
  // A second link pasted into an open tab is a hash change and nothing else:
  // the screen does not remount for one, and whatever renders next is decided
  // above the route. So the credential comes out of the new address before
  // React is told there is one.
  takeFromHash();
  noteHistoryEntry("moved");
  announce();
}

// Both events, because neither covers every move. `hashchange` fires only when
// two entries differ in the fragment alone. A record's entry sits under
// `/r/<type>/<id>` (lib/recordaddress.ts), so Back from a record to the list at
// `/` changes the path, and the browser fires `popstate` and no `hashchange`.
// `popstate` is the one that fires on every Back and Forward, but not on an
// assignment to `location.hash` everywhere.
function subscribe(onChange: () => void): () => void {
  if (watchers.size === 0) {
    publishRecordAddress();
    // The entry the app starts on, so the first move is counted from it.
    noteHistoryEntry("start");
    lastAddress = liveAddress();
    globalThis.addEventListener("hashchange", addressChanged);
    globalThis.addEventListener("popstate", addressChanged);
  }
  watchers.add(onChange);
  return () => {
    watchers.delete(onChange);
    if (watchers.size === 0) {
      globalThis.removeEventListener("hashchange", addressChanged);
      globalThis.removeEventListener("popstate", addressChanged);
    }
  };
}

/**
 * Run `onChange` each time the address moves. The router has already taken
 * any credential out of it and counted the history entry.
 *
 * A raw `hashchange` listener misses a Back or Forward that changes the path.
 * This one hears every move the router hears.
 */
export function watchAddress(onChange: () => void): () => void {
  return subscribe(onChange);
}

/**
 * Tell the address store the hash moved, for a move this app made itself.
 *
 * Only lib/urlstate.ts needs it: it writes the address directly, so nothing has
 * dispatched anything yet. An event that lands for the same address afterwards
 * finds it already announced and does nothing.
 */
export function announceAddressChanged(): void {
  announce();
}

/**
 * The whole address, as a value that changes when the reader navigates.
 *
 * Exported because an address has two halves with two owners. The path names
 * what is on screen and is this module's. The query names which view of it (how
 * a list is narrowed, ordered and paged) and belongs to lib/urlstate.ts.
 * Both must re-render on the same announcement, and a second subscription to
 * the browser's events would be a second answer to where the reader is.
 */
export function useHash(): string {
  // Taken during the FIRST render of whoever reads the address, ahead of the
  // snapshot below, so the address React routes on is the scrubbed one and
  // nothing can render with a credential still in the bar. An effect would be
  // late by exactly the gate this exists for: it runs after that gate has
  // rendered, and the gate is what stops the screen mounting to scrub it at all.
  //
  // It lives here rather than in useRoute because the query half has its own
  // reader (lib/urlstate.ts). A scrub done by only one of the two would depend
  // on which of them a screen called first.
  useState(() => {
    restoreRecordAddress();
    takeFromHash();
  });
  return useSyncExternalStore(
    subscribe,
    () => globalThis.location.hash,
    () => "",
  );
}

export function useRoute(): Route {
  const hash = useHash();
  useEffect(() => {
    if (/^#\/?brief(?:[/?]|$)/.test(hash)) {
      navigateReplacing(
        parseHash(hash),
        new Map(new URLSearchParams(hash.split("?")[1])),
      );
    }
  }, [hash]);
  return parseHash(hash);
}

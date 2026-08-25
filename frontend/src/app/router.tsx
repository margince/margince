import { useState, useSyncExternalStore } from "react";

// Hash routing: "#/deals/01J9ZK" → { screen: "deals", id: "01J9ZK" }.
// Client routes live behind '#', so any static host serves index.html for
// every entry point — no server-side SPA fallback needed.

// Every address this product answers, spelled ONCE. The other places that
// enumerate destinations — the nav table and RAIL_LESS_SCREENS in app/nav.ts,
// the dispatch in App.tsx — are typed against `Screen` rather than repeating the
// names, so a destination cannot exist in one of them and be missing from the
// rest, and `navigate({ screen: "dealz" })` fails to compile instead of
// rendering a surface that reads as unbuilt.
//
// Three members are also spelled as a named constant elsewhere, because the
// module that owns the route owns the name: `ext` is app/extensions.ts's
// EXTENSION_SCREEN, `reset-password` is screens/auth.tsx's RESET_ROUTE, and
// `scheduled` is screens/scheduledsends.tsx's SCHEDULED_SCREEN. All three are
// literal types used where a `Screen` is expected, so a rename there stops
// compiling here rather than drifting.
const SCREENS = [
  "home",
  "contacts",
  "companies",
  "partners",
  "leads",
  "deals",
  "projects",
  "today",
  "tasks",
  "inbox",
  "reports",
  "ai",
  "settings",
  "dedupe",
  "filters",
  "scheduled",
  "offers",
  "search",
  "share",
  "onboarding",
  "client",
  "book",
  "preferences",
  "room",
  "oauth-consent",
  "reset-password",
  "ext",
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

// A type predicate, not a cast: the runtime test and the compile-time claim are
// the same expression, so there is no way for them to drift (the shape
// app/extensions.ts uses for unit RBAC objects).
export function isScreen(value: string): value is Screen {
  return SCREEN_NAMES.has(value);
}

export function parseHash(hash: string): Route {
  // A hash may carry a query of its own ("#/onboarding?utm=x"); the query is
  // not part of the route and must never leak into a screen name.
  const parts = hash
    .replace(/^#\/?/, "")
    .split("?")[0]
    .split("/")
    .filter(Boolean);
  if (parts.length === 0) {
    return { screen: "home" };
  }
  const [screen, id, id2, id3] = parts;
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
 * How many leading segments of an address identify WHAT is on screen.
 *
 * Anything beyond that depth chooses a VIEW of the same thing — a tab — and
 * moving between views must not throw the page away: the screen keeps its state,
 * the chrome above the tab strip keeps its DOM nodes, and the arrival animation
 * (design-system/enter.css) plays for the panel that actually changed instead of
 * for the header, the readings and the tab bar the reader just clicked.
 *
 * `WHOLE_ADDRESS` — every segment names the thing — is what almost every screen
 * wants, and the exceptions are only the screens that put a TAB in the URL. So
 * this table looks like it does nothing, and that is the point: it is the one
 * place a new tabbed screen declares itself, and a screen added to the union
 * cannot inherit an answer nobody chose, because `Record` makes the set
 * complete. Lower a screen's depth ONLY for a segment that selects a view of
 * what is already on screen — a flow step, an outcome, or a second record id is
 * a different thing, and remounting is right for those.
 */
const WHOLE_ADDRESS = 4;

const IDENTITY_DEPTH: Readonly<Record<Screen, number>> = {
  home: WHOLE_ADDRESS,
  // #/contacts/<person>/<tab> — the six person tabs are a view of one person,
  // and they are the reason this table exists.
  contacts: 2,
  companies: WHOLE_ADDRESS,
  partners: WHOLE_ADDRESS,
  leads: WHOLE_ADDRESS,
  deals: WHOLE_ADDRESS,
  projects: WHOLE_ADDRESS,
  today: WHOLE_ADDRESS,
  tasks: WHOLE_ADDRESS,
  inbox: WHOLE_ADDRESS,
  reports: WHOLE_ADDRESS,
  ai: WHOLE_ADDRESS,
  // Not a tab, however much the sidebar looks like one: every settings entry is
  // its own page, and the admin half is a segment deeper.
  settings: WHOLE_ADDRESS,
  dedupe: WHOLE_ADDRESS,
  filters: WHOLE_ADDRESS,
  scheduled: WHOLE_ADDRESS,
  offers: WHOLE_ADDRESS,
  search: WHOLE_ADDRESS,
  share: WHOLE_ADDRESS,
  onboarding: WHOLE_ADDRESS,
  client: WHOLE_ADDRESS,
  book: WHOLE_ADDRESS,
  preferences: WHOLE_ADDRESS,
  room: WHOLE_ADDRESS,
  "oauth-consent": WHOLE_ADDRESS,
  "reset-password": WHOLE_ADDRESS,
  ext: WHOLE_ADDRESS,
  "not-found": WHOLE_ADDRESS,
};

/**
 * The address with its view-only segments dropped: what is on screen, not which
 * view of it.
 *
 * Built out of `routeHash` rather than joined by hand, so there is still one
 * spelling of an address in this module and a truncated one cannot drift from a
 * whole one.
 */
export function routeIdentity(route: Route): string {
  const depth = IDENTITY_DEPTH[route.screen];
  return routeHash({
    screen: route.screen,
    id: depth > 1 ? route.id : undefined,
    id2: depth > 2 ? route.id2 : undefined,
    id3: depth > 3 ? route.id3 : undefined,
  });
}

export function navigate(route: Route): void {
  globalThis.location.hash = routeHash(route);
}

/**
 * Go to an address WITHOUT leaving the current one in history.
 *
 * For a redirect, and only for a redirect: an address the product answers by
 * sending the reader somewhere else must not be a history entry, or Back lands
 * on it, it redirects again, and the reader cannot get out of the loop with the
 * one key that exists for getting out of things.
 *
 * `location.replace` rather than `history.replaceState`, because the hash IS
 * this router's state: `replaceState` leaves no `hashchange` behind, so the
 * store would keep serving the old address while the URL bar showed the new
 * one.
 */
export function navigateReplacing(route: Route): void {
  globalThis.location.replace(routeHash(route));
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
  // name is screens/auth.tsx's RESET_ROUTE, spelled as the literal here for the
  // same reason SCREENS above spells it — importing a screen from the router
  // would be a cycle, and the `Screen` type still fails a rename.
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

function subscribe(onChange: () => void): () => void {
  const addressChanged = () => {
    // A second link pasted into an open tab is a hash change and nothing else:
    // the screen does not remount for one, and whatever renders next is decided
    // above the route. So the credential comes out of the new address before
    // React is told there is one.
    takeFromHash();
    onChange();
  };
  globalThis.addEventListener("hashchange", addressChanged);
  return () => globalThis.removeEventListener("hashchange", addressChanged);
}

export function useRoute(): Route {
  // Taken during this component's FIRST render, ahead of the snapshot below, so
  // the address React routes on is the scrubbed one and nothing can render with
  // a credential still in the bar. An effect would be late by exactly the gate
  // this exists for: it runs after that gate has rendered, and the gate is what
  // stops the screen mounting to scrub it at all.
  useState(takeFromHash);
  const hash = useSyncExternalStore(
    subscribe,
    () => globalThis.location.hash,
    () => "",
  );
  return parseHash(hash);
}

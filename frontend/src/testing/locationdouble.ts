// A stand-in for `location` that a suite can watch a navigation through.
//
// NAMED MEMBERS, not a spread, and the difference is the whole reason this file
// exists. A `Location`'s members are accessors on its PROTOTYPE, so
// `{ ...globalThis.location }` copies whichever of them the environment happens
// to have put on the instance — under jsdom that was enough to keep `hash`
// working, under happy-dom it copies nothing, and the router then read
// `location.hash` as `undefined` and the suite failed inside production code
// with a TypeError. A double that depends on how an environment implements the
// thing it doubles is a double that tests the environment.
//
// Three suites replace `location` to observe a navigation — the sign-in screen,
// the connections card and the OAuth consent screen — and each spelled the copy
// itself. One spelling, so the next one cannot be written the way that broke.

/** The members of `Location` a screen reads, carried by value. */
export type LocationDouble = Pick<
  Location,
  | "hash"
  | "host"
  | "hostname"
  | "href"
  | "origin"
  | "pathname"
  | "port"
  | "protocol"
  | "search"
> & {
  assign: (url: string) => void;
  replace?: (url: string) => void;
  reload?: () => void;
};

/**
 * A double of the CURRENT location, with the given members replaced.
 *
 * `assign` is required rather than optional: the reason a suite reaches for
 * this is to watch a navigation, and a double with no way to record one is a
 * double that silently lets the browser's real `assign` be called — which, in a
 * test environment, is either a no-op or a cross-origin error, and neither says
 * what the suite wanted to know.
 */
export function locationDouble(
  over: Partial<LocationDouble> & Pick<LocationDouble, "assign">,
): LocationDouble {
  const current = globalThis.location;
  return {
    hash: current.hash,
    host: current.host,
    hostname: current.hostname,
    href: current.href,
    origin: current.origin,
    pathname: current.pathname,
    port: current.port,
    protocol: current.protocol,
    search: current.search,
    ...over,
  };
}

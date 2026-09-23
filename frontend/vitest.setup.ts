import { afterEach, beforeEach, vi } from "vitest";
import { restoreClipboardStubs } from "./src/design-system/clipboard-testing";
import { takeUnroutedSessionProbes } from "./src/screens/unrouted-session";

// Node ≥23 ships its own global Web Storage: a `localStorage` getter that
// yields undefined unless the process was started with --localstorage-file.
// Because the key already exists on the Node global, vitest's populateGlobal
// keeps it instead of copying the environment's Storage onto the test global
// (only keys on vitest's own KEYS allowlist override an existing global, and
// the storage keys are not on it).
//
// UNDER HAPPY-DOM THE WINDOW *IS* THE GLOBAL, so there is no second object to
// rebind from — which is how the suite went red on Node 26 while staying green
// on the Node 22 runner, where Node's getter does not exist and happy-dom's own
// storage stands. 970 failures across 58 files, all of them "Cannot read
// properties of undefined (reading 'setItem')".
//
// So this INSTALLS one rather than borrowing it: a Storage the tests can use,
// per test file, cleared between files because each gets its own module
// instance. It is deliberately the whole surface the DOM one has, because a
// partial double fails as a puzzle rather than as a missing feature.
class TestStorage implements Storage {
  private entries = new Map<string, string>();

  get length(): number {
    return this.entries.size;
  }

  clear(): void {
    this.entries.clear();
  }

  // EVERY ARGUMENT IS COERCED, because the real Storage does. A test calling
  // setItem(1, x) and then setItem("1", y) has written one entry to a browser
  // and would have written two here, which is a double that fails as a puzzle
  // rather than as a missing feature.
  getItem(key: string): string | null {
    return this.entries.get(String(key)) ?? null;
  }

  key(index: number): string | null {
    return [...this.entries.keys()][Math.trunc(Number(index)) || 0] ?? null;
  }

  removeItem(key: string): void {
    this.entries.delete(String(key));
  }

  setItem(key: string, value: string): void {
    this.entries.set(String(key), String(value));
  }
}

for (const key of ["localStorage", "sessionStorage"] as const) {
  const testGlobal: Record<string, unknown> = globalThis;
  // WORKING STORAGE IS LEFT ALONE. A DOM that provided its own — jsdom does,
  // and so does happy-dom on a runtime without Node's getter — keeps it, so
  // this repair is invisible everywhere it is not needed.
  const existing = testGlobal[key] as Storage | undefined;
  if (existing && typeof existing.setItem === "function") {
    continue;
  }
  const storage = new TestStorage();
  Object.defineProperty(globalThis, key, {
    get: () => storage,
    configurable: true,
  });
}

// The two DOM stubs below are guarded on there BEING a DOM: this setup file runs
// for every suite, and most of them are node-environment (jsdom is opted into
// per file with `@vitest-environment jsdom`). Unguarded, they threw
// "window is not defined" at setup time and took 20 unrelated suites with them.
if (typeof window !== "undefined") {
  // UNMOUNT what a case rendered. Testing Library arms this for itself only
  // when the runner has put `afterEach` on the global, and `globals` is off in
  // vite.config.ts — so RTL's own guard (`typeof afterEach === "function"`)
  // sees nothing and never fires. Nothing reports that: a render simply
  // outlives the case that made it, its React tree still mounted and its query
  // observers still subscribed, so the next case queries a document holding
  // every tree before it, and `notifyManager` goes on flushing renders into
  // trees no test owns — including after the file's own teardown. Calling
  // `cleanup()` per file is a habit rather than a guarantee, and the files that
  // forgot left whole screens subscribed. Registered ONCE, here, so a file
  // cannot forget; scripts/autocleanup.test.tsx fails if it stops arming.
  //
  // Imported HERE rather than at the top of the file: this setup runs for every
  // suite and most are node-environment, which would otherwise pay for
  // react-dom to register a hook with no DOM to unmount from.
  const { cleanup } = await import("@testing-library/react");
  afterEach(cleanup);

  // jsdom ships no matchMedia, and every motion-aware component asks it for
  // prefers-reduced-motion on first render. Default to "no preference" so the
  // animated path is what the tests exercise; a test that wants the reduced path
  // overrides this per case.
  //
  // happy-dom HAS one, so this installs for the jsdom file alone. Its answers
  // are the same either way — "no preference", and not narrow — which is what
  // makes the two environments agree about which arrangement a screen is in.
  // The viewport that decides the second of those is stated in vite.config.ts
  // rather than inherited from whichever environment is installed.
  if (!window.matchMedia) {
    window.matchMedia = ((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addEventListener: () => {},
      removeEventListener: () => {},
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    })) as typeof window.matchMedia;
  }

  // jsdom ships no ResizeObserver, and the list table watches its own body so
  // the frozen column's edge shadow follows a resized column. A stub that never
  // fires is the honest stand-in: the component measures once on mount either
  // way, which is what the tests assert on. happy-dom has one, so this too
  // installs for the jsdom file alone.
  if (!window.ResizeObserver) {
    window.ResizeObserver = class {
      observe() {}
      unobserve() {}
      disconnect() {}
    } as unknown as typeof window.ResizeObserver;
  }

  // The Margince Core draws its liquid on a WebGL canvas; jsdom has no GL
  // context. Returning null is the same signal a browser without WebGL gives,
  // and the Core has a REQUIRED CSS rendering of every state for exactly that
  // case (WDS-CORE-3) — so this stub is what makes the suite exercise the
  // fallback rung of the ladder rather than the shader.
  //
  // Assigned UNCONDITIONALLY, and that is the fix rather than the style choice:
  // jsdom DOES define getContext, as a method that throws "Not implemented". An
  // `if (!…)` guard therefore never fires, and every render of a screen carrying
  // the Core prints a twelve-line jsdom stack to stderr — noise that trains a
  // reader to ignore test output, which is where the next real error hides.
  //
  // Re-applied before EVERY case, not once at setup: a suite whose `afterEach`
  // calls `vi.restoreAllMocks()` (auth.test.tsx does) hands getContext back to
  // jsdom after its first case, and every later render brings the stack trace
  // back. The install at setup time covers a render that happens while a test
  // file is still being imported, before any hook has run.
  const stubCanvasContext = () => {
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(null);
  };

  stubCanvasContext();
  beforeEach(stubCanvasContext);

  // Every case opens at the same address, the way a new tab does.
  //
  // The address is not a detail of the router any more: a list reads its
  // search, sort, filters and page size out of it (app/urlstate.ts), so the
  // hash one case leaves behind is state the NEXT case starts narrowed by.
  // That reads as an unrelated failure — a table with no rows, a chip already
  // chosen — and it is order-dependent, so the file passes when the case is run
  // alone. Registered here rather than per file because the leak belongs to
  // every suite that renders a list, including the ones nobody has written yet.
  //
  // Cleared to NOTHING rather than to `#/`, which is the same address but not
  // the same string: a suite proving that some path did not navigate asserts on
  // the hash it started with, and a baseline of `#/` would read as a move.
  //
  // This hook runs BEFORE a test file's own, so a suite that opens at a
  // specific address still gets it.
  beforeEach(() => {
    window.location.hash = "";
  });
}

// A case that left `GET /me` unrouted FAILS, rather than warning into the log.
//
// The stub cannot guess a session, so it refuses one — and a refused session
// reads as a malformed one: every capability hook fails closed and the surface
// draws its denied branch. Which branch a case then asserts against depends on
// whether that query settled first, so the case passes alone and fails under
// load, on a different name each run (#3483). Failing here makes the branch a
// case runs against its own choice again.
//
// Registered for every environment, not just jsdom: the stub is reachable from
// any suite that imports it, and a guard that skips where it thinks the defect
// cannot be is how a census stops seeing its subject.
//
// Read AFTER the case rather than watched during it, because the probe can fire
// from a render the case kicked off and never awaited — which is exactly the
// case that would otherwise pass.
afterEach(() => {
  const unrouted = takeUnroutedSessionProbes();
  if (unrouted > 0) {
    throw new Error(
      `this case left GET /me unrouted (${unrouted} probe(s)): the fetch stub refused a session ` +
        "it cannot guess, so every capability hook failed closed and the surface drew its denied " +
        "branch. Route it — '\"GET /me\": meRoute({ … })' with the grants the case is about — or, " +
        "if the denied branch is the point, take the count with takeUnroutedSessionProbes() to say so.",
    );
  }
});

// A stubbed clipboard belongs to the case that installed it.
//
// `stubClipboard` replaces `navigator.clipboard` outright, and a case that
// fails an assertion never reaches a restore of its own — so the stub stands
// for every later case in the file, which reads as a run of unrelated failures
// with the real one first and unremarkable. Registered here rather than per
// file because the leak belongs to every suite that stubs, including the ones
// nobody has written yet, and AFTER the probe check above so that under
// vitest's stacked hook order the navigator is handed back even when that
// check is the thing that throws.
afterEach(restoreClipboardStubs);

// The calendar-drift lane: run the whole suite as if it were N days from now.
//
// A test must not depend on the real clock. The half of that rule a grep can
// hold is small — "an absolute date in a file that never pins the clock" matches
// most of this suite's fixtures, nearly all of them harmless — because a date
// only misleads when the COMPONENT compares it to now to decide a state, and
// nothing static separates those from the dates a component merely formats. So
// the check is a second RUN: shift the clock, require the same verdict.
//
// Only the no-argument Date and Date.now move. Timers stay real, because a
// suite-wide vi.useFakeTimers would change what every async test is waiting for
// and report its own breakage as calendar drift. A test that pins its own clock
// overrides this and is unaffected, which is correct: it is already immune to
// what this looks for.
//
// `make fe-clock-drift` runs it; docs/reference/make-targets.md says why it runs
// daily on main rather than on a pull request.
const skewRequest = process.env.FE_CLOCK_SKEW_DAYS ?? "";
if (skewRequest !== "") {
  const days = Number(skewRequest);
  // A gate that cannot arm must FAIL, not run the ordinary suite and report
  // green. Number("") is 0 and Number("later") is NaN, so a typo or a stray
  // quote would otherwise shift no clock at all — and a lane that silently
  // checks nothing is the same colour as one that checked everything, which is
  // the confusion this lane was built to remove.
  if (!Number.isFinite(days) || days === 0) {
    throw new Error(
      `FE_CLOCK_SKEW_DAYS=${skewRequest} is not a non-zero number of days: the clock-drift lane ` +
        "would run the ordinary suite and report green over a check that never happened",
    );
  }
  const skewMs = days * 24 * 60 * 60 * 1000;
  const RealDate = globalThis.Date;
  class SkewedDate extends RealDate {
    constructor(...args: ConstructorParameters<typeof Date>) {
      if (args.length === 0) {
        super(RealDate.now() + skewMs);
        return;
      }
      super(...args);
    }
    static now(): number {
      return RealDate.now() + skewMs;
    }
  }
  globalThis.Date = SkewedDate as DateConstructor;
  // The instrument proves itself before any test runs. Everything this lane
  // reports rests on the clock actually having moved, and that is one
  // assignment away from being true of nothing.
  const shifted = Date.now() - RealDate.now();
  if (Math.abs(shifted - skewMs) > 1000) {
    throw new Error(
      `the clock-drift skew did not take: Date.now() is ${shifted}ms ahead, want ${skewMs}ms`,
    );
  }
}

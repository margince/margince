/** @vitest-environment happy-dom */
import {
  afterEach,
  beforeEach,
  describe,
  expect,
  it,
  type Mock,
  vi,
} from "vitest";
import {
  isStaleChunkError,
  reloadForNewBuild,
  reloadOnStaleBuild,
} from "./stalebuild";

// A tab older than the deploy asks for screen code the deploy removed. These
// cases pin that it reloads onto the new build once, and never twice in a row.

let reload: Mock<() => void>;

beforeEach(() => {
  sessionStorage.clear();
  reload = vi.fn<() => void>();
  vi.spyOn(window, "location", "get").mockReturnValue({
    ...window.location,
    reload,
  });
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("a stale build", () => {
  it("is recognised by each engine's failed-import message", () => {
    for (const message of [
      "Failed to fetch dynamically imported module: https://x/assets/settings-a1.js",
      "error loading dynamically imported module: https://x/assets/settings-a1.js",
      "Importing a module script failed.",
      "Unable to preload CSS for /assets/settings-a1.css",
    ]) {
      expect(isStaleChunkError(new TypeError(message))).toBe(true);
    }
  });

  it("is not confused with a defect in the screen", () => {
    expect(
      isStaleChunkError(new Error("cannot read properties of undefined")),
    ).toBe(false);
    expect(
      isStaleChunkError("Failed to fetch dynamically imported module"),
    ).toBe(false);
  });

  it("reloads the page once", () => {
    expect(reloadForNewBuild(1_000_000)).toBe(true);
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("does not reload again when the reload did not help", () => {
    reloadForNewBuild(1_000_000);
    expect(reloadForNewBuild(1_030_000)).toBe(false);
    expect(reload).toHaveBeenCalledTimes(1);
  });

  it("reloads for the next deploy later in the day", () => {
    reloadForNewBuild(1_000_000);
    expect(reloadForNewBuild(1_000_000 + 10 * 60_000)).toBe(true);
    expect(reload).toHaveBeenCalledTimes(2);
  });

  it("does not reload without storage, where nothing could stop a loop", () => {
    vi.spyOn(window, "sessionStorage", "get").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    expect(reloadForNewBuild()).toBe(false);
    expect(reload).not.toHaveBeenCalled();
  });

  it("reloads when Vite reports a chunk it could not load", () => {
    reloadOnStaleBuild();
    const event = new Event("vite:preloadError", { cancelable: true });
    window.dispatchEvent(event);
    expect(reload).toHaveBeenCalledTimes(1);
    expect(event.defaultPrevented).toBe(true);
  });
});

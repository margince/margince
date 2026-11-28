import userEvent from "@testing-library/user-event";
import { onTestFinished, vi } from "vitest";

// Testing Library advances fake timers only through a global `jest`; without
// one, every awaited interaction hangs. The shim lives for one test: suites on
// `shouldAdvanceTime` move their own clock, and it would fire their timers.
// Not a *.test.* file, so the lint gates read it as they read app code.

/**
 * Installs fake timers for the current test and returns a userEvent driver on
 * them. Call it once per test: a second driver keeps its own focus state, and
 * its clicks can then do nothing. Real timers come back when the test ends.
 */
export function steppedClock(
  options: Readonly<{
    toFake?: NonNullable<Parameters<typeof vi.useFakeTimers>[0]>["toFake"];
  }> = {},
) {
  vi.useFakeTimers(options.toFake ? { toFake: options.toFake } : undefined);
  Object.assign(globalThis, {
    jest: {
      advanceTimersByTime: (ms: number) => {
        vi.advanceTimersByTime(ms);
      },
    },
  });
  onTestFinished(() => {
    Reflect.deleteProperty(globalThis, "jest");
    vi.useRealTimers();
  });
  return userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
}

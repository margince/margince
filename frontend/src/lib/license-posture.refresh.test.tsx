/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { TRIAL_END_RETRY_MS, useRefreshAtTrialEnd } from "./license-posture";

type LicenseTrial = components["schemas"]["LicenseTrial"];

const NOW = Date.parse("2026-11-12T10:00:00Z");

function trial(endsIn: number, ended = false): LicenseTrial {
  return {
    started_at: new Date(NOW + endsIn - 14 * 86_400_000).toISOString(),
    ends_at: new Date(NOW + endsIn).toISOString(),
    ended,
  };
}

function mount(first: LicenseTrial) {
  const client = new QueryClient();
  const reads = vi.spyOn(client, "invalidateQueries");
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
  const hook = renderHook(
    ({ current }: { current: LicenseTrial }) => useRefreshAtTrialEnd(current),
    { wrapper, initialProps: { current: first } },
  );
  // Each refresh reads two queries: /me and the licence.
  const refreshes = () => reads.mock.calls.length / 2;
  return { hook, refreshes };
}

beforeEach(() => {
  vi.useFakeTimers();
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
});

it("reads the lock at the trial's end, not before", () => {
  const { refreshes } = mount(trial(60_000));
  vi.advanceTimersByTime(59_999);
  expect(refreshes()).toBe(0);
  vi.advanceTimersByTime(1);
  expect(refreshes()).toBe(1);
});

it("asks again while a server whose clock runs behind still reports the trial running", () => {
  const { refreshes } = mount(trial(0));
  vi.advanceTimersByTime(0);
  expect(refreshes()).toBe(1);
  vi.advanceTimersByTime(2 * TRIAL_END_RETRY_MS);
  expect(refreshes()).toBe(3);
});

it("stops asking once the server reports the trial ended", () => {
  const { hook, refreshes } = mount(trial(0));
  vi.advanceTimersByTime(0);
  hook.rerender({ current: trial(0, true) });
  vi.advanceTimersByTime(5 * TRIAL_END_RETRY_MS);
  expect(refreshes()).toBe(1);
});

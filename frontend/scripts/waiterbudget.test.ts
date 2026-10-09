/** @vitest-environment happy-dom */

// That a waiter stating no timeout runs on the budget vitest.budget.ts derives,
// and not on the libraries' own one second. vitest.setup.ts sets it for both
// Testing Library and `vi.waitFor`. Nothing reports it if either stops: the
// suite then mostly still passes, and fails only on a busy runner.
//
// Behavioural rather than a read of the config: each case waits on a condition
// that turns true only after the library default has passed.

import { waitFor } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { ASYNC_UTIL_TIMEOUT_MS } from "../vitest.budget";

const LIBRARY_DEFAULT_MS = 1_000;
const PAST_LIBRARY_DEFAULT_MS = LIBRARY_DEFAULT_MS + 200;

function trueAfter(ms: number): () => void {
  const start = performance.now();
  return () => {
    expect(performance.now() - start).toBeGreaterThan(ms);
  };
}

it("is wider than the library default it replaces", () => {
  expect(ASYNC_UTIL_TIMEOUT_MS).toBeGreaterThan(PAST_LIBRARY_DEFAULT_MS);
});

it("lets a Testing Library waiter outlast the library default", async () => {
  await waitFor(trueAfter(PAST_LIBRARY_DEFAULT_MS));
});

it("lets a vi.waitFor outlast the library default", async () => {
  await vi.waitFor(trueAfter(PAST_LIBRARY_DEFAULT_MS));
});

it("gives a vi.waitFor whose timeout is undefined the derived budget", async () => {
  await vi.waitFor(trueAfter(PAST_LIBRARY_DEFAULT_MS), { timeout: undefined });
});

it("keeps a vi.waitFor timeout the caller states", async () => {
  const never = trueAfter(60_000);
  const start = performance.now();
  await expect(vi.waitFor(never, { timeout: 50 })).rejects.toThrow();
  await expect(vi.waitFor(never, 50)).rejects.toThrow();
  expect(performance.now() - start).toBeLessThan(LIBRARY_DEFAULT_MS);
});

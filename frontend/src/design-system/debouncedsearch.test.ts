/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, renderHook, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { SEARCH_DEBOUNCE_MS, useDebouncedSearch } from "./debouncedsearch";

// What a picker over a set too large to enumerate must never do is show rows
// that no longer answer the reader's question. These are the two moments where
// it could.

describe("a debounced search", () => {
  it("keeps the last answer while the next one is still coming", async () => {
    const search = vi
      .fn()
      .mockResolvedValueOnce([{ value: "company-1", label: "Northgate" }])
      .mockImplementation(() => new Promise(() => {}));
    const { result, rerender } = renderHook(
      ({ query }) => useDebouncedSearch(search, query),
      { initialProps: { query: "north" } },
    );

    await waitFor(() => expect(result.current.results).toHaveLength(1));
    rerender({ query: "northg" });

    // Clearing here would empty the list under the reader on every keystroke,
    // and an empty list is the one thing that reads as a confident "no
    // matches". `pending` is what says a newer answer is on its way.
    await waitFor(() => expect(result.current.pending).toBe(true));
    expect(result.current.results).toHaveLength(1);
  });

  it("drops the last answer when the next search fails", async () => {
    const search = vi
      .fn()
      .mockResolvedValueOnce([{ value: "company-1", label: "Northgate" }])
      .mockRejectedValue(new Error("network"));
    const { result, rerender } = renderHook(
      ({ query }) => useDebouncedSearch(search, query),
      { initialProps: { query: "north" } },
    );

    await waitFor(() => expect(result.current.results).toHaveLength(1));
    rerender({ query: "zzz" });

    // The opposite call to the one above, for the opposite reason: no newer
    // answer is coming, so rows from a query no longer on screen would sit
    // under an error line contradicting them — and stay selectable.
    await waitFor(() => expect(result.current.failed).toBe(true));
    expect(result.current.results).toHaveLength(0);
  });

  it("drops the answer when the box goes back to empty", async () => {
    const search = vi
      .fn()
      .mockResolvedValue([{ value: "company-1", label: "Northgate" }]);
    const { result, rerender } = renderHook(
      ({ query }) => useDebouncedSearch(search, query),
      { initialProps: { query: "north" } },
    );

    await waitFor(() => expect(result.current.results).toHaveLength(1));
    rerender({ query: "" });

    // Results for a query nobody can see any more, and no request to replace
    // them: the empty box means the reader has asked nothing.
    await waitFor(() => expect(result.current.results).toHaveLength(0));
    expect(result.current.pending).toBe(false);
  });

  it("asks once for a burst of keystrokes", async () => {
    vi.useFakeTimers();
    const search = vi.fn().mockResolvedValue([]);
    const { rerender } = renderHook(
      ({ query }) => useDebouncedSearch(search, query),
      { initialProps: { query: "n" } },
    );
    rerender({ query: "no" });
    rerender({ query: "nor" });
    act(() => {
      vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS + 1);
    });

    // Typing a company name is one request, not one per letter — and the one
    // that runs is for what the reader has actually typed.
    expect(search).toHaveBeenCalledTimes(1);
    expect(search).toHaveBeenCalledWith("nor");
    vi.useRealTimers();
  });
});

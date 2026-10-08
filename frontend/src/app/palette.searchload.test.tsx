// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the palette's live search does under load: a burst of keystrokes is one
// question, a keystroke the reader has already typed past stops costing the
// server anything, and a search the server refuses as too broad says so in the
// server's words.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SEARCH_DEBOUNCE_MS } from "../design-system/debouncedsearch";
import { LocaleProvider } from "../i18n";
import { steppedClock } from "../testing/steppedclock";
import { CommandPalette } from "./palette";

afterEach(() => {
  cleanup();
  vi.useRealTimers();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

function renderPalette() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CommandPalette open onClose={() => {}} commands={[]} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function pause() {
  await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
}

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

// The `q` each request carried, read the way the server reads it.
function askedWords(fetchMock: ReturnType<typeof vi.fn>): string[] {
  return fetchMock.mock.calls.map(([input]) => {
    const url = input instanceof Request ? input.url : String(input);
    return new URL(url, "http://localhost").searchParams.get("q") ?? "";
  });
}

describe("a word typed in one burst", () => {
  it("is asked once, after the typing pauses, with the whole word", async () => {
    const user = steppedClock();
    const fetchMock = vi.fn(async () =>
      json({
        data: [{ type: "company", id: "o1", title: "Acme GmbH" }],
        page: { next_cursor: null, has_more: false },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderPalette();

    await user.type(screen.getByRole("searchbox"), "acme");
    expect(fetchMock).not.toHaveBeenCalled();
    await pause();

    expect(
      await screen.findByRole("button", { name: "Acme GmbH" }),
    ).toBeTruthy();
    expect(askedWords(fetchMock)).toEqual(["acme"]);
  });
});

describe("typing past a search that is still running", () => {
  it("aborts the request the newer words replaced, without calling it a failure", async () => {
    const user = steppedClock();
    const signals: AbortSignal[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (input instanceof Request) {
          signals.push(input.signal);
        }
        // Held until aborted, the way a ranking that runs to the ceiling is.
        return new Promise<Response>((_resolve, reject) => {
          if (input instanceof Request) {
            input.signal.addEventListener("abort", () =>
              reject(new DOMException("aborted", "AbortError")),
            );
          }
        });
      }),
    );
    renderPalette();

    await user.type(screen.getByRole("searchbox"), "ac");
    await pause();
    await waitFor(() => expect(signals.length).toBe(1));
    await user.type(screen.getByRole("searchbox"), "me");
    await pause();
    await waitFor(() => expect(signals.length).toBeGreaterThan(1));

    expect(signals[0].aborted).toBe(true);
    expect(screen.queryByText("Search failed")).toBeNull();
  });
});

describe("a hit still showing while the next words are asked", () => {
  // A message opens on the results screen for the words that found it; the
  // hits kept on screen while the next words are asked were found by the last.
  it("opens a message with the words that found it", async () => {
    const user = steppedClock();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL) =>
      fetchMock.mock.calls.length > 1
        ? new Promise<Response>(() => {})
        : json({
            data: [
              {
                type: "activity",
                id: "a1",
                title: "Re: Acme renewal",
                email_summary: {
                  activity_id: "a1",
                  subject: "Re: Acme renewal",
                  occurred_at: "2026-09-01T09:15:00Z",
                  counterparty: "Dana Buyer",
                },
              },
            ],
            page: { next_cursor: null, has_more: false },
          }),
    );
    vi.stubGlobal("fetch", fetchMock);
    renderPalette();

    await user.type(screen.getByRole("searchbox"), "acme");
    await pause();
    await screen.findByRole("button", { name: /Acme renewal/ });
    await user.type(screen.getByRole("searchbox"), " log");
    await pause();
    expect(askedWords(fetchMock)).toEqual(["acme", "acme log"]);

    await user.click(screen.getByRole("button", { name: /Acme renewal/ }));
    expect(window.location.hash).toBe("#/search/acme/a1");
  });
});

describe("a search the server refuses as too broad", () => {
  it("shows the server's advice instead of a bare failure", async () => {
    const user = steppedClock();
    const advice =
      "this search matched too much of the workspace to rank in time — add another word, or narrow it with types";
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            // The shape httperr.Validation renders a field fault into: the
            // top-level code is the shared validation one, and the search's own
            // code rides in the per-field list.
            JSON.stringify({
              title: "Unprocessable",
              status: 422,
              code: "validation_error",
              detail: advice,
              details: {
                errors: [
                  { field: "q", code: "query_too_broad", message: advice },
                ],
              },
            }),
            {
              status: 422,
              headers: { "content-type": "application/problem+json" },
            },
          ),
      ),
    );
    renderPalette();

    await user.type(screen.getByRole("searchbox"), "co");
    await pause();

    expect(await screen.findByText(advice)).toBeTruthy();
  });
});

describe("words typed before the pause after an earlier answer", () => {
  function answering(title: string) {
    return vi.fn(async () =>
      json({
        data: [{ type: "company", id: "o1", title }],
        page: { next_cursor: null, has_more: false },
      }),
    );
  }

  it("drops the earlier hits once the words change subject", async () => {
    const user = steppedClock();
    vi.stubGlobal("fetch", answering("Abbott AG"));
    renderPalette();
    const box = screen.getByRole("searchbox");

    await user.type(box, "ab");
    await pause();
    await screen.findByRole("button", { name: "Abbott AG" });
    await user.clear(box);
    await user.type(box, "nor");

    expect(screen.queryByRole("button", { name: "Abbott AG" })).toBeNull();
  });

  it("keeps the earlier hits while the words only grow", async () => {
    const user = steppedClock();
    vi.stubGlobal("fetch", answering("Acme GmbH"));
    renderPalette();
    const box = screen.getByRole("searchbox");

    await user.type(box, "acme");
    await pause();
    await screen.findByRole("button", { name: "Acme GmbH" });
    await user.type(box, " log");

    expect(screen.getByRole("button", { name: "Acme GmbH" })).toBeTruthy();
  });

  it("keeps the earlier hits when only the case changes", async () => {
    const user = steppedClock();
    vi.stubGlobal("fetch", answering("Acme GmbH"));
    renderPalette();
    const box = screen.getByRole("searchbox");

    await user.type(box, "acme");
    await pause();
    await screen.findByRole("button", { name: "Acme GmbH" });
    await user.clear(box);
    await user.type(box, "ACME");

    expect(screen.getByRole("button", { name: "Acme GmbH" })).toBeTruthy();
  });

  it("drops the earlier failure once the words change subject", async () => {
    const user = steppedClock();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("{}", { status: 500 })),
    );
    renderPalette();
    const box = screen.getByRole("searchbox");

    await user.type(box, "ab");
    await pause();
    await screen.findByText("Search failed");
    await user.clear(box);
    await user.type(box, "nor");

    expect(screen.queryByText("Search failed")).toBeNull();
  });
});

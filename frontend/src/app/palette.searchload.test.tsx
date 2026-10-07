// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the palette's live search does under load: a keystroke the reader has
// already typed past stops costing the server anything, and a search the server
// refuses as too broad says so in the server's words.

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { CommandPalette } from "./palette";

afterEach(() => {
  cleanup();
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

describe("typing past a search that is still running", () => {
  it("aborts the request the newer keystroke replaced", async () => {
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
    const user = userEvent.setup();
    renderPalette();

    await user.type(screen.getByRole("searchbox"), "ac");
    await waitFor(() => expect(signals.length).toBe(1));
    await user.type(screen.getByRole("searchbox"), "me");
    await waitFor(() => expect(signals.length).toBeGreaterThan(1));

    expect(signals[0].aborted).toBe(true);
  });
});

describe("a search the server refuses as too broad", () => {
  it("shows the server's advice instead of a bare failure", async () => {
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
    const user = userEvent.setup();
    renderPalette();

    await user.type(screen.getByRole("searchbox"), "co");

    expect(await screen.findByText(advice)).toBeTruthy();
  });
});

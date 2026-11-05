/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { DepartedOwnerCaption } from "./worklist.departedowner";

// A team view keeps a departed colleague's open work, so its row says the work
// needs a new owner. A row owned by somebody still here says nothing extra.

function names() {
  return vi.fn(
    async () =>
      new Response(
        JSON.stringify({
          data: [
            { id: "u-gone", display_name: "Shay Left", departed: true },
            { id: "u-here", display_name: "Ada Here", departed: false },
          ],
        }),
        { status: 200, headers: { "Content-Type": "application/json" } },
      ),
  );
}

function wrap(children: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a Worklist row owned by a colleague who left", () => {
  it("says it needs a new owner", async () => {
    vi.stubGlobal("fetch", names());
    render(
      wrap(<DepartedOwnerCaption owner={{ kind: "user", id: "u-gone" }} />),
    );
    expect(
      await screen.findByText("Shay Left left. This needs a new owner."),
    ).toBeTruthy();
  });

  it("says nothing for a colleague who is still here", async () => {
    const fetchMock = names();
    vi.stubGlobal("fetch", fetchMock);
    const { container } = render(
      wrap(<DepartedOwnerCaption owner={{ kind: "user", id: "u-here" }} />),
    );
    await vi.waitFor(() => expect(fetchMock).toHaveBeenCalled());
    expect(container.textContent).toBe("");
  });
});

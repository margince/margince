// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ClientSurfaceScreen } from "./client";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function stubSearch(hits: readonly unknown[]) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: hits,
            page: { next_cursor: null, has_more: false },
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        ),
    ),
  );
}

function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ClientSurfaceScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function lookUp(sender: string) {
  const user = userEvent.setup();
  await user.type(screen.getByLabelText("Sender"), sender);
  await user.click(screen.getByRole("button", { name: "Look up" }));
}

describe("ClientSurfaceScreen", () => {
  it("shows one row per contact hit the lookup returns, and drops other types", async () => {
    stubSearch([
      {
        id: "p-1",
        type: "contact",
        title: "Bettina Krause",
        snippet: "Head of Fleet",
      },
      { id: "p-2", type: "contact", title: "Jonas Brandt" },
      { id: "o-1", type: "company", title: "Brandt Automotive GmbH" },
    ]);
    render();
    await lookUp("brandt.example");

    expect(await screen.findByText("Bettina Krause")).toBeTruthy();
    expect(screen.getByText("Jonas Brandt")).toBeTruthy();
    expect(screen.getByText("Head of Fleet")).toBeTruthy();
    expect(screen.queryByText("Brandt Automotive GmbH")).toBeNull();
    const links = screen.getAllByRole("link", { name: "Open 360 view" });
    expect(links.map((link) => link.getAttribute("href"))).toEqual([
      "#/contacts/p-1",
      "#/contacts/p-2",
    ]);
  });

  it("offers to capture a lead when the sender matches no contact", async () => {
    stubSearch([{ id: "o-1", type: "company", title: "Brandt Automotive" }]);
    const { container } = render();
    await lookUp("stranger@nowhere.example");

    expect(await screen.findByText("Not in your company yet.")).toBeTruthy();
    const answer = container.querySelector<HTMLElement>(
      ".clientsurface-answer",
    );
    if (!answer) {
      throw new Error("the unknown-sender answer card never rendered");
    }
    expect(
      within(answer)
        .getByRole("link", { name: "Capture as lead" })
        .getAttribute("href"),
    ).toBe("#/leads");
  });
});

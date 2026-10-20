/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { ApprovalPreferencesCard } from "./approval-preferences";

function response(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
function mount() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ApprovalPreferencesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows the manual-policy consequence and sends only the reader's kind preference", async () => {
  const user = userEvent.setup();
  const changes: unknown[] = [];
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const req = input instanceof Request ? input : new Request(String(input));
    if (req.url.endsWith("/me")) return response(meFixture({}));
    if (req.method === "PATCH") {
      changes.push(await req.json());
      return response({
        data: [{ kind: "close_date_correction", muted: true }],
      });
    }
    return response({
      data: [{ kind: "close_date_correction", muted: false }],
    });
  });
  mount();
  expect(
    await screen.findByText(
      /Muting or restoring a kind turns off your automatic changes/,
    ),
  ).toBeTruthy();
  await user.click(await screen.findByRole("switch"));
  await waitFor(() =>
    expect(changes).toEqual([{ kind: "close_date_correction", muted: true }]),
  );
  expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("true");
});

it("opens an explicit include-muted review without changing the preference", async () => {
  const user = userEvent.setup();
  const reviews: URL[] = [];
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const req = input instanceof Request ? input : new Request(String(input));
    const url = new URL(req.url);
    if (url.pathname.endsWith("/me")) return response(meFixture({}));
    if (url.pathname.endsWith("/approvals")) {
      reviews.push(url);
      return response({ data: [], page: { has_more: false } });
    }
    expect(req.method).toBe("GET");
    return response({ data: [{ kind: "close_date_correction", muted: true }] });
  });
  mount();
  await user.click(
    await screen.findByRole("button", { name: "Review including muted" }),
  );
  await waitFor(() => expect(reviews).toHaveLength(1));
  expect(reviews[0].searchParams.get("include_muted")).toBe("true");
  expect(reviews[0].searchParams.has("status")).toBe(false);
  expect(reviews[0].searchParams.get("kind")).toBe("close_date_correction");
  expect(
    screen.getByText(/Bundles in your ordinary queue can show fewer members/),
  ).toBeTruthy();
});

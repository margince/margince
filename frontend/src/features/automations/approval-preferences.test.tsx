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

it("keeps the current preference when saving fails and shows the server explanation", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const request =
      input instanceof Request ? input : new Request(String(input));
    if (request.url.endsWith("/me")) return response(meFixture({}));
    if (request.method === "PATCH")
      return response(
        {
          title: "Conflict",
          status: 409,
          detail: "Preference changed in another session.",
          code: "conflict",
        },
        409,
      );
    return response({
      data: [{ kind: "close_date_correction", muted: false }],
    });
  });
  mount();
  await user.click(await screen.findByRole("switch"));
  expect(
    await screen.findByText("Preference changed in another session."),
  ).toBeTruthy();
  expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("false");
});

it("pages explicit muted history, returns to the first page, and closes review", async () => {
  const user = userEvent.setup();
  const cursors: (string | null)[] = [];
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const request =
      input instanceof Request ? input : new Request(String(input));
    expect(request.method).toBe("GET");
    const url = new URL(request.url);
    if (url.pathname.endsWith("/me")) return response(meFixture({}));
    if (url.pathname.endsWith("/approvals")) {
      expect(url.searchParams.get("include_muted")).toBe("true");
      expect(url.searchParams.get("kind")).toBe("close_date_correction");
      const next = url.searchParams.has("cursor");
      cursors.push(url.searchParams.get("cursor"));
      return response({
        data: [
          {
            id: next ? "approved-second" : "approved-first",
            kind: "close_date_correction",
            status: "approved",
            summary: next ? "Second reviewed date" : "First reviewed date",
            proposed_by: "system:close-date",
            created_at: "2026-08-20T09:00:00Z",
            proposed_change: {
              expected_close_date: "2026-11-15",
              basis: "Buyer confirmed timing",
            },
          },
        ],
        page: next
          ? { has_more: false }
          : { has_more: true, next_cursor: "older-reviews" },
      });
    }
    return response({ data: [{ kind: "close_date_correction", muted: true }] });
  });
  mount();
  await user.click(
    await screen.findByRole("button", { name: "Review including muted" }),
  );
  expect(await screen.findByText("First reviewed date")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Next page" }));
  expect(await screen.findByText("Second reviewed date")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Next page" })).toBeNull();
  await user.click(screen.getByRole("button", { name: "First page" }));
  expect(await screen.findByText("First reviewed date")).toBeTruthy();
  expect(cursors.slice(0, 2)).toEqual([null, "older-reviews"]);
  await user.click(screen.getByRole("button", { name: "Close review" }));
  expect(screen.queryByText("First reviewed date")).toBeNull();
  expect(screen.getByRole("switch").getAttribute("aria-checked")).toBe("true");
});

it("draws an answered proposal in the review as history, without verbs", async () => {
  const user = userEvent.setup();
  vi.stubGlobal("fetch", async (input: RequestInfo | URL) => {
    const request =
      input instanceof Request ? input : new Request(String(input));
    const url = new URL(request.url);
    if (url.pathname.endsWith("/me")) return response(meFixture({}));
    if (url.pathname.endsWith("/approvals")) {
      return response({
        data: ["rejected", "pending"].map((status) => ({
          id: `ap-${status}`,
          kind: "close_date_correction",
          status,
          summary: `A ${status} date`,
          proposed_by: "system:close-date",
          created_at: "2026-08-20T09:00:00Z",
          proposed_change: { expected_close_date: "2026-11-15" },
        })),
        page: { has_more: false },
      });
    }
    return response({ data: [{ kind: "close_date_correction", muted: true }] });
  });
  mount();
  await user.click(
    await screen.findByRole("button", { name: "Review including muted" }),
  );
  await screen.findByText("A rejected date");

  expect(screen.getAllByText("Rejected")).toHaveLength(1);
  expect(screen.getAllByRole("button", { name: "Reject" })).toHaveLength(1);
  expect(screen.getAllByRole("button", { name: "Approve" })).toHaveLength(1);
});

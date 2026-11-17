// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { ArchiveReviewCard } from "./archivereview";

const FREE = "00000000-0000-4000-8000-0000000000a1";
const HELD = "00000000-0000-4000-8000-0000000000a2";

const PAGE = {
  data: [
    {
      activity_id: FREE,
      kind: "email",
      subject: "Old offer",
      occurred_at: "2022-03-01T10:00:00Z",
      occurred_precision: "instant",
      archived_at: "2025-03-01T10:00:00Z",
      contacts: [{ id: "c1", name: "Ada Archived" }],
      companies: [],
    },
    {
      activity_id: HELD,
      kind: "email",
      subject: "Disputed invoice",
      occurred_at: "2022-04-01T10:00:00Z",
      occurred_precision: "instant",
      archived_at: "2025-04-01T10:00:00Z",
      contacts: [],
      companies: [{ id: "o1", name: "Held Company" }],
      kept: "legal_hold",
    },
  ],
  page: { has_more: false, next_cursor: null },
};

const reply = (body: unknown) =>
  new Response(JSON.stringify(body), {
    headers: { "Content-Type": "application/json" },
  });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(allow: Record<string, string[]>) {
  const deletes: unknown[] = [];
  const lists: URLSearchParams[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(request.url);
      if (url.pathname.endsWith("/v1/me")) return reply(meFixture({ allow }));
      if (url.pathname.endsWith("/retention/archive/delete")) {
        deletes.push(await request.json());
        return reply({
          deleted: [FREE],
          kept: [],
        });
      }
      if (url.pathname.endsWith("/retention/archive")) {
        lists.push(url.searchParams);
        return reply(PAGE);
      }
      throw new Error(`Unexpected request: ${request.method} ${url.pathname}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ArchiveReviewCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { deletes, lists };
}

it("deletes only the shown records nothing keeps, and reports what it did", async () => {
  const user = userEvent.setup();
  const { deletes } = show({ retention_policy: ["read", "delete"] });
  expect(await screen.findByText("Old offer")).toBeInTheDocument();
  expect(screen.getByText("Kept: legal hold")).toBeInTheDocument();
  await user.click(
    await screen.findByRole("button", { name: "Delete 1 shown record" }),
  );
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("button", { name: "Delete" }));
  await waitFor(() => expect(deletes).toEqual([{ activity_ids: [FREE] }]));
  expect(await screen.findByText("1 record deleted.")).toBeInTheDocument();
});

it("offers no deletion to a reader without the delete grant", async () => {
  show({ retention_policy: ["read"] });
  expect(await screen.findByText("Old offer")).toBeInTheDocument();
  expect(
    screen.queryByRole("button", { name: /Delete .* shown/ }),
  ).not.toBeInTheDocument();
});

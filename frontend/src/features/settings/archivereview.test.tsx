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
import { en as englishCopy } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
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

const reply = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

type DeleteReply = (body: { activity_ids: string[] }, call: number) => Response;

const deleteFree: DeleteReply = () =>
  reply({ deleted: [FREE], kept: [], failed: [] });

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function show(
  allow: GrantSpec,
  page: unknown = PAGE,
  onDelete: DeleteReply = deleteFree,
) {
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
        const body = await request.json();
        deletes.push(body);
        return onDelete(body, deletes.length);
      }
      if (url.pathname.endsWith("/retention/archive")) {
        lists.push(url.searchParams);
        return reply(page);
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
  expect(
    screen.getByText(englishCopy["archiveReview.kept.legalHold"]),
  ).toBeInTheDocument();
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

it("shows what was done before a later request failed, and reloads the archive", async () => {
  const user = userEvent.setup();
  const rows = Array.from({ length: 101 }, (_, n) => ({
    ...PAGE.data[0],
    activity_id: `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`,
    subject: `Old offer ${n}`,
  }));
  const { deletes, lists } = show(
    { retention_policy: ["read", "delete"] },
    { data: rows, page: PAGE.page },
    (body, call) =>
      call === 1
        ? reply({
            deleted: body.activity_ids.slice(1),
            kept: [],
            failed: body.activity_ids.slice(0, 1),
          })
        : reply({ code: "internal", message: "The server failed." }, 500),
  );
  await user.click(
    await screen.findByRole("button", { name: "Delete 101 shown records" }),
  );
  const dialog = await screen.findByRole("dialog");
  await user.click(within(dialog).getByRole("button", { name: "Delete" }));
  await waitFor(() => expect(deletes).toHaveLength(2));
  expect(
    await within(dialog).findByText(englishCopy["archiveReview.partialTitle"]),
  ).toBeInTheDocument();
  expect(within(dialog).getByText("99 records deleted.")).toBeInTheDocument();
  expect(
    within(dialog).getByText(
      "1 record could not be deleted and stays as it was.",
    ),
  ).toBeInTheDocument();
  await waitFor(() => expect(lists.length).toBeGreaterThan(1));
});

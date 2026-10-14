/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  LIST_PAGE_SIZES,
  type ListPage,
  type ListQuery,
  ListTable,
  useListQuery,
} from "./listquery";

// A view tab names the whole list, down to the search box: picking one puts its
// search on the wire and in the box, and a preset asks for none.

type Row = { id: string; name: string };

afterEach(() => {
  cleanup();
});

function emptyPage(): ListPage<Row> {
  return { data: [], page: { next_cursor: null, has_more: false } };
}

function Rail({
  fetchPage,
}: Readonly<{
  fetchPage: (
    query: ListQuery,
    cursor: string | null,
  ) => Promise<ListPage<Row>>;
}>) {
  const state = useListQuery<Row>({ key: "tab-search", fetchPage });
  return (
    <ListTable
      state={state}
      unit="nav.contacts"
      columns={[
        { key: "name", header: "contacts.name", cell: (row: Row) => row.name },
      ]}
      rowKey={(row) => row.id}
      views={[{ label: "list.viewAll" }]}
      dataViews={[
        {
          id: "v-1",
          label: "Acme",
          q: "acme",
          sort: "",
          filters: {},
          includeArchived: false,
          perPage: LIST_PAGE_SIZES[0],
        },
      ]}
    />
  );
}

function mount(
  fetchPage: (
    query: ListQuery,
    cursor: string | null,
  ) => Promise<ListPage<Row>>,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Rail fetchPage={fetchPage} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("a view tab and the search box", () => {
  it("applies the search a tab was saved with, and a preset clears it", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    mount(fetchPage);
    const search = await screen.findByPlaceholderText("Search");
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());

    fireEvent.click(screen.getByRole("button", { name: "Acme" }));

    await waitFor(() =>
      expect(fetchPage.mock.calls.at(-1)?.[0].q).toBe("acme"),
    );
    expect(search).toHaveProperty("value", "acme");
    expect(
      screen.getByRole("button", { name: "Acme" }).getAttribute("aria-pressed"),
    ).toBe("true");

    fireEvent.click(screen.getByRole("button", { name: "All" }));

    await waitFor(() => expect(fetchPage.mock.calls.at(-1)?.[0].q).toBe(""));
    expect(search).toHaveProperty("value", "");
    expect(
      screen.getByRole("button", { name: "All" }).getAttribute("aria-pressed"),
    ).toBe("true");
  });

  it("drops a word still settling when a tab is pressed", async () => {
    const fetchPage = vi.fn(async (_query: ListQuery, _cursor: string | null) =>
      emptyPage(),
    );
    mount(fetchPage);
    const search = await screen.findByPlaceholderText("Search");
    await waitFor(() => expect(fetchPage).toHaveBeenCalled());

    // All asks for no search, which is already the list's search, so nothing
    // but the press itself can stop the typed word from landing on it.
    vi.useFakeTimers();
    try {
      fireEvent.change(search, { target: { value: "glo" } });
      fireEvent.click(screen.getByRole("button", { name: "All" }));
      await act(async () => {
        vi.advanceTimersByTime(500);
        await Promise.resolve();
      });
    } finally {
      vi.useRealTimers();
    }

    expect(fetchPage.mock.calls.some(([query]) => query.q === "glo")).toBe(
      false,
    );
    expect(search).toHaveProperty("value", "");
    expect(
      screen.getByRole("button", { name: "All" }).getAttribute("aria-pressed"),
    ).toBe("true");
  });
});

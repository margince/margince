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
} from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  type ListPage,
  ListTable,
  SEARCH_DEBOUNCE_MS,
  useListQuery,
} from "./listquery";

// Back moves the address at `popstate` and announces it at `hashchange`, a task
// later. A search box whose debounce fires in that gap must not write over the
// list the reader has just returned to.

type Row = { id: string; name: string };

function emptyPage(): Promise<ListPage<Row>> {
  return Promise.resolve({
    data: [],
    page: { next_cursor: null, has_more: false },
  });
}

function SearchableList() {
  const state = useListQuery<Row>({ key: "leave", fetchPage: emptyPage });
  return (
    <ListTable
      state={state}
      unit="unit.contacts"
      searchable
      columns={[{ key: "name", header: "Name", cell: (row: Row) => row.name }]}
      rowKey={(row) => row.id}
    />
  );
}

function renderList() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SearchableList />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

/** The address Back restores, before the app has been told it moved. */
function backLandsOn(hash: string) {
  globalThis.history.replaceState(null, "", hash);
}

// happy-dom announces a replaceState with a `hashchange`, which a browser never
// does; held back, so each case runs inside the gap it is about.
const holdHashchange = (event: Event) => event.stopImmediatePropagation();

beforeEach(() => {
  globalThis.addEventListener("hashchange", holdHashchange);
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  globalThis.history.replaceState(null, "", "#/");
  globalThis.removeEventListener("hashchange", holdHashchange);
});

describe("a search box the reader is leaving", () => {
  it("writes nothing when nobody typed in it", async () => {
    // A record page mounts its own searchable list, and a reader who opened the
    // wrong record presses Back at once.
    globalThis.history.replaceState(null, "", "#/companies/o-brandt");
    vi.useFakeTimers();
    renderList();

    backLandsOn("#/companies?q=brandt");
    await act(async () => {
      vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS);
    });

    expect(globalThis.location.hash).toBe("#/companies?q=brandt");
  });

  it("drops a word still settling when Back lands, before its hashchange does", async () => {
    globalThis.history.replaceState(null, "", "#/contacts");
    vi.useFakeTimers();
    renderList();
    fireEvent.change(screen.getByPlaceholderText("Search"), {
      target: { value: "acme" },
    });

    backLandsOn("#/companies?q=brandt");
    // Two acts, as in the browser: a `popstate` update is discrete, so React
    // renders it before the next task, and the timer is one.
    await act(async () => {
      globalThis.dispatchEvent(new PopStateEvent("popstate"));
    });
    await act(async () => {
      vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS);
    });

    expect(globalThis.location.hash).toBe("#/companies?q=brandt");
  });
});

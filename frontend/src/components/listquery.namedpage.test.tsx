/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { replaceDial } from "../lib/urlstate";
import { render } from "../testing/listquery.testkit";
import {
  type ListPage,
  ListTable,
  SEARCH_DEBOUNCE_MS,
  useListQuery,
} from "./listquery";

// A list that stays mounted while its address changes. A link names the page
// it wants; a dial the reader turns narrows the list and starts it at page one.

type Row = { id: string; name: string };

const ROWS: Row[] = Array.from({ length: 100 }, (_, at) => ({
  id: `c-${at + 1}`,
  name: `Contact ${at + 1}`,
}));

function allRows(): Promise<ListPage<Row>> {
  return Promise.resolve({
    data: ROWS,
    page: { next_cursor: null, has_more: false },
  });
}

function ContactList() {
  const state = useListQuery<Row>({ key: "named-page", fetchPage: allRows });
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

function renderListAt(hash: string) {
  globalThis.history.replaceState(null, "", hash);
  render(<ContactList />);
}

/** The reader opens a link in the same tab. */
async function followLink(hash: string) {
  await act(async () => {
    globalThis.history.pushState(null, "", hash);
    globalThis.dispatchEvent(new PopStateEvent("popstate"));
  });
}

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  globalThis.history.replaceState(null, "", "#/");
});

describe("a link to a later page of the list on screen", () => {
  it("opens that page when it drops the search", async () => {
    renderListAt("#/contacts?q=a");
    await screen.findByText("Contact 1");

    await followLink("#/contacts?page=3");

    expect(await screen.findByText("Contact 51")).toBeInTheDocument();
    expect(globalThis.location.hash).toBe("#/contacts?page=3");
  });

  it("opens that page when it drops the sort", async () => {
    renderListAt("#/contacts?sort=name");
    await screen.findByText("Contact 1");

    await followLink("#/contacts?page=3");

    expect(await screen.findByText("Contact 51")).toBeInTheDocument();
    expect(globalThis.location.hash).toBe("#/contacts?page=3");
  });

  it("opens that page when the list was already on it", async () => {
    renderListAt("#/contacts?page=3&q=a");
    await screen.findByText("Contact 51");

    await followLink("#/contacts?page=3");

    expect(await screen.findByText("Contact 51")).toBeInTheDocument();
    expect(globalThis.location.hash).toBe("#/contacts?page=3");
  });
});

describe("Back to a later page the pager opened", () => {
  it("stays on that page after a link narrowed the list", async () => {
    renderListAt("#/contacts");
    await screen.findByText("Contact 1");
    // The writer the pager's press goes through.
    await act(async () => replaceDial("page", "3"));
    await screen.findByText("Contact 51");
    await followLink("#/contacts?q=x");
    await screen.findByText("Contact 1");

    await act(async () => {
      globalThis.history.back();
      await new Promise((settled) => setTimeout(settled, 0));
    });

    expect(await screen.findByText("Contact 51")).toBeInTheDocument();
    expect(globalThis.location.hash).toBe("#/contacts?page=3");
  });
});

describe("a dial the reader turns on a later page", () => {
  it("starts at page one after a link that named only the page", async () => {
    renderListAt("#/contacts");
    await screen.findByText("Contact 1");
    await followLink("#/contacts?page=3");
    await screen.findByText("Contact 51");
    vi.useFakeTimers();

    fireEvent.change(screen.getByPlaceholderText("Search"), {
      target: { value: "b" },
    });
    await act(async () => {
      vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS);
    });

    expect(globalThis.location.hash).toBe("#/contacts?q=b");
  });

  it("starts the narrowed list at page one", async () => {
    renderListAt("#/contacts?q=a");
    await screen.findByText("Contact 1");
    await followLink("#/contacts?page=3");
    await screen.findByText("Contact 51");
    vi.useFakeTimers();

    fireEvent.change(screen.getByPlaceholderText("Search"), {
      target: { value: "b" },
    });
    await act(async () => {
      vi.advanceTimersByTime(SEARCH_DEBOUNCE_MS);
    });

    expect(globalThis.location.hash).toBe("#/contacts?q=b");
  });

  it("starts a list sorted anew at page one", async () => {
    renderListAt("#/contacts?q=a");
    await screen.findByText("Contact 1");
    await followLink("#/contacts?page=3");
    await screen.findByText("Contact 51");

    // The writer the sort control's change goes through.
    await act(async () => replaceDial("sort", "full_name"));

    expect(globalThis.location.hash).toBe("#/contacts?sort=full_name");
  });
});

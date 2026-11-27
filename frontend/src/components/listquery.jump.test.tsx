/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { render } from "../testing/listquery.testkit";
import {
  type ListPage,
  type ListQuery,
  type ListReach,
  ListTable,
  listFetchLimit,
  SEARCH_DEBOUNCE_MS,
  useListQuery,
} from "./listquery";
import { jumpTarget } from "./listquery.jump";

type Row = { id: string; name: string };

const rowAt = (at: number): Row => ({
  id: `c-${at}`,
  name: `Contact ${at + 1}`,
});

/**
 * An endpoint that counts and reads at an offset, as /contacts does. A cursor
 * names the row it resumes at, and a search for "few" matches 60 rows.
 */
function serverOf(all: number) {
  const reads: { cursor: string | null; reach?: ListReach }[] = [];
  const fetchPage = vi.fn(
    (
      query: ListQuery,
      cursor: string | null,
      _signal: AbortSignal,
      reach?: ListReach,
    ): Promise<ListPage<Row>> => {
      reads.push({ cursor, reach });
      const matching = query.q === "few" ? 60 : all;
      const start = reach?.offset ?? Number(cursor ?? 0);
      const end = Math.min(
        matching,
        start + listFetchLimit(query.perPage, reach),
      );
      const counts = cursor === null;
      return Promise.resolve({
        data: Array.from({ length: Math.max(0, end - start) }, (_, index) =>
          rowAt(start + index),
        ),
        page: {
          next_cursor: end < matching ? String(end) : null,
          has_more: end < matching,
          total: counts ? matching : undefined,
        },
      });
    },
  );
  return { reads, fetchPage };
}

function ContactList({
  fetchPage,
}: Readonly<{ fetchPage: ReturnType<typeof serverOf>["fetchPage"] }>) {
  const state = useListQuery<Row>({
    key: "jumped-contacts",
    fetchPage,
    readsAtOffset: true,
  });
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

function renderListAt(hash: string, all = 3412) {
  globalThis.history.replaceState(null, "", hash);
  const server = serverOf(all);
  render(<ContactList fetchPage={server.fetchPage} />);
  return server;
}

const pageButton = (number: number) =>
  screen.getByRole("button", { name: `Page ${number}` });

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  globalThis.history.replaceState(null, "", "#/");
});

describe("jumpTarget", () => {
  const prefix = { rows: 50, hasMore: true, pending: false };

  it("leaves a page the cursor's rows hold, or its first read will, to them", () => {
    expect(jumpTarget(2, 25, prefix)).toBeUndefined();
    expect(jumpTarget(2, 25, { ...prefix, rows: 0, pending: true })).toBe(
      undefined,
    );
    expect(jumpTarget(4, 25, { ...prefix, rows: 100 })).toBeUndefined();
  });

  it("reads nothing past a list whose every row is in hand", () => {
    expect(jumpTarget(9, 25, { ...prefix, hasMore: false })).toBeUndefined();
  });

  it("starts on a read boundary, so the next page is in the same read", () => {
    expect(jumpTarget(137, 25, prefix)).toEqual({
      at: 3400,
      pastCap: false,
      from: 3400,
      perPage: 25,
    });
    expect(jumpTarget(138, 25, prefix)?.at).toBe(3400);
    expect(jumpTarget(139, 25, prefix)?.at).toBe(3450);
  });

  it("reads at the cap and walks on from there past it", () => {
    expect(jumpTarget(402, 25, prefix)).toMatchObject({
      at: 10000,
      pastCap: false,
    });
    expect(jumpTarget(403, 25, prefix)).toMatchObject({
      at: 10000,
      pastCap: true,
      from: 10050,
    });
  });
});

describe("a page past the cursor's rows, on a list read at an offset", () => {
  it("opens a pasted link to the last page in one read", async () => {
    const server = renderListAt("#/contacts?page=137");

    await screen.findByText("Contact 3401");
    expect(server.reads).toEqual([
      { cursor: null, reach: undefined },
      { cursor: null, reach: { offset: 3400, pastCap: false } },
    ]);
    expect(pageButton(137)).toHaveAttribute("aria-current", "page");
    expect(screen.getByRole("button", { name: "Next" })).toBeDisabled();
  });

  it("opens a page the reader clicks in one read, and the page after it in none", async () => {
    const user = userEvent.setup();
    const server = renderListAt("#/contacts");
    await screen.findByText("Contact 1");

    await user.click(pageButton(137));
    await screen.findByText("Contact 3401");
    await user.click(pageButton(136));
    await screen.findByText("Contact 3376");
    await user.click(pageButton(135));
    await screen.findByText("Contact 3351");

    expect(server.reads.map((read) => read.reach?.offset)).toEqual([
      undefined,
      3400,
      3350,
    ]);
    expect(server.reads.every((read) => read.cursor === null)).toBe(true);
  });

  it("walks the cursor on from the cap in the widest reads past it", async () => {
    const server = renderListAt("#/contacts?page=500", 20000);

    await screen.findByText("Contact 12476");
    const [, atCap, ...walk] = server.reads;
    expect(atCap).toEqual({
      cursor: null,
      reach: { offset: 10000, pastCap: true },
    });
    expect(walk).toHaveLength(Math.ceil((12500 - 10000) / 200) - 1);
    expect(walk.every((read) => read.reach?.pastCap === true)).toBe(true);
  });

  it("starts a narrowed list at page one, numbered by its new count", async () => {
    renderListAt("#/contacts?page=137");
    await screen.findByText("Contact 3401");
    vi.useFakeTimers();

    fireEvent.change(screen.getByPlaceholderText("Search"), {
      target: { value: "few" },
    });
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));

    expect(globalThis.location.hash).toBe("#/contacts?q=few");
    expect(screen.getByText("Contact 1")).toBeInTheDocument();
    expect(pageButton(3)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Page 137" })).toBeNull();
  });
});

/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useRowSelection } from "../design-system/listselection";
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
import { heldRows, jumpTarget } from "./listquery.jump";

type Row = { id: string; name: string };

const rowAt = (at: number): Row => ({
  id: `c-${at}`,
  name: `Contact ${at + 1}`,
});

/**
 * An endpoint that counts and reads at an offset, as /contacts does. A cursor
 * names the row it resumes at, and a search for "few" matches 60 rows.
 */
function serverOf(
  all: number,
  answers: Readonly<{
    /** A read that fails, the first time it is asked. */
    failsOnce?: string;
    /** A query whose reads never answer. */
    holds?: (query: ListQuery) => boolean;
  }> = {},
) {
  const reads: { cursor: string | null; reach?: ListReach }[] = [];
  const failed = new Set<string>();
  const fetchPage = vi.fn(
    (
      query: ListQuery,
      cursor: string | null,
      _signal: AbortSignal,
      reach?: ListReach,
    ): Promise<ListPage<Row>> => {
      reads.push({ cursor, reach });
      if (answers.holds?.(query)) {
        return new Promise(() => undefined);
      }
      if (
        cursor !== null &&
        cursor === answers.failsOnce &&
        !failed.has(cursor)
      ) {
        failed.add(cursor);
        return Promise.reject(new Error("the read failed"));
      }
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

// The rows a bulk verb was pressed for.
const acted: Row[] = [];

function ContactList({
  fetchPage,
}: Readonly<{ fetchPage: ReturnType<typeof serverOf>["fetchPage"] }>) {
  const state = useListQuery<Row>({
    key: "jumped-contacts",
    fetchPage,
    readsAtOffset: { rowKey: (row) => row.id },
  });
  const chosen = useRowSelection({
    rows: state.held,
    keyOf: (row) => row.id,
  });
  return (
    <ListTable
      state={state}
      unit="unit.contacts"
      searchable
      columns={[
        {
          key: "name",
          header: "Name",
          cell: (row: Row) => row.name,
          fixed: true,
        },
      ]}
      rowKey={(row) => row.id}
      selection={{
        selected: chosen.selected,
        limit: chosen.limit,
        onChange: chosen.onChange,
        label: (row) => `Select ${row.name}`,
        bar: (
          <>
            <span>{chosen.selectedRows.length} picked</span>
            <button
              type="button"
              onClick={() => acted.push(...chosen.selectedRows)}
            >
              Act
            </button>
          </>
        ),
      }}
    />
  );
}

function renderListAt(
  hash: string,
  all = 3412,
  answers: Parameters<typeof serverOf>[1] = {},
) {
  globalThis.history.replaceState(null, "", hash);
  const server = serverOf(all, answers);
  render(<ContactList fetchPage={server.fetchPage} />);
  return server;
}

const pageButton = (number: number) =>
  screen.getByRole("button", { name: `Page ${number}` });

afterEach(() => {
  acted.length = 0;
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

describe("heldRows", () => {
  it("holds a record both reads carry once", () => {
    const keyOf = (row: Row) => row.id;
    expect(heldRows([rowAt(0), rowAt(1)], [rowAt(1), rowAt(2)], keyOf)).toEqual(
      [rowAt(0), rowAt(1), rowAt(2)],
    );
  });
});

describe("a row on a page read at an offset", () => {
  it("can be selected, and a bulk verb receives it", async () => {
    const user = userEvent.setup();
    renderListAt("#/contacts?page=137");
    await screen.findByText("Contact 3401");

    await user.click(
      screen.getByRole("checkbox", { name: "Select Contact 3401" }),
    );

    expect(screen.getByText("1 picked")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Act" }));
    expect(acted).toEqual([rowAt(3400)]);
  });
});

describe("a read that fails on the walk past the cap", () => {
  it("stops the walk, shows the failure, and walks on after a retry", async () => {
    const user = userEvent.setup();
    const server = renderListAt("#/contacts?page=500", 20000, {
      failsOnce: "10200",
    });

    const retry = await screen.findByRole("button", { name: "Retry" });
    const failedAt = server.reads.length;
    expect(server.reads.at(-1)?.cursor).toBe("10200");
    expect(pageButton(500)).not.toHaveAttribute("aria-busy");
    await act(() => Promise.resolve());
    expect(server.reads).toHaveLength(failedAt);

    await user.click(retry);
    expect(await screen.findByText("Contact 12476")).toBeInTheDocument();
  });
});

describe("a dial the reader turns before the first rows land", () => {
  it("starts the narrowed list at page one", async () => {
    renderListAt("#/contacts?page=137", 3412, {
      holds: (query) => query.q === "",
    });
    vi.useFakeTimers();

    fireEvent.change(screen.getByPlaceholderText("Search"), {
      target: { value: "few" },
    });
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));
    await act(() => vi.advanceTimersByTimeAsync(SEARCH_DEBOUNCE_MS));

    expect(globalThis.location.hash).toBe("#/contacts?q=few");
    expect(screen.getByText("Contact 1")).toBeInTheDocument();
  });
});

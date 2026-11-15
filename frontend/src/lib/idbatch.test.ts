// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, expect, it, vi } from "vitest";
import { endIdBatches, idBatcher } from "./idbatch";

const PAGE_MS = 100;

afterEach(() => {
  vi.useRealTimers();
});

// A transport that answers every id by name after PAGE_MS, and counts the
// requests and the most it ever had open at once.
function slowTransport(
  refuse: (page: readonly string[]) => unknown = () => null,
) {
  const seen = { requests: 0, open: 0, peak: 0 };
  const read = (page: readonly string[]) =>
    new Promise<ReadonlyMap<string, string>>((resolve, reject) => {
      seen.requests += 1;
      seen.open += 1;
      seen.peak = Math.max(seen.peak, seen.open);
      setTimeout(() => {
        seen.open -= 1;
        const problem = refuse(page);
        if (problem) {
          reject(problem);
          return;
        }
        resolve(new Map(page.map((id) => [id, `Name ${id}`])));
      }, PAGE_MS);
    });
  return { seen, read };
}

function ids(count: number, prefix = "id-") {
  return Array.from({ length: count }, (_, at) => `${prefix}${at}`);
}

// Resolves to the fake clock time at which every id was answered.
function finishedAt(names: Promise<unknown>) {
  return names.then(() => Date.now());
}

it("reads one page for a batch of 100 ids", async () => {
  vi.useFakeTimers({ now: 0 });
  const transport = slowTransport();
  const read = idBatcher(100, transport.read);
  const done = finishedAt(Promise.all(ids(100).map(read)));
  await vi.runAllTimersAsync();
  expect(await done).toBe(PAGE_MS);
  expect(transport.seen).toMatchObject({ requests: 1, peak: 1 });
});

it.each([
  { count: 101, requests: 2, rounds: 1 },
  { count: 201, requests: 3, rounds: 2 },
  { count: 501, requests: 6, rounds: 3 },
])(
  "answers $count ids after $rounds page waits with at most two pages open",
  async ({ count, requests, rounds }) => {
    vi.useFakeTimers({ now: 0 });
    const transport = slowTransport();
    const read = idBatcher(100, transport.read);
    const asked = ids(count);
    const names = Promise.all(asked.map(read));
    const done = finishedAt(names);
    await vi.runAllTimersAsync();
    expect(await done).toBe(rounds * PAGE_MS);
    expect(await names).toEqual(asked.map((id) => `Name ${id}`));
    expect(transport.seen).toMatchObject({ requests, peak: 2 });
  },
);

it("tells a missing id from a blank name and asks once per distinct id", async () => {
  const readPage = vi.fn(
    async (_page: readonly string[]) =>
      new Map([
        ["named", "Name"],
        ["blank", ""],
      ]),
  );
  const read = idBatcher(100, readPage);
  const names = ["named", "named", "blank", "missing"].map(read);
  expect(await Promise.all(names)).toEqual(["Name", "Name", "", null]);
  expect(readPage.mock.calls).toEqual([[["named", "blank", "missing"]]]);
});

it("holds one refused page against its own ids and answers the rest", async () => {
  vi.useFakeTimers({ now: 0 });
  const refusal = { status: 403, title: "Forbidden" };
  const transport = slowTransport((page) =>
    page.includes("id-100") ? refusal : null,
  );
  const read = idBatcher(100, transport.read);
  const asked = ids(250);
  const settled = Promise.allSettled(asked.map(read));
  await vi.runAllTimersAsync();
  const results = await settled;
  expect(results.slice(100, 200)).toEqual(
    asked.slice(100, 200).map(() => ({ status: "rejected", reason: refusal })),
  );
  expect([...results.slice(0, 100), ...results.slice(200)]).toEqual(
    [...asked.slice(0, 100), ...asked.slice(200)].map((id) => ({
      status: "fulfilled",
      value: `Name ${id}`,
    })),
  );
  expect(transport.seen).toMatchObject({ requests: 3, peak: 2 });
});

it("frees a page's turn when the reader throws before it returns a promise", async () => {
  const failure = new Error("Read refused");
  const readPage = vi.fn((page: readonly string[]) => {
    if (page.includes("failed")) {
      throw failure;
    }
    return Promise.resolve(new Map(page.map((id) => [id, id])));
  });
  const read = idBatcher(1, readPage);
  const settled = await Promise.allSettled(
    ["failed", "next", "last"].map(read),
  );
  expect(settled).toEqual([
    { status: "rejected", reason: failure },
    { status: "fulfilled", value: "next" },
    { status: "fulfilled", value: "last" },
  ]);
  expect(readPage).toHaveBeenCalledTimes(3);
});

it("answers a later one-id batch without waiting behind a large older batch", async () => {
  vi.useFakeTimers({ now: 0 });
  const transport = slowTransport();
  const read = idBatcher(100, transport.read);
  const background = finishedAt(Promise.all(ids(501, "old-").map(read)));
  await vi.advanceTimersByTimeAsync(10);
  const foreground = finishedAt(read("new"));
  await vi.runAllTimersAsync();
  expect(await foreground).toBe(10 + PAGE_MS);
  expect(await background).toBe(3 * PAGE_MS);
  // Two pages for the older batch, one for the later batch.
  expect(transport.seen).toMatchObject({ requests: 7, peak: 3 });
});

it("lets two large batches from different ticks progress side by side", async () => {
  vi.useFakeTimers({ now: 0 });
  const transport = slowTransport();
  const read = idBatcher(100, transport.read);
  const first = finishedAt(Promise.all(ids(201, "a-").map(read)));
  await vi.advanceTimersByTimeAsync(5);
  const second = finishedAt(Promise.all(ids(201, "b-").map(read)));
  await vi.runAllTimersAsync();
  expect(await first).toBe(2 * PAGE_MS);
  expect(await second).toBe(5 + 2 * PAGE_MS);
  expect(transport.seen).toMatchObject({ requests: 6, peak: 4 });
});

function heldReader() {
  const opened: { page: readonly string[]; finish: () => void }[] = [];
  const read = vi.fn(
    (page: readonly string[]) =>
      new Promise<ReadonlyMap<string, string>>((resolve) => {
        opened.push({
          page,
          finish: () => resolve(new Map(page.map((id) => [id, `Name ${id}`]))),
        });
      }),
  );
  return { opened, read };
}

it("asks no further page and answers no caller once the session ends", async () => {
  const reader = heldReader();
  const read = idBatcher(1, reader.read);
  const settled = Promise.allSettled(["a", "b", "c"].map(read));
  await vi.waitFor(() => expect(reader.opened).toHaveLength(2));
  endIdBatches();
  reader.opened[0]?.finish();
  reader.opened[1]?.finish();
  const results = await settled;
  expect(results.map((result) => result.status)).toEqual([
    "rejected",
    "rejected",
    "rejected",
  ]);
  expect(results[0]).toMatchObject({ reason: { name: "AbortError" } });
  expect(reader.read.mock.calls).toEqual([[["a"]], [["b"]]]);
  // The next session's batch reads as usual.
  const next = read("d");
  await vi.waitFor(() => expect(reader.opened).toHaveLength(3));
  reader.opened[2]?.finish();
  expect(await next).toBe("Name d");
});

it("merges pages in page order whichever page answers first", async () => {
  const opened: {
    page: readonly string[];
    resolve: (answer: ReadonlyMap<string, string>) => void;
  }[] = [];
  const read = idBatcher(
    1,
    (page) =>
      new Promise<ReadonlyMap<string, string>>((resolve) => {
        opened.push({ page, resolve });
      }),
  );
  const names = Promise.all(["first", "second"].map(read));
  await vi.waitFor(() => expect(opened).toHaveLength(2));
  // Both pages name "first"; the later page answers first, and still wins.
  opened[1]?.resolve(
    new Map([
      ["first", "From page two"],
      ["second", "Two"],
    ]),
  );
  opened[0]?.resolve(new Map([["first", "From page one"]]));
  expect(await names).toEqual(["From page two", "Two"]);
});

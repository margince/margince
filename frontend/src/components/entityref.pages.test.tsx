// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  cancellationClient,
  type HeldRead,
  holdReads,
} from "../testing/cancellation";
import { fetchEntityReference, useEntityName } from "./entityref.queries";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

function idsOf(read: HeldRead | undefined) {
  return new URL(read?.request.url ?? "https://test.local").searchParams.getAll(
    "id",
  );
}

function answerAll(read: HeldRead | undefined) {
  read?.answer({
    data: idsOf(read).map((id) => ({
      id,
      name: `Name ${id}`,
      archived: false,
    })),
  });
}

function heldReferences() {
  return holdReads((url) => url.pathname === "/v1/records/references");
}

it("opens two pages of a 201-id batch and starts the third when one answers", async () => {
  const held = heldReferences();
  const asked = Array.from({ length: 201 }, (_, at) => `id-${at}`);
  const names = Promise.all(
    asked.map((id) => fetchEntityReference("contact", id)),
  );
  await waitFor(() => expect(held).toHaveLength(2));
  expect(held.map((read) => idsOf(read).length)).toEqual([100, 100]);
  answerAll(held[1]);
  await waitFor(() => expect(held).toHaveLength(3));
  expect(idsOf(held[2])).toEqual(["id-200"]);
  answerAll(held[0]);
  answerAll(held[2]);
  expect(await names).toEqual(
    asked.map((id) => ({ name: `Name ${id}`, archived: false })),
  );
  expect(held).toHaveLength(3);
});

it("rejects a caller aborted mid-batch and keeps its page for the siblings", async () => {
  const held = heldReferences();
  const controller = new AbortController();
  const settled = Promise.allSettled(
    Array.from({ length: 201 }, (_, at) =>
      fetchEntityReference(
        "contact",
        `id-${at}`,
        at === 200 ? controller.signal : undefined,
      ),
    ),
  );
  await waitFor(() => expect(held).toHaveLength(2));
  controller.abort();
  answerAll(held[0]);
  await waitFor(() => expect(held).toHaveLength(3));
  answerAll(held[1]);
  answerAll(held[2]);
  const results = await settled;
  expect(results[200]).toMatchObject({
    status: "rejected",
    reason: { name: "AbortError" },
  });
  expect(
    results.slice(0, 200).every((result) => result.status === "fulfilled"),
  ).toBe(true);
  expect(held.every((read) => !read.request.signal.aborted)).toBe(true);
});

it("names a new reference while a large older batch still holds both its pages", async () => {
  const held = heldReferences();
  const { client, wrapper } = cancellationClient();
  const old = Array.from({ length: 501 }, (_, at) => `old-${at}`);
  const board = renderHook(() => old.map((id) => useEntityName("deal", id)), {
    wrapper,
  });
  await waitFor(() => expect(held).toHaveLength(2));
  board.unmount();
  const record = renderHook(() => useEntityName("deal", "new"), { wrapper });
  await waitFor(() => expect(held).toHaveLength(3));
  expect(idsOf(held[2])).toEqual(["new"]);
  await act(async () => {
    answerAll(held[2]);
  });
  await waitFor(() => expect(record.result.current.name).toBe("Name new"));
  // The older batch still finishes: its six pages arrive two at a time.
  const answered = new Set(held.slice(2, 3));
  for (const opened of [5, 7, 7]) {
    await act(async () => {
      for (const read of held.filter((read) => !answered.has(read))) {
        answered.add(read);
        answerAll(read);
      }
    });
    await waitFor(() => expect(held).toHaveLength(opened));
  }
  expect(held).toHaveLength(7);
  expect(held.every((read) => idsOf(read).length <= 100)).toBe(true);
  expect(record.result.current.name).toBe("Name new");
  client.clear();
});

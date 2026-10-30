// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useEntityName } from "./entityref.queries";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function asked(input: RequestInfo | URL): URL {
  return new URL((input instanceof Request ? input : new Request(input)).url);
}

// Names every company asked for except the ones the reader may not see, which
// the list leaves out as row scope does.
function companiesExcept(hidden: ReadonlySet<string> = new Set()) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const ids = asked(input).searchParams.getAll("id");
    return jsonResponse({
      data: ids
        .filter((id) => !hidden.has(id))
        .map((id) => ({ id, display_name: `Company ${id}` })),
      next_cursor: null,
    });
  });
}

let client: QueryClient;

beforeEach(() => {
  client = new QueryClient();
});

afterEach(() => {
  client.clear();
  vi.restoreAllMocks();
});

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client }, children);
}

function nameAll(ids: readonly string[]) {
  return renderHook(() => ids.map((id) => useEntityName("company", id)), {
    wrapper,
  });
}

describe("company names", () => {
  it("names every company rendered in one tick with one list read", async () => {
    const fetchMock = companiesExcept();
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);
    const ids = Array.from({ length: 40 }, (_, at) => `c-${at}`);

    const { result } = nameAll([...ids, ...ids]);

    await waitFor(() => expect(result.current.every((n) => n.name)).toBe(true));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [first] = fetchMock.mock.calls;
    if (!first) throw new Error("no company read was sent");
    const url = asked(first[0]);
    expect(url.pathname).toBe("/v1/companies");
    expect(url.searchParams.getAll("id")).toHaveLength(40);
    expect(url.searchParams.get("include_archived")).toBe("true");
    expect(url.searchParams.get("include_anchor")).toBe("true");
    expect(result.current[79]?.name).toBe("Company c-39");
  });

  it("splits more companies than the list's bound into reads of 100", async () => {
    const fetchMock = companiesExcept();
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const { result } = nameAll(
      Array.from({ length: 250 }, (_, at) => `c-${at}`),
    );

    await waitFor(() => expect(result.current.every((n) => n.name)).toBe(true));
    expect(
      fetchMock.mock.calls.map(
        ([input]) => asked(input).searchParams.getAll("id").length,
      ),
    ).toEqual([100, 100, 50]);
  });

  it("asks nothing again for names already read", async () => {
    const fetchMock = companiesExcept();
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);
    const first = nameAll(["c-1", "c-2"]);
    await waitFor(() =>
      expect(first.result.current.every((n) => n.name)).toBe(true),
    );

    const again = nameAll(["c-2", "c-1"]);

    expect(again.result.current.map((n) => n.name)).toEqual([
      "Company c-2",
      "Company c-1",
    ]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("reads a company the reader may not see as unnamed, and names the rest", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(
      companiesExcept(new Set(["c-hidden"])),
    );

    const { result } = nameAll(["c-open", "c-hidden"]);

    await waitFor(() => expect(result.current[1]?.reading).toBe("unnamed"));
    expect(result.current[0]?.name).toBe("Company c-open");
    expect(result.current[1]?.name).toBeNull();
  });

  it("holds a refused read as failed, never as unnamed", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({ title: "Forbidden", status: 403 }, 403),
    );
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const { result } = nameAll(["c-1", "c-2"]);

    await waitFor(() =>
      expect(result.current.every((n) => n.reading === "failed")).toBe(true),
    );
    expect(result.current.map((n) => n.name)).toEqual([null, null]);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("keeps the names an earlier read returned when a later one never arrives", async () => {
    const answered = companiesExcept();
    let reads = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      reads += 1;
      if (reads === 2) throw new TypeError("network down");
      return answered(input);
    });

    const { result } = nameAll(
      Array.from({ length: 150 }, (_, at) => `c-${at}`),
    );

    await waitFor(() => expect(result.current[149]?.reading).toBe("failed"));
    expect(result.current[0]?.name).toBe("Company c-0");
    expect(result.current[99]?.name).toBe("Company c-99");
    expect(result.current[100]?.name).toBeNull();
  });
});

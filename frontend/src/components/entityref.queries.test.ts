// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useCompanyMarksByID } from "./companymarks";
import { companyReferenceQuery, useEntityName } from "./entityref.queries";

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
// the scoped reference read omits.
function companiesExcept(hidden: ReadonlySet<string> = new Set()) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const ids = asked(input).searchParams.getAll("id");
    return jsonResponse({
      data: ids
        .filter((id) => !hidden.has(id))
        .map((id) => ({ id, name: `Company ${id}`, archived: false })),
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
  it("names every company rendered in one tick with one reference read", async () => {
    const fetchMock = companiesExcept();
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);
    const ids = Array.from({ length: 40 }, (_, at) => `c-${at}`);

    const { result } = nameAll([...ids, ...ids]);

    await waitFor(() => expect(result.current.every((n) => n.name)).toBe(true));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [first] = fetchMock.mock.calls;
    if (!first) throw new Error("no company read was sent");
    const url = asked(first[0]);
    expect(url.pathname).toBe("/v1/records/references");
    expect(url.searchParams.getAll("id")).toHaveLength(40);
    expect(url.searchParams.get("kind")).toBe("company");
    expect(
      [...url.searchParams.keys()].every(
        (key) => key === "kind" || key === "id",
      ),
    ).toBe(true);
    expect(result.current[79]?.name).toBe("Company c-39");
  });

  it("splits more companies than the reference bound into reads of 100", async () => {
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

describe("company reference cache", () => {
  it("reads only new ids when marks overlap an earlier name batch", async () => {
    const fetchMock = companiesExcept();
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);
    const names = nameAll(["c-1", "c-2"]);
    await waitFor(() =>
      expect(names.result.current.every((name) => name.name)).toBe(true),
    );

    const marks = renderHook(() => useCompanyMarksByID(["c-2", "c-3"]), {
      wrapper,
    });
    await waitFor(() => expect(marks.result.current.marks.size).toBe(2));

    expect(
      fetchMock.mock.calls.map(([input]) =>
        asked(input).searchParams.getAll("id"),
      ),
    ).toEqual([["c-1", "c-2"], ["c-3"]]);
    const lastName = nameAll(["c-3"]);
    expect(lastName.result.current[0]?.name).toBe("Company c-3");
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("keeps legacy string entries separate from rich company references", async () => {
    client.setQueryData(["company", "ref", "c-1"], "Earlier name");
    const fetchMock = companiesExcept();
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const names = nameAll(["c-1"]);
    await waitFor(() =>
      expect(names.result.current[0]?.name).toBe("Company c-1"),
    );

    expect(client.getQueryData(companyReferenceQuery("c-1").queryKey)).toEqual({
      name: "Company c-1",
      archived: false,
      logo: undefined,
    });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("shares a cached name and logo between marks and name observers", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({
        data: [
          { id: "c-1", name: "Company", archived: false, logo: "/logo.svg" },
        ],
      }),
    );
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);
    const marks = renderHook(() => useCompanyMarksByID(["c-1", "c-1"]), {
      wrapper,
    });
    await waitFor(() =>
      expect(marks.result.current.marks.get("c-1")?.logoUrl).toBe("/logo.svg"),
    );
    const name = nameAll(["c-1"]);
    expect(name.result.current[0]?.name).toBe("Company");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("refreshes names and marks together after invalidation", async () => {
    let name = "Before";
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async () =>
        jsonResponse({ data: [{ id: "c-1", name, archived: false }] }),
      );
    const names = nameAll(["c-1"]);
    const marks = renderHook(() => useCompanyMarksByID(["c-1"]), { wrapper });
    await waitFor(() => expect(names.result.current[0]?.name).toBe("Before"));
    name = "After";
    await client.invalidateQueries({
      predicate: (query) => query.queryKey[1] === "ref",
    });
    await waitFor(() => {
      expect(names.result.current[0]?.name).toBe("After");
      expect(marks.result.current.marks.get("c-1")?.name).toBe("After");
    });
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("settles missing marks separately from failed and pending reads", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(
      companiesExcept(new Set(["hidden"])),
    );
    const { result } = renderHook(
      () => useCompanyMarksByID(["visible", "hidden"]),
      { wrapper },
    );
    expect(result.current.pending.has("hidden")).toBe(true);
    await waitFor(() => expect(result.current.pending.size).toBe(0));
    expect(result.current.marks.has("visible")).toBe(true);
    expect(result.current.marks.has("hidden")).toBe(false);
    expect(result.current.unreadable.size).toBe(0);
  });

  it("retains batched answers for unmounted company observers and their siblings", async () => {
    let release!: (response: Response) => void;
    const fetchMock = vi.fn(
      () =>
        new Promise<Response>((resolve) => {
          release = resolve;
        }),
    );
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);
    const first = nameAll(["c-1"]);
    const second = nameAll(["c-2"]);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
    first.unmount();
    release(
      jsonResponse({
        data: [
          { id: "c-1", name: "First", archived: false },
          { id: "c-2", name: "Second", archived: false },
        ],
      }),
    );
    await waitFor(() => expect(second.result.current[0]?.name).toBe("Second"));
    expect(
      client.getQueryData(companyReferenceQuery("c-1").queryKey),
    ).toMatchObject({ name: "First" });
    const remounted = nameAll(["c-1"]);
    expect(remounted.result.current[0]?.name).toBe("First");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});

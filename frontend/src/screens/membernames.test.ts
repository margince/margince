// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useMemberName, useMemberNames } from "./membernames";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Names back whatever the request asked for, so a test that counts requests
// does not also carry a fixture of who exists.
function namesWhoeverIsAsked() {
  return vi.fn(async (input: RequestInfo | URL) => {
    // openapi-fetch always hands the mock a Request, but `fetch` itself also
    // takes a bare string or URL, and `mockImplementation` checks the
    // replacement against that whole signature.
    const request = input instanceof Request ? input : new Request(input);
    const asked = new URL(request.url).searchParams.getAll("id");
    return jsonResponse({
      data: asked.map((id) => ({ id, display_name: `Name ${id}` })),
    });
  });
}

// One client per test, not per render: a second `renderHook` in the same test
// has to see what the first one cached, or a test claiming a shared cache
// entry would be reading an empty one and proving nothing.
let client: QueryClient;

beforeEach(() => {
  // No `retry` default here: the failed-read tests below exist to prove the
  // module's own `retry: false` (memberNameQueryOptions), not to stand in for
  // it with the client's.
  client = new QueryClient();
});

function wrapper({ children }: { children: ReactNode }) {
  return createElement(QueryClientProvider, { client }, children);
}

afterEach(() => {
  client.clear();
  vi.restoreAllMocks();
});

describe("useMemberName", () => {
  it("asks once for one id however many components name it", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({ data: [{ id: "u-1", display_name: "Ada Lovelace" }] }),
    );
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const { result } = renderHook(
      () => [useMemberName("u-1"), useMemberName("u-1"), useMemberName("u-1")],
      { wrapper },
    );

    await waitFor(() => expect(result.current[0].data).toBe("Ada Lovelace"));
    expect(result.current[2].data).toBe("Ada Lovelace");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("asks once for the distinct ids named in one tick", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({
        data: [
          { id: "u-1", display_name: "Ada Lovelace" },
          { id: "u-2", display_name: "Grace Hopper" },
        ],
      }),
    );
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const { result } = renderHook(
      () => [useMemberName("u-1"), useMemberName("u-2")],
      { wrapper },
    );

    await waitFor(() => expect(result.current[1].data).toBe("Grace Hopper"));
    expect(result.current[0].data).toBe("Ada Lovelace");
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("splits a window wider than the contract's bound into two requests", async () => {
    const ids = Array.from({ length: 150 }, (_, at) => `u-${at}`);
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      const asked = new URL(request.url).searchParams.getAll("id");
      expect(asked.length).toBeLessThanOrEqual(100);
      return jsonResponse({
        data: asked.map((id) => ({ id, display_name: `Name ${id}` })),
      });
    });
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const { result } = renderHook(() => ids.map((id) => useMemberName(id)), {
      wrapper,
    });

    await waitFor(() => expect(result.current[149].data).toBe("Name u-149"));
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it("keeps the names one page read when a later page is refused", async () => {
    const ids = Array.from({ length: 150 }, (_, at) => `u-${at}`);
    let pages = 0;
    vi.spyOn(globalThis, "fetch").mockImplementation(
      async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : new Request(input);
        const asked = new URL(request.url).searchParams.getAll("id");
        pages += 1;
        return pages === 1
          ? jsonResponse({
              data: asked.map((id) => ({ id, display_name: `Name ${id}` })),
            })
          : jsonResponse({ title: "Server error" }, 500);
      },
    );

    const { result } = renderHook(() => ids.map((id) => useMemberName(id)), {
      wrapper,
    });

    await waitFor(() => expect(result.current[0].data).toBe("Name u-0"));
    // The refusal is about the ids on the page it refused. Carrying it to the
    // rest would report colleagues the server named as ones it could not read.
    expect(result.current[149].isError).toBe(true);
    expect(result.current[149].data).toBeUndefined();
  });

  it("holds a failed read as an error on every reference it covered", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({ title: "Server error" }, 500),
    );
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const { result } = renderHook(
      () => [useMemberName("u-1"), useMemberName("u-2")],
      { wrapper },
    );

    await waitFor(() => expect(result.current[0].isError).toBe(true));
    expect(result.current[1].isError).toBe(true);
    expect(result.current[0].data).toBeUndefined();
    // One request, not one per retry: a batch that fails must not reopen
    // itself into up to N single-id requests.
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("reads an id the answer omits as a settled absence, not an error", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
      jsonResponse({ data: [] }),
    );

    const { result } = renderHook(() => useMemberName("u-gone"), { wrapper });

    await waitFor(() => expect(result.current.isPending).toBe(false));
    expect(result.current.data).toBeNull();
    expect(result.current.isError).toBe(false);
  });

  it("asks nothing for a null id, and still names the real one beside it", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      const asked = new URL(request.url).searchParams.getAll("id");
      expect(asked).toEqual(["u-1"]);
      return jsonResponse({
        data: [{ id: "u-1", display_name: "Ada Lovelace" }],
      });
    });
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const { result } = renderHook(
      () => [useMemberName(null), useMemberName("u-1")],
      { wrapper },
    );

    await waitFor(() => expect(result.current[1].data).toBe("Ada Lovelace"));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("opens a new window once the previous one has closed", async () => {
    const fetchMock = namesWhoeverIsAsked();
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const first = renderHook(() => useMemberName("u-1"), { wrapper });
    await waitFor(() => expect(first.result.current.data).toBe("Name u-1"));

    const second = renderHook(() => useMemberName("u-2"), { wrapper });
    await waitFor(() => expect(second.result.current.data).toBe("Name u-2"));

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

describe("useMemberNames", () => {
  it("names every id it is given", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
      jsonResponse({
        data: [
          { id: "u-1", display_name: "Ada Lovelace" },
          { id: "u-2", display_name: "Grace Hopper" },
        ],
      }),
    );

    const { result } = renderHook(() => useMemberNames(["u-1", "u-2"]), {
      wrapper,
    });

    await waitFor(() => expect(result.current.get("u-2")).toBe("Grace Hopper"));
    expect(result.current.get("u-1")).toBe("Ada Lovelace");
  });

  it("names nothing for a failed read", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({ title: "Server error" }, 500),
    );
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    // `single` gives this test a settled state to wait on; `bulk`'s own
    // reads carry none, having no shape left for pending vs. failed.
    const { result } = renderHook(
      () => ({
        bulk: useMemberNames(["u-1", "u-2"]),
        single: useMemberName("u-1"),
      }),
      { wrapper },
    );

    await waitFor(() => expect(result.current.single.isError).toBe(true));
    expect(result.current.bulk.size).toBe(0);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("joins useMemberName's batch window rather than opening one of its own", async () => {
    const fetchMock = vi.fn(async () =>
      jsonResponse({
        data: [
          { id: "u-1", display_name: "Ada Lovelace" },
          { id: "u-2", display_name: "Grace Hopper" },
        ],
      }),
    );
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const { result } = renderHook(
      () => ({
        bulk: useMemberNames(["u-1", "u-2"]),
        single: useMemberName("u-2"),
      }),
      { wrapper },
    );

    await waitFor(() =>
      expect(result.current.single.data).toBe("Grace Hopper"),
    );
    expect(result.current.bulk.get("u-2")).toBe("Grace Hopper");
    // One request for the whole tick, which is the window and not the key:
    // both hooks reach the same module-level batch, so neither opens a loader
    // of its own. The cross-tick case below is what the shared key decides.
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("costs nothing for an id useMemberName later names on its own", async () => {
    const fetchMock = namesWhoeverIsAsked();
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const bulk = renderHook(() => useMemberNames(["u-1", "u-2"]), { wrapper });
    await waitFor(() =>
      expect(bulk.result.current.get("u-2")).toBe("Name u-2"),
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);

    // A later tick, past that batch window: only a cache entry under the same
    // per-id key can answer this without a second request. Diverging keys
    // would read as a miss and ask again.
    const single = renderHook(() => useMemberName("u-2"), { wrapper });
    await waitFor(() => expect(single.result.current.data).toBe("Name u-2"));
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it("asks nothing for an empty list", () => {
    const fetchMock = vi.fn();
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const { result } = renderHook(() => useMemberNames([]), { wrapper });

    expect(result.current.size).toBe(0);
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

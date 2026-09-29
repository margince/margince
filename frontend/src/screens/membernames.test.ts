// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useMemberName } from "./membernames";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return createElement(QueryClientProvider, { client }, children);
}

afterEach(() => {
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
      // openapi-fetch always hands the mock a Request, but `fetch` itself
      // also takes a bare string or URL, and `mockImplementation` checks the
      // replacement against that whole signature.
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

  it("holds a failed read as an error on every reference it covered", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async () =>
      jsonResponse({ title: "Server error" }, 500),
    );

    const { result } = renderHook(
      () => [useMemberName("u-1"), useMemberName("u-2")],
      { wrapper },
    );

    await waitFor(() => expect(result.current[0].isError).toBe(true));
    expect(result.current[1].isError).toBe(true);
    expect(result.current[0].data).toBeUndefined();
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
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : new Request(input);
      const asked = new URL(request.url).searchParams.getAll("id");
      return jsonResponse({
        data: asked.map((id) => ({ id, display_name: `Name ${id}` })),
      });
    });
    vi.spyOn(globalThis, "fetch").mockImplementation(fetchMock);

    const first = renderHook(() => useMemberName("u-1"), { wrapper });
    await waitFor(() => expect(first.result.current.data).toBe("Name u-1"));

    const second = renderHook(() => useMemberName("u-2"), { wrapper });
    await waitFor(() => expect(second.result.current.data).toBe("Name u-2"));

    expect(fetchMock).toHaveBeenCalledTimes(2);
  });
});

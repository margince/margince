// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { locationDouble } from "../testing/locationdouble";
import {
  type ConnectAccountInput,
  notConfiguredProvider,
  useConnectAccount,
} from "./connect-account";
import { problemCodeOf } from "./problem";

// Every connect surface presses through this one hook, so what it sends is
// what Connections, Meetings and onboarding send. These cases pin the body the
// server reads from, the hand-off to the vendor, and the unwired provider.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubConnect(answer: () => Response) {
  const requests: Request[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      requests.push(request);
      return answer();
    }),
  );
  return requests;
}

function wrapper({ children }: Readonly<{ children: ReactNode }>) {
  return (
    <QueryClientProvider client={new QueryClient()}>
      {children}
    </QueryClientProvider>
  );
}

async function press(
  input: ConnectAccountInput,
  onRedirect?: (input: ConnectAccountInput) => void,
) {
  const { result } = renderHook(() => useConnectAccount({ onRedirect }), {
    wrapper,
  });
  await act(async () => {
    await result.current.mutateAsync(input).catch(() => undefined);
  });
  // The observer re-renders after the promise settles, so wait for that.
  await waitFor(() =>
    expect(result.current.isSuccess || result.current.isError).toBe(true),
  );
  return result;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("useConnectAccount", () => {
  it("posts only the return surface when nothing else is asked for", async () => {
    vi.stubGlobal("location", locationDouble({ assign: vi.fn() }));
    const requests = stubConnect(() =>
      jsonResponse({ authorize_url: "https://accounts.google/consent" }),
    );
    await press({ provider: "gcal", returnTo: "meetings" });
    expect(new URL(requests[0].url).pathname).toMatch(
      /\/connectors\/gcal\/connect$/,
    );
    expect(await requests[0].json()).toEqual({ return_to: "meetings" });
  });

  it("carries the paired calendar and the import window on the signed body", async () => {
    vi.stubGlobal("location", locationDouble({ assign: vi.fn() }));
    const requests = stubConnect(() =>
      jsonResponse({ authorize_url: "https://accounts.google/consent" }),
    );
    await press({
      provider: "gmail",
      returnTo: "settings",
      alsoConnect: "gcal",
      importWindow: "6m",
    });
    expect(await requests[0].json()).toEqual({
      return_to: "settings",
      also_connect: "gcal",
      import_window: "6m",
    });
  });

  it("hands the page to the vendor, telling the caller first", async () => {
    const order: string[] = [];
    const assign = vi.fn(() => order.push("assign"));
    vi.stubGlobal("location", locationDouble({ assign }));
    stubConnect(() =>
      jsonResponse({ authorize_url: "https://login.microsoftonline/x" }),
    );
    const onRedirect = vi.fn(() => order.push("onRedirect"));
    const result = await press(
      { provider: "graph", returnTo: "onboarding" },
      onRedirect,
    );
    expect(assign).toHaveBeenCalledWith("https://login.microsoftonline/x");
    expect(onRedirect).toHaveBeenCalledWith({
      provider: "graph",
      returnTo: "onboarding",
    });
    expect(order).toEqual(["onRedirect", "assign"]);
    expect(result.current.data).toEqual({ kind: "redirecting" });
  });

  it("reads an unwired provider as a result naming it, and stays on the page", async () => {
    const assign = vi.fn();
    vi.stubGlobal("location", locationDouble({ assign }));
    stubConnect(() =>
      jsonResponse({ code: "not_implemented", detail: "not wired" }, 501),
    );
    const result = await press({ provider: "graphcal", returnTo: "settings" });
    expect(result.current.isSuccess).toBe(true);
    expect(notConfiguredProvider(result.current.data)).toBe("graphcal");
    expect(assign).not.toHaveBeenCalled();
  });

  it("fails on any other refusal, carrying the server's problem", async () => {
    const assign = vi.fn();
    vi.stubGlobal("location", locationDouble({ assign }));
    stubConnect(() =>
      jsonResponse(
        { code: "also_connect_mismatch", detail: "gcal pairs with gmail" },
        422,
      ),
    );
    const result = await press({
      provider: "graph",
      returnTo: "settings",
      alsoConnect: "gcal",
    });
    expect(problemCodeOf(result.current.error)).toBe("also_connect_mismatch");
    expect(notConfiguredProvider(result.current.data)).toBeNull();
    expect(assign).not.toHaveBeenCalled();
  });
});

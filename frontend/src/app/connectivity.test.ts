/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// Two outages told apart: the device's own report, and a server that stopped
// answering on a working network. The probe's backoff runs on fake timers.

const shadowed: string[] = [];
const listening: (() => void)[] = [];

function shadow(target: object, name: string, value: unknown): void {
  Object.defineProperty(target, name, { configurable: true, value });
  shadowed.push(name);
}

afterEach(() => {
  // A store left listening would answer the next case's window events.
  for (const stop of listening.splice(0)) stop();
  for (const name of shadowed.splice(0)) {
    Reflect.deleteProperty(navigator, name);
    Reflect.deleteProperty(document, name);
  }
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function goOffline(): void {
  shadow(navigator, "onLine", false);
  window.dispatchEvent(new Event("offline"));
}

function goOnline(): void {
  shadow(navigator, "onLine", true);
  window.dispatchEvent(new Event("online"));
}

function hideTab(hidden: boolean): void {
  shadow(document, "visibilityState", hidden ? "hidden" : "visible");
  document.dispatchEvent(new Event("visibilitychange"));
}

const json = (status: number) =>
  new Response("{}", {
    status,
    headers: { "Content-Type": "application/json" },
  });

/** The api and the health probe, each down or up: `/healthz` answers with the
 *  next status queued (503 once the queue is empty), or never while `stall`. */
function network() {
  const state = { api: true, health: [] as number[], stall: false };
  const probes: number[] = [];
  const fetch = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url !== "/healthz") {
      if (!state.api) throw new TypeError("Failed to fetch");
      return json(200);
    }
    probes.push(Date.now());
    if (state.stall) {
      return new Promise<Response>((_resolve, reject) => {
        init?.signal?.addEventListener("abort", () =>
          reject(new DOMException("aborted", "AbortError")),
        );
      });
    }
    return new Response("ok", { status: state.health.shift() ?? 503 });
  });
  vi.stubGlobal("fetch", fetch);
  return { state, probes };
}

/** A fresh store per case: it lives as long as the page does. */
async function page() {
  vi.resetModules();
  const connectivity = await import("./connectivity");
  const client = await import("../api/client");
  return { ...connectivity, ...client };
}

/** A fresh store with a banner watching it, as the shell has. */
async function watched() {
  const store = await page();
  const heard = vi.fn();
  listening.push(store.watchConnectivity(heard));
  return { ...store, heard };
}

describe("the device's own report", () => {
  it("follows the browser's offline and online events", async () => {
    const { useConnectivity } = await page();
    const hook = renderHook(() => useConnectivity());
    expect(hook.result.current).toBe("online");

    act(goOffline);
    expect(hook.result.current).toBe("offline");

    act(goOnline);
    expect(hook.result.current).toBe("online");
  });
});

describe("what a request through the api client proves", () => {
  it("asks /healthz about a refused request, and names Margince only when it fails too", async () => {
    vi.useFakeTimers({ now: 0 });
    const { api, connectivityNow, ConnectivityError, heard } = await watched();
    const net = network();
    net.state.api = false;

    const failure = await api.GET("/me").catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(ConnectivityError);
    expect(failure).toMatchObject({
      outage: "unreachable",
      method: "GET",
      unsent: false,
    });
    expect(connectivityNow()).toBe("online");

    await vi.advanceTimersByTimeAsync(0);
    expect(net.probes).toEqual([0]);
    expect(connectivityNow()).toBe("unreachable");
    expect(heard).toHaveBeenCalledTimes(1);

    // A 5xx is still an answer: the server is there to refuse.
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => json(500)),
    );
    await api.GET("/me");
    expect(connectivityNow()).toBe("online");
  });

  // A blocked URL or a firewall's 503 on one route: /healthz answers, so the
  // failure stays that request's own and nothing pauses.
  it("declares nothing when the probe it sends is answered", async () => {
    vi.useFakeTimers({ now: 0 });
    const { api, connectivityNow, heard } = await watched();
    const net = network();
    net.state.api = false;
    net.state.health.push(200);

    await api.GET("/me").catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(20_000);
    expect(net.probes).toEqual([0]);
    expect(connectivityNow()).toBe("online");
    expect(heard).not.toHaveBeenCalled();
  });

  it("knows a request an offline device sent was never sent at all", async () => {
    const { api, connectivityNow } = await watched();
    network().state.api = false;
    goOffline();

    const failure = await api
      .POST("/auth/logout")
      .catch((error: unknown) => error);
    expect(failure).toMatchObject({
      outage: "offline",
      method: "POST",
      unsent: true,
    });

    goOnline();
    expect(connectivityNow()).toBe("online");
  });

  // nginx, the Vite proxy and the desktop launcher all answer a bare 502 when
  // the api behind them is down; the api's own 5xx always carries a problem.
  it("takes a proxy's bare gateway answer for Margince gone, and the api's own 5xx for an answer", async () => {
    vi.useFakeTimers({ now: 0 });
    const { api, connectivityNow } = await watched();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response("<html>502 Bad Gateway</html>", {
            status: 502,
            headers: { "Content-Type": "text/html" },
          }),
      ),
    );

    // The request's own failure stays a plain refusal: the api may have
    // acted before the proxy gave up, so nothing claims it was not saved.
    const refused = await api.POST("/auth/logout");
    expect(refused.response.status).toBe(502);
    await vi.advanceTimersByTimeAsync(0);
    expect(connectivityNow()).toBe("unreachable");

    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ status: 503, code: "service_unavailable" }),
            {
              status: 503,
              headers: { "Content-Type": "application/problem+json" },
            },
          ),
      ),
    );
    await api.GET("/me");
    expect(connectivityNow()).toBe("online");
  });

  it("reads a proxy that stopped waiting on a model as a slow model, not an outage", async () => {
    vi.useFakeTimers({ now: 0 });
    const { api, connectivityNow } = await watched();
    const gateway = vi.fn(async () => new Response(null, { status: 504 }));
    vi.stubGlobal("fetch", gateway);

    await api.POST("/contacts/{id}/draft-email", {
      params: { path: { id: "p-1" } },
      body: {},
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(gateway).toHaveBeenCalledTimes(1);
    expect(connectivityNow()).toBe("online");
  });

  it("learns nothing from a request its caller abandoned", async () => {
    vi.useFakeTimers({ now: 0 });
    const { api, connectivityNow, ConnectivityError } = await watched();
    const net = network();
    net.state.api = false;
    const abandoned = new AbortController();
    abandoned.abort();

    const failure = await api
      .GET("/me", { signal: abandoned.signal })
      .catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(0);
    expect(failure).not.toBeInstanceOf(ConnectivityError);
    expect(net.probes).toEqual([]);
    expect(connectivityNow()).toBe("online");
  });

  it("counts a request that outlived its deadline, unless a model was working on it", async () => {
    vi.useFakeTimers();
    const store = await watched();
    const silent = vi.fn(
      (_request: Request, init?: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          init?.signal?.addEventListener("abort", () =>
            reject(init.signal?.reason),
          );
        }),
    );
    vi.stubGlobal("fetch", silent);

    const draft = store.api
      .POST("/contacts/{id}/draft-email", {
        params: { path: { id: "p-1" } },
        body: {},
      })
      .catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(store.MODEL_ROUTE_TIMEOUT_MS);
    expect(await draft).toBeInstanceOf(store.RequestTimeoutError);
    expect(silent).toHaveBeenCalledTimes(1);

    // The stall keeps its own type: a write that timed out may have landed.
    const read = store.api.GET("/me").catch((error: unknown) => error);
    await vi.advanceTimersByTimeAsync(store.REQUEST_TIMEOUT_MS);
    expect(await read).toBeInstanceOf(store.RequestTimeoutError);
    // The probe it sent stalls as well, and gives up past its own deadline.
    await vi.advanceTimersByTimeAsync(11_000);
    expect(store.connectivityNow()).toBe("unreachable");
  });
});

describe("the probe while Margince is unreachable", () => {
  it("backs off from two seconds to a thirty-second ceiling, and a 2xx clears it", async () => {
    vi.useFakeTimers({ now: 0 });
    const { reportUnreached, connectivityNow, heard } = await watched();
    const net = network();

    reportUnreached();
    for (const wait of [0, 2_000, 4_000, 8_000, 16_000, 30_000, 30_000]) {
      await vi.advanceTimersByTimeAsync(wait);
    }
    expect(net.probes).toEqual([
      0, 2_000, 6_000, 14_000, 30_000, 60_000, 90_000,
    ]);
    expect(connectivityNow()).toBe("unreachable");
    expect(heard).toHaveBeenCalledTimes(1);

    net.state.health.push(204);
    await vi.advanceTimersByTimeAsync(30_000);
    expect(connectivityNow()).toBe("online");
    expect(heard).toHaveBeenCalledTimes(2);

    await vi.advanceTimersByTimeAsync(120_000);
    expect(net.probes).toHaveLength(8);
  });

  it("gives up on a probe that never answers, and tries again", async () => {
    vi.useFakeTimers({ now: 0 });
    const { reportUnreached } = await watched();
    const net = network();
    net.state.stall = true;

    reportUnreached();
    await vi.advanceTimersByTimeAsync(10_000 + 2_000);
    expect(net.probes).toEqual([0, 12_000]);
  });

  it("rests while the tab is hidden and asks at once when it is shown", async () => {
    vi.useFakeTimers({ now: 0 });
    const { reportUnreached, connectivityNow } = await watched();
    const net = network();

    reportUnreached();
    await vi.advanceTimersByTimeAsync(0);
    expect(connectivityNow()).toBe("unreachable");
    hideTab(true);
    await vi.advanceTimersByTimeAsync(150_000);
    // A background request failing while hidden does not wake it either.
    reportUnreached();
    await vi.advanceTimersByTimeAsync(150_000);
    expect(net.probes).toEqual([0]);

    net.state.health.push(200);
    hideTab(false);
    await vi.advanceTimersByTimeAsync(0);
    expect(net.probes).toEqual([0, 300_000]);
    expect(connectivityNow()).toBe("online");
  });

  it("stops while the device is offline and asks at once when it is back", async () => {
    vi.useFakeTimers({ now: 0 });
    const { reportUnreached, connectivityNow } = await watched();
    const net = network();

    reportUnreached();
    await vi.advanceTimersByTimeAsync(0);
    goOffline();
    expect(connectivityNow()).toBe("offline");
    await vi.advanceTimersByTimeAsync(60_000);
    expect(net.probes).toEqual([0]);

    // The server's standing survives the device's: nothing has answered yet.
    goOnline();
    expect(connectivityNow()).toBe("unreachable");
    net.state.health.push(200);
    await vi.advanceTimersByTimeAsync(0);
    expect(net.probes).toEqual([0, 60_000]);
    expect(connectivityNow()).toBe("online");
  });

  it("lets an api answer that lands while the probe is out outrank the probe", async () => {
    vi.useFakeTimers({ now: 0 });
    const { reportUnreached, reportReached, connectivityNow } = await watched();
    const net = network();
    net.state.stall = true;

    reportUnreached();
    await vi.advanceTimersByTimeAsync(1_000);
    reportReached();
    await vi.advanceTimersByTimeAsync(9_000);
    expect(net.probes).toEqual([0]);
    expect(connectivityNow()).toBe("online");
  });

  it("holds no outage a follower alone is watching, and releases it with the last banner", async () => {
    vi.useFakeTimers({ now: 0 });
    const store = await page();
    const net = network();
    const follower = vi.fn();
    listening.push(store.subscribeConnectivity(follower));

    expect(store.reportUnreached()).toBeNull();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(net.probes).toEqual([]);
    expect(store.connectivityNow()).toBe("online");

    const stop = store.watchConnectivity(() => undefined);
    expect(store.reportUnreached()).toBe("unreachable");
    await vi.advanceTimersByTimeAsync(0);
    expect(net.probes).toEqual([60_000]);
    expect(follower).toHaveBeenCalledTimes(1);

    // The follower is released, not left paused behind a banner that is gone.
    stop();
    expect(store.connectivityNow()).toBe("online");
    expect(follower).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(60_000);
    expect(net.probes).toHaveLength(1);
  });
});

// A public page (an unsubscribe link, a booking, a buyer room) draws no banner,
// so an outage there must reach the page's own error state, not a pause.
describe("a surface no banner watches", () => {
  it("gets a refused request's own failure, and holds no outage", async () => {
    const { api, connectivityNow, ConnectivityError } = await page();
    const net = network();
    net.state.api = false;

    const failure = await api.GET("/me").catch((error: unknown) => error);
    expect(failure).toBeInstanceOf(TypeError);
    expect(failure).not.toBeInstanceOf(ConnectivityError);
    expect(connectivityNow()).toBe("online");
  });

  it("gets a proxy's bare 502 as a plain refusal, and holds no outage", async () => {
    const { api, connectivityNow } = await page();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response("Bad Gateway", { status: 502 })),
    );

    const { response } = await api.GET("/me");
    expect(response.status).toBe(502);
    expect(connectivityNow()).toBe("online");
  });
});

// The session's own connection screen states the outage in its own words, so it
// may hold one open; what it must never do is check in a loop.
describe("a screen that states the outage itself", () => {
  it("checks once per mount, never while a check is in flight", async () => {
    const { useOutageRecovery } = await page();
    let settle: (movesOn: boolean) => void = () => undefined;
    const check = vi.fn(
      () =>
        new Promise<boolean>((resolve) => {
          settle = resolve;
        }),
    );
    const recheck = vi.fn();

    const screen = renderHook(() => useOutageRecovery(true, check, recheck));
    screen.rerender();
    renderHook(() => useOutageRecovery(true, check, recheck));
    expect(check).toHaveBeenCalledTimes(1);

    // An answer the screen would only draw again gives it nothing to move to.
    await act(async () => settle(false));
    expect(recheck).not.toHaveBeenCalled();
  });

  it("reads the session again when its check finds an answer to give way to", async () => {
    const { useOutageRecovery } = await page();
    const recheck = vi.fn();

    renderHook(() => useOutageRecovery(true, async () => true, recheck));
    await act(async () => undefined);
    expect(recheck).toHaveBeenCalledTimes(1);
  });

  it("holds an outage only while it states one", async () => {
    vi.useFakeTimers({ now: 0 });
    const store = await page();
    network();
    const pending = () => new Promise<boolean>(() => undefined);

    const screen = renderHook(
      ({ active }) => store.useOutageRecovery(active, pending, () => undefined),
      { initialProps: { active: false } },
    );
    expect(store.reportUnreached()).toBeNull();

    screen.rerender({ active: true });
    expect(store.reportUnreached()).toBe("unreachable");
    await vi.advanceTimersByTimeAsync(0);
    expect(store.connectivityNow()).toBe("unreachable");

    screen.unmount();
    expect(store.connectivityNow()).toBe("online");
  });
});

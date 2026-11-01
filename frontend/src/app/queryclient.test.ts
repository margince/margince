import {
  MutationObserver,
  onlineManager,
  type QueryClient,
  QueryObserver,
} from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ENTITY_NAME_KEY } from "../components/entityref";
import {
  ConnectivityError,
  reportReached,
  reportUnreached,
  watchConnectivity,
} from "../lib/connectivity";
import { ProblemError, throwProblem } from "../lib/problem";
import {
  createQueryClient,
  liveInterval,
  liveOnMount,
  liveOnReturn,
  notePrefetched,
  retryQuery,
  staleFor,
} from "./queryclient";

// The data-layer parameters are invisible until they are wrong: a retried 4xx
// doubles a refusal the server already made final, and an unreported failure
// leaves nothing behind for whoever has to explain it. Both are pinned here.

function problem(status: number, code?: string): ProblemError {
  return new ProblemError({
    type: "about:blank",
    status,
    code,
    detail: "a server problem",
  });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("the query retry policy", () => {
  it("retries a server error, at most twice", () => {
    const unavailable = problem(503, "service_unavailable");
    expect(retryQuery(0, unavailable)).toBe(true);
    expect(retryQuery(1, unavailable)).toBe(true);
    expect(retryQuery(2, unavailable)).toBe(false);
  });

  it("never retries a refusal the server has settled, however early", () => {
    // Both are classed as server errors and neither is a failure the server
    // may recover from: 501 does not support what was asked, 505 will not
    // speak this version. A build reaches 501 deliberately — httperr answers
    // it for a surface the contract specifies and this build does not wire —
    // so a retry here is three requests and three console errors for one
    // settled answer, read first by an operator looking for a real fault.
    for (const [status, code] of [
      [501, "not_implemented"],
      [505, undefined],
    ] as const) {
      expect(retryQuery(0, problem(status, code)), String(status)).toBe(false);
    }
    // And the neighbours are still retried, so the exclusion is these two
    // rather than a range that swallowed them.
    for (const [status, code] of [
      [500, "internal"],
      [502, "provider_unusable"],
      [503, "service_unavailable"],
      [504, undefined],
    ] as const) {
      expect(retryQuery(0, problem(status, code)), String(status)).toBe(true);
    }
  });

  it("never retries a host calendar the server could not read", () => {
    expect(retryQuery(0, problem(502, "calendar_unavailable"))).toBe(false);
  });

  it("never retries a client error, however early the failure", () => {
    for (const [status, code] of [
      [400, "invalid_argument"],
      [401, "unauthorized"],
      [403, "permission_denied"],
      [404, "not_found"],
      [409, "conflict"],
      [422, "validation_error"],
      [429, "rate_limited"],
    ] as const) {
      expect(retryQuery(0, problem(status, code)), String(status)).toBe(false);
    }
  });

  it("does not retry a failure that carries no server status", () => {
    // A rejected fetch and a failure raised inside a query function: the
    // server reported neither, so FE-PARAM-2 retries neither.
    expect(retryQuery(0, new TypeError("Failed to fetch"))).toBe(false);
    expect(retryQuery(0, new Error("record not found"))).toBe(false);
  });

  it("ignores a problem body whose status is not a number", () => {
    expect(retryQuery(0, new ProblemError({ status: "503" }))).toBe(false);
    expect(retryQuery(0, new ProblemError(null))).toBe(false);
  });
});

describe("the query client defaults", () => {
  it("serves cached data for the pinned staleness window", () => {
    expect(createQueryClient().getDefaultOptions().queries?.staleTime).toBe(
      staleFor,
    );
    expect(staleFor({ queryKey: ["contact360", "p-1"] })).toBe(30_000);
    expect(staleFor({ queryKey: ["worklist", "handled"] })).toBe(30_000);
  });

  it("reads Home's day on change and once it is five minutes old, never on a cadence", () => {
    for (const queryKey of [
      ["worklist", "mine", "all", ""],
      ["magic"],
      ["brief"],
    ]) {
      expect(liveInterval({ queryKey })).toBe(false);
      expect(liveOnReturn({ queryKey })).toBe(true);
      expect(staleFor({ queryKey })).toBe(5 * 60_000);
      expect(
        liveOnMount(
          { queryKey, queryHash: "h", state: { dataUpdatedAt: 0 } },
          1,
        ),
      ).toBe(true);
    }
    // Another owner's day or a filtered one is not Home's day.
    expect(staleFor({ queryKey: ["worklist", "mine", "all", "u-2"] })).toBe(
      30_000,
    );
    // The reader's own handled list stays live: it is one cheap read.
    expect(liveInterval({ queryKey: ["worklist", "handled"] })).toBe(60_000);
  });

  // FE-PARAM-3 and FE-PARAM-5 are one predicate each rather than a flag, and
  // each is asked for both of its answers. A record read repeats; what comes
  // back on return is what a stale answer misstates. Both kinds of key are
  // asked, because a change can get one wrong and the other right.
  //
  // The predicates, not the defaults they are installed as. What they are
  // wired to is proved end to end in features/records/liverecord.test.tsx,
  // which drives each record page's own read through this client.
  it("repeats a record read, and nothing else", () => {
    expect(liveInterval({ queryKey: ["contact360", "p-1"] })).toBe(60_000);
    expect(liveInterval({ queryKey: ["contact", "p-1"] })).toBe(60_000);
    expect(liveInterval({ queryKey: ["company", "o-1"] })).toBe(60_000);
    expect(liveInterval({ queryKey: ["contact", "p-1", "employer"] })).toBe(
      false,
    );
    expect(liveInterval({ queryKey: ["me", "ai-activity"] })).toBe(false);
  });

  it("reads again on return whatever a stale answer would misstate", () => {
    // "always" and not `true`: `true` refetches only a read already stale, so
    // inside the staleness window above the return would be served the cache.
    expect(liveOnReturn({ queryKey: ["contact360", "p-1"] })).toBe("always");
    expect(liveOnReturn({ queryKey: ["me", "ai-activity"] })).toBe(false);
    // A timeline comes back with the reader. Every row it serves says whether
    // the reader may change it, and a grant expires without a signal.
    expect(liveOnReturn({ queryKey: ["activities", "contact", "c-1"] })).toBe(
      "always",
    );
    // It does not poll: it pages, so a cadence would refetch every page a
    // reader has scrolled through.
    expect(liveInterval({ queryKey: ["activities", "contact", "c-1"] })).toBe(
      false,
    );
    // Not every key whose first word is activities: a one-word prefix is a
    // write path's invalidation target, not a read.
    expect(liveOnReturn({ queryKey: ["activities"] })).toBe(false);

    // The installation read carries the policy that decides whether figures
    // already on screen may stay there, so it comes back with the reader.
    expect(liveOnReturn({ queryKey: ["installation-settings"] })).toBe(
      "always",
    );
  });

  it("re-reads a live work read on mount unless it is the prefetched answer, under a second old", () => {
    const day = { queryKey: ["worklist", "handled"], queryHash: "day" };
    const at = (dataUpdatedAt: number) => ({
      ...day,
      state: { dataUpdatedAt },
    });
    expect(liveOnMount(at(10_000), 10_100)).toBe("always");
    notePrefetched("day", 10_000);
    expect(liveOnMount(at(10_000), 10_999)).toBe(true);
    expect(liveOnMount(at(10_000), 11_000)).toBe("always");
    expect(liveOnMount(at(10_500), 10_600)).toBe("always");
    expect(
      liveOnMount(
        {
          queryKey: ["contact", "p-1"],
          queryHash: "c",
          state: { dataUpdatedAt: 0 },
        },
        5_000,
      ),
    ).toBe(true);
  });

  it("re-reads a live work read for a second view, unless it is Home's day", () => {
    const handled = {
      queryKey: ["worklist", "handled"],
      queryHash: "handled",
      state: { dataUpdatedAt: 10_000 },
    };
    expect(liveOnMount(handled, 20_000)).toBe("always");
    expect(
      liveOnMount(
        {
          ...handled,
          queryKey: ["worklist", "mine", "all", ""],
          queryHash: "d",
        },
        20_000,
      ),
    ).toBe(true);
  });

  it("fetches Home's day once for two views on screen, and serves it to a later mount", async () => {
    const client = createQueryClient();
    let fetched = 0;
    const options = {
      queryKey: ["worklist", "mine", "all", ""],
      queryFn: async () => ++fetched,
    };
    const home = new QueryObserver(client, options);
    const stopHome = home.subscribe(() => {});
    await vi.waitFor(() =>
      expect(home.getCurrentResult().fetchStatus).toBe("idle"),
    );
    expect(fetched).toBe(1);
    const drawer = new QueryObserver(client, options);
    const stopDrawer = drawer.subscribe(() => {});
    expect(drawer.getCurrentResult().fetchStatus).toBe("idle");
    expect(fetched).toBe(1);
    stopDrawer();
    stopHome();
    const back = new QueryObserver(client, options);
    const stopBack = back.subscribe(() => {});
    expect(back.getCurrentResult().fetchStatus).toBe("idle");
    expect(fetched).toBe(1);
    stopBack();
    client.clear();
  });

  it("reports a failed query once, through the global sink", async () => {
    const reported = vi.spyOn(console, "error").mockImplementation(() => {});
    const client = createQueryClient();

    await expect(
      client.fetchQuery({
        queryKey: ["boundary-test"],
        queryFn: () => Promise.reject(new Error("the query failed")),
      }),
    ).rejects.toThrow("the query failed");

    expect(reported).toHaveBeenCalledTimes(1);
    expect(reported.mock.calls[0]?.[1]).toBeInstanceOf(Error);
  });
});

// A mutation is the half of the data layer nothing observes on its own: its
// result lives on the hook instance that started it, so a failure the reader
// is shown as one generic sentence leaves no trace anywhere unless the client
// itself keeps one. These pin that the client does, for every mutation the
// application runs rather than for the ones a screen remembered to wire.
describe("the mutation failure sink", () => {
  function failWith(client: QueryClient, error: unknown): Promise<unknown> {
    return new MutationObserver(client, {
      mutationFn: () => Promise.reject(error),
    }).mutate();
  }

  it("keeps a failure nobody wrote for a reader, once and as it was thrown", async () => {
    const reported = vi.spyOn(console, "error").mockImplementation(() => {});
    const crash = new TypeError("Cannot read properties of undefined");

    await expect(failWith(createQueryClient(), crash)).rejects.toThrow(crash);

    // The value itself: a message string would drop the stack the console is
    // being read for.
    expect(reported).toHaveBeenCalledTimes(1);
    expect(reported).toHaveBeenCalledWith(crash);
  });

  it("stays silent for a failure the server already described", async () => {
    const reported = vi.spyOn(console, "error").mockImplementation(() => {});

    await expect(
      failWith(createQueryClient(), new ProblemError({ detail: "no seat" })),
    ).rejects.toThrow("no seat");

    // The reader can already read that cause; a console copy would report the
    // same failure twice and add nothing to it.
    expect(reported).not.toHaveBeenCalled();
  });
});

describe("names the chrome is showing", () => {
  function nameQuery(client: QueryClient, id: string): Promise<unknown> {
    return client.fetchQuery({
      queryKey: ["company", ENTITY_NAME_KEY, id],
      queryFn: () => Promise.resolve({ name: "Globex" }),
    });
  }

  // A rename lands as an ordinary mutation on a screen that knows nothing
  // about the trail at the top of the window. Nothing else brings those names
  // back inside their freshness window, so the trail went on naming the record
  // by what it used to be called until the reader reloaded.
  it("brings a record's name back after a write that could have changed it", async () => {
    const client = createQueryClient();
    await nameQuery(client, "o-1");
    expect(
      client.getQueryState(["company", ENTITY_NAME_KEY, "o-1"])?.isInvalidated,
    ).toBe(false);

    await new MutationObserver(client, {
      mutationFn: () => Promise.resolve("renamed"),
    }).mutate();

    expect(
      client.getQueryState(["company", ENTITY_NAME_KEY, "o-1"])?.isInvalidated,
    ).toBe(true);
  });

  // The other keys are a screen's own reads, and a screen that has just
  // written invalidates what it owns. Refetching every read in the cache after
  // every write would put the whole page back on the network.
  it("leaves a screen's own reads to the screen", async () => {
    const client = createQueryClient();
    await client.fetchQuery({
      queryKey: ["company360", "o-1"],
      queryFn: () => Promise.resolve({ id: "o-1" }),
    });

    await new MutationObserver(client, {
      mutationFn: () => Promise.resolve("written"),
    }).mutate();

    expect(client.getQueryState(["company360", "o-1"])?.isInvalidated).toBe(
      false,
    );
  });
});

describe("the history a reader is looking at", () => {
  // A record's history is a read of what has just been written to it. A write
  // made from another panel on the same page — or a restore, whose whole point
  // is to add a line to the list on screen — left the open history showing the
  // state before it until the reader navigated away and back.
  it("brings a record's history back after any successful write", async () => {
    const client = createQueryClient();
    for (const key of [
      ["record-history", "company", "o-1"],
      ["field-history", "company", "o-1", "", ""],
    ]) {
      await client.fetchQuery({
        queryKey: key,
        queryFn: () => Promise.resolve([]),
      });
      expect(client.getQueryState(key)?.isInvalidated).toBe(false);
    }

    await new MutationObserver(client, {
      mutationFn: () => Promise.resolve("written"),
    }).mutate();

    for (const key of [
      ["record-history", "company", "o-1"],
      ["field-history", "company", "o-1", "", ""],
    ]) {
      expect(client.getQueryState(key)?.isInvalidated).toBe(true);
    }
  });

  // A failed write changed nothing, so the history on screen is still correct
  // and refetching it would spend a read to redraw the same list.
  it("leaves the history alone when the write failed", async () => {
    const client = createQueryClient();
    const key = ["record-history", "company", "o-2"];
    await client.fetchQuery({
      queryKey: key,
      queryFn: () => Promise.resolve([]),
    });

    await new MutationObserver(client, {
      mutationFn: () => Promise.reject(new Error("refused")),
      retry: false,
    })
      .mutate()
      .catch(() => undefined);

    expect(client.getQueryState(key)?.isInvalidated).toBe(false);
  });
});

describe("who can see a record", () => {
  // A share granted or revoked, a record made private, an owner changed: each
  // is a different write, and the panel listing who can see the record must
  // not keep naming the colleagues it named before any of them.
  it("is read again after any successful write", async () => {
    const client = createQueryClient();
    const key = ["record-access", "contact", "c-1"];
    await client.fetchQuery({
      queryKey: key,
      queryFn: () => Promise.resolve({ data: [] }),
    });

    await new MutationObserver(client, {
      mutationFn: () => Promise.resolve("shared"),
    }).mutate();

    expect(client.getQueryState(key)?.isInvalidated).toBe(true);
  });
});

// Reads wait out an outage and run again when it clears, but only where the
// shell's banner says why; a write is attempted at once either way.
describe("while Margince cannot be reached", () => {
  const banners: (() => void)[] = [];
  const bannerUp = () => banners.push(watchConnectivity(() => undefined));

  async function readMe(): Promise<unknown> {
    const { data, error } = await api.GET("/me");
    if (error) throwProblem(error);
    return data;
  }

  // A refused request sends a probe, and only the probe failing too is an outage.
  async function outageConfirmed() {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError("Failed to fetch");
      }),
    );
    reportUnreached();
    await vi.advanceTimersByTimeAsync(0);
  }

  afterEach(() => {
    for (const bannerDown of banners.splice(0)) bannerDown();
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("tells the data layer about the server, not only about the device", async () => {
    vi.useFakeTimers();
    createQueryClient();
    bannerUp();
    await outageConfirmed();
    expect(onlineManager.isOnline()).toBe(false);
    reportReached();
    expect(onlineManager.isOnline()).toBe(true);
  });

  it("holds a read until Margince answers, then runs it", async () => {
    vi.useFakeTimers();
    const client = createQueryClient();
    client.mount();
    bannerUp();
    const read = vi.fn(async () => "fresh");
    await outageConfirmed();

    const answer = client.fetchQuery({
      queryKey: ["outage-read"],
      queryFn: read,
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(client.getQueryState(["outage-read"])?.fetchStatus).toBe("paused");
    expect(read).not.toHaveBeenCalled();

    reportReached();
    await expect(answer).resolves.toBe("fresh");
    client.unmount();
  });

  it("attempts a write at once and lets it fail, rather than holding it for later", async () => {
    vi.useFakeTimers();
    vi.spyOn(console, "error").mockImplementation(() => {});
    const client = createQueryClient();
    expect(client.getDefaultOptions().mutations?.networkMode).toBe("always");
    bannerUp();
    await outageConfirmed();
    expect(onlineManager.isOnline()).toBe(false);

    const observer = new MutationObserver(client, {
      mutationFn: () => Promise.reject(new Error("not reached")),
    });
    const attempt = observer.mutate();
    expect(observer.getCurrentResult().isPaused).toBe(false);
    await expect(attempt).rejects.toThrow("not reached");
  });

  // A public page draws no banner: a pause there is a spinner that never ends.
  it("lets reads run and fail on a proxy's bare 502 while no banner is up", async () => {
    vi.useFakeTimers();
    vi.spyOn(console, "error").mockImplementation(() => {});
    const client = createQueryClient();
    client.mount();
    const gateway = vi.fn(
      async () => new Response("Bad Gateway", { status: 502 }),
    );
    vi.stubGlobal("fetch", gateway);

    for (const read of ["first", "second"]) {
      await expect(
        client.fetchQuery({ queryKey: ["gateway", read], queryFn: readMe }),
      ).rejects.toBeInstanceOf(ProblemError);
    }
    expect(gateway).toHaveBeenCalledTimes(2);
    expect(onlineManager.isOnline()).toBe(true);
    client.unmount();
  });

  it("pauses reads behind a proxy's bare 502 while the banner is up, until a probe answers", async () => {
    vi.useFakeTimers();
    vi.spyOn(console, "error").mockImplementation(() => {});
    const client = createQueryClient();
    client.mount();
    bannerUp();
    let apiUp = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        if (!apiUp) return new Response("Bad Gateway", { status: 502 });
        const url = input instanceof Request ? input.url : String(input);
        return url === "/healthz"
          ? new Response("ok")
          : new Response("{}", {
              headers: { "Content-Type": "application/json" },
            });
      }),
    );

    await expect(
      client.fetchQuery({ queryKey: ["gateway", "first"], queryFn: readMe }),
    ).rejects.toBeInstanceOf(ProblemError);
    // The probe the bare 502 sent gets a bare 502 as well.
    await vi.advanceTimersByTimeAsync(0);
    const held = client.fetchQuery({
      queryKey: ["gateway", "second"],
      queryFn: readMe,
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(client.getQueryState(["gateway", "second"])?.fetchStatus).toBe(
      "paused",
    );

    apiUp = true;
    await vi.advanceTimersByTimeAsync(2_000);
    await expect(held).resolves.toEqual({});
    client.unmount();
  });
});

// The banner states an outage once; a console line per refused request would
// bury the failures an operator actually opens the console for.
describe("the failure sinks during an outage", () => {
  const refused = (method: string) =>
    new ConnectivityError(
      "unreachable",
      new Request("http://localhost/v1/contacts", { method }),
      new TypeError("Failed to fetch"),
      false,
    );

  it("report neither a read nor a write the network refused", async () => {
    const reported = vi.spyOn(console, "error").mockImplementation(() => {});
    const client = createQueryClient();

    await expect(
      client.fetchQuery({
        queryKey: ["outage-sink"],
        queryFn: () => Promise.reject(refused("GET")),
      }),
    ).rejects.toBeInstanceOf(ConnectivityError);
    await expect(
      new MutationObserver(client, {
        mutationFn: () => Promise.reject(refused("PATCH")),
      }).mutate(),
    ).rejects.toBeInstanceOf(ConnectivityError);

    expect(reported).not.toHaveBeenCalled();
  });
});

import {
  MutationObserver,
  onlineManager,
  type QueryClient,
} from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ProblemError, throwProblem } from "../screens/common";
import { ENTITY_NAME_KEY } from "../screens/entityref";
import {
  ConnectivityError,
  reportReached,
  reportUnreached,
  watchConnectivity,
} from "./connectivity";
import {
  createQueryClient,
  liveInterval,
  liveOnReturn,
  retryQuery,
} from "./queryclient";

// The data-layer parameters are invisible until they are wrong: a retried 4xx
// doubles a refusal the server already made final, and an unreported failure
// leaves nothing behind for whoever has to explain it. Both are pinned here.

function problem(status: number): ProblemError {
  return new ProblemError({
    type: "about:blank",
    status,
    code: "test_case",
    detail: "a server problem",
  });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("the query retry policy", () => {
  it("retries a server error, at most twice", () => {
    expect(retryQuery(0, problem(503))).toBe(true);
    expect(retryQuery(1, problem(503))).toBe(true);
    expect(retryQuery(2, problem(503))).toBe(false);
  });

  it("never retries a refusal the server has settled, however early", () => {
    // Both are classed as server errors and neither is a failure the server
    // may recover from: 501 does not support what was asked, 505 will not
    // speak this version. A build reaches 501 deliberately — httperr answers
    // it for a surface the contract specifies and this build does not wire —
    // so a retry here is three requests and three console errors for one
    // settled answer, read first by an operator looking for a real fault.
    for (const status of [501, 505]) {
      expect(retryQuery(0, problem(status)), String(status)).toBe(false);
    }
    // And the neighbours are still retried, so the exclusion is these two
    // rather than a range that swallowed them.
    for (const status of [500, 502, 503, 504]) {
      expect(retryQuery(0, problem(status)), String(status)).toBe(true);
    }
  });

  it("never retries a client error, however early the failure", () => {
    for (const status of [400, 401, 403, 404, 409, 422, 429]) {
      expect(retryQuery(0, problem(status)), String(status)).toBe(false);
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
      30_000,
    );
  });

  // FE-PARAM-3 and FE-PARAM-5 are one predicate each rather than a flag, and
  // both answers of each are load-bearing: a record read repeats and refetches
  // on the way back to the tab, and nothing else does either. Asked of both
  // kinds of key, because what a change could get wrong is one kind while the
  // other stays right.
  //
  // The predicates, not the defaults they are installed as: what they are
  // wired to is proved end to end in screens/liverecord.test.tsx, by driving
  // each record page's own read through this client.
  it("repeats a record read, and nothing else", () => {
    expect(liveInterval({ queryKey: ["contact360", "p-1"] })).toBe(60_000);
    expect(liveInterval({ queryKey: ["me", "ai-activity"] })).toBe(false);
  });

  it("refetches a record on the way back to the tab, and nothing else", () => {
    // "always" and not `true`: `true` refetches only a read already stale, so
    // inside the staleness window above the return would be served the cache.
    expect(liveOnReturn({ queryKey: ["contact360", "p-1"] })).toBe("always");
    expect(liveOnReturn({ queryKey: ["me", "ai-activity"] })).toBe(false);
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

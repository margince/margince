import { MutationObserver, type QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ProblemError } from "../screens/common";
import { ENTITY_NAME_KEY } from "../screens/entityref";
import { createQueryClient, retryQuery } from "./queryclient";

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
  it("serves cached data for the pinned staleness window, and not on focus", () => {
    const defaults = createQueryClient().getDefaultOptions().queries;
    expect(defaults?.staleTime).toBe(30_000);
    expect(defaults?.refetchOnWindowFocus).toBe(false);
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
      queryKey: ["organization", ENTITY_NAME_KEY, id],
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
      client.getQueryState(["organization", ENTITY_NAME_KEY, "o-1"])
        ?.isInvalidated,
    ).toBe(false);

    await new MutationObserver(client, {
      mutationFn: () => Promise.resolve("renamed"),
    }).mutate();

    expect(
      client.getQueryState(["organization", ENTITY_NAME_KEY, "o-1"])
        ?.isInvalidated,
    ).toBe(true);
  });

  // The other keys are a screen's own reads, and a screen that has just
  // written invalidates what it owns. Refetching every read in the cache after
  // every write would put the whole page back on the network.
  it("leaves a screen's own reads to the screen", async () => {
    const client = createQueryClient();
    await client.fetchQuery({
      queryKey: ["organization360", "o-1"],
      queryFn: () => Promise.resolve({ id: "o-1" }),
    });

    await new MutationObserver(client, {
      mutationFn: () => Promise.resolve("written"),
    }).mutate();

    expect(
      client.getQueryState(["organization360", "o-1"])?.isInvalidated,
    ).toBe(false);
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
      ["record-history", "organization", "o-1"],
      ["field-history", "organization", "o-1", "", ""],
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
      ["record-history", "organization", "o-1"],
      ["field-history", "organization", "o-1", "", ""],
    ]) {
      expect(client.getQueryState(key)?.isInvalidated).toBe(true);
    }
  });

  // A failed write changed nothing, so the history on screen is still correct
  // and refetching it would spend a read to redraw the same list.
  it("leaves the history alone when the write failed", async () => {
    const client = createQueryClient();
    const key = ["record-history", "organization", "o-2"];
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

/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { api, RequestTimeoutError } from "./client";

// The spec for the client's deadline. The failure it exists for is the one
// nothing above can see: a request that opens and never answers looks exactly
// like a request still arriving, so `isPending` stays true, no error state is
// ever reached, and the authenticated shell holds its splash with no error, no
// retry and no explanation until the reader reloads the page.
//
// Fake timers throughout: the deadline is 45 seconds, and a test that waited
// for it would be the slowest one in the suite and would still be measuring the
// runner's mood rather than the client's promise.

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

// A server that accepted the connection and then said nothing — a proxy holding
// it open, a socket that died under a suspended laptop. It settles only for
// whoever aborts it, which is the whole point: without the deadline nobody
// does.
function neverAnswers() {
  return vi.fn((_request: Request, init?: RequestInit) => {
    const signal = init?.signal;
    if (!signal) {
      throw new Error("the client opened a request with no abort signal");
    }
    return new Promise<Response>((_resolve, reject) => {
      signal.addEventListener("abort", () => reject(signal.reason));
    });
  });
}

describe("the api client's request deadline", () => {
  it("turns a request that never answers into an error", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", neverAnswers());
    const read = api.GET("/me");
    const settled = vi.fn();
    // Attached before either assertion so neither outcome can be an unhandled
    // rejection, and so the first assertion is about the promise rather than
    // about how long this test happened to take.
    read.then(settled, settled);

    await vi.advanceTimersByTimeAsync(44_000);
    expect(settled).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1_000);
    await expect(read).rejects.toBeInstanceOf(RequestTimeoutError);
  });

  it("names the request that ran out of time, rather than failing anonymously", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", neverAnswers());
    const read = api.GET("/me");
    const reason = read.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(45_000);
    // A stall and a refusal are different facts, and the console line an
    // operator reads has to say which one this was and for which request.
    expect(await reason).toMatchObject({
      name: "RequestTimeoutError",
      message: expect.stringContaining("/v1/me"),
    });
  });

  it("drops the deadline when the request answers", async () => {
    vi.useFakeTimers();
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response("{}", {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );
    await api.GET("/me");
    // A timer still armed here would abort a controller nobody is listening to
    // and keep the page awake for 45 seconds after every answered request.
    expect(vi.getTimerCount()).toBe(0);
  });
});

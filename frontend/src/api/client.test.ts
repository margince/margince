/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { api, REQUEST_TIMEOUT_MS, RequestTimeoutError } from "./client";

// The spec for the client's deadline. The failure it exists for is the one
// nothing above can see: a request that opens and never answers looks exactly
// like a request still arriving, so `isPending` stays true, no error state is
// ever reached, and the authenticated shell holds its splash with no error, no
// retry and no explanation until the reader reloads the page.
//
// Fake timers throughout: the deadline is minutes long, and a test that waited
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

    await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS - 1_000);
    expect(settled).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1_000);
    await expect(read).rejects.toBeInstanceOf(RequestTimeoutError);
  });

  it("names the request that ran out of time, rather than failing anonymously", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch", neverAnswers());
    const read = api.GET("/me");
    const reason = read.catch((error: unknown) => error);

    await vi.advanceTimersByTimeAsync(REQUEST_TIMEOUT_MS);
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
    // and keep the page awake for the whole deadline after every answered request.
    expect(vi.getTimerCount()).toBe(0);
  });
});

describe("a gateway that gave up on the app behind it", () => {
  // 502/503/504 come from the PROXY, not the app, so they carry no RFC 7807
  // body. Every reader of problemDetail then finds no code and falls back to
  // its own generic sentence — which is how a reply draft that ran 45 seconds
  // and had its connection cut reached the screen as "The request failed.
  // Please try again." with nothing in it to act on.
  it("gives a bodiless 502 a problem body the reader can be told about", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(null, { status: 502, statusText: "Bad Gateway" }),
      ),
    );

    const { error, response } = await api.GET("/organizations/{id}/dossier", {
      params: { path: { id: "01a0-4cd2" } },
    });

    expect(response.status).toBe(502);
    expect(error).toMatchObject({
      code: "gateway_unavailable",
      status: 502,
    });
  });

  // A gateway status the app DID answer keeps its own body: the server's
  // sentence is always better than one synthesized here.
  it("leaves a 503 the app itself explained alone", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(
            JSON.stringify({ code: "maintenance_mode", detail: "upgrading" }),
            {
              status: 503,
              headers: { "Content-Type": "application/problem+json" },
            },
          ),
      ),
    );

    const { error } = await api.GET("/organizations/{id}/dossier", {
      params: { path: { id: "01a0-4cd2" } },
    });

    expect(error).toMatchObject({ code: "maintenance_mode" });
  });

  // An ordinary refusal is untouched.
  it("does not invent a body for a 422", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ code: "validation_error" }), {
            status: 422,
            headers: { "Content-Type": "application/problem+json" },
          }),
      ),
    );

    const { error } = await api.GET("/organizations/{id}/dossier", {
      params: { path: { id: "01a0-4cd2" } },
    });

    expect(error).toMatchObject({ code: "validation_error" });
  });

  // The narrowness is the contract, not an accident: surfaces across the app
  // read a bodiless 5xx as an ordinary server fault — the composer branches on
  // a bare 501, the connector screens on a bodiless 503 — and telling those
  // readers "the work may still be running" would be false about a mailer that
  // is simply not wired.
  it("leaves a bodiless 502 on an ordinary route alone", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(null, { status: 502 })),
    );

    const { error } = await api.GET("/me");

    expect(error).not.toMatchObject({ code: "gateway_unavailable" });
  });
});

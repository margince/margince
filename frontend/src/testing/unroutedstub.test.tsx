// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  emptyListFallback,
  installFetchStub,
  jsonResponse,
  refuseUnrouted,
} from "./story-utils";
import {
  takeEmptyListAnswers,
  takeUnroutedSessionProbes,
  UNROUTED_WARNING,
} from "./unrouted-session";

let consoleError: ReturnType<typeof vi.spyOn>;
let consoleWarn: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  consoleWarn = vi.spyOn(console, "warn").mockImplementation(() => {});
  takeEmptyListAnswers();
});

afterEach(() => {
  vi.restoreAllMocks();
  takeEmptyListAnswers();
});

describe("a stub's unrouted non-session request", () => {
  it("is answered with an empty list and recorded by method and path", async () => {
    installFetchStub({});

    const body = await (await fetch("/v1/worklist")).json();

    expect(body.data).toEqual([]);
    expect(takeEmptyListAnswers()).toEqual(["GET /worklist"]);
  });

  it("names the route in a console warning a story gate can read", async () => {
    installFetchStub({}, emptyListFallback);
    const spoken = vi.fn();
    consoleWarn.mockImplementation(spoken);

    await fetch("/v1/activities/email-1", { method: "GET" });

    // The setup keeps unit suites quiet; a story run is the one that is told.
    expect(takeEmptyListAnswers()).toEqual(["GET /activities/email-1"]);
    expect(spoken.mock.calls.length <= 1).toBe(true);
    for (const [text] of spoken.mock.calls) {
      expect(text).toBe(`${UNROUTED_WARNING} GET /activities/email-1`);
    }
  });

  it("leaves a routed read alone and records nothing", async () => {
    installFetchStub({ "GET /tags": () => jsonResponse({ ok: 1 }) });

    expect(await (await fetch("/v1/tags")).json()).toEqual({ ok: 1 });
    expect(takeEmptyListAnswers()).toEqual([]);
  });
});

describe("a story that refuses what it did not route", () => {
  it("refuses a read, naming the method and path", async () => {
    installFetchStub({}, refuseUnrouted);

    const answer = await fetch("/v1/worklist");

    expect(answer.status).toBe(501);
    expect((await answer.json()).title).toBe(
      "GET /worklist was not routed by this story",
    );
    expect(String(consoleError.mock.calls[0]?.[0])).toContain("GET /worklist");
  });

  it("refuses a write the same way", async () => {
    installFetchStub({}, refuseUnrouted);

    const answer = await fetch("/v1/contacts", { method: "POST", body: "{}" });

    expect(answer.status).toBe(501);
    expect(String(consoleError.mock.calls[0]?.[0])).toContain("POST /contacts");
  });
});

describe("the session probe", () => {
  it("is still refused and counted when the fallback is an empty list", async () => {
    installFetchStub({}, emptyListFallback);

    const answer = await fetch("/v1/me");

    expect(answer.status).toBe(501);
    expect(takeUnroutedSessionProbes()).toBe(1);
    expect(takeEmptyListAnswers()).toEqual([]);
  });
});

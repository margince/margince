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
  quietEmptyListAnswers,
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
  quietEmptyListAnswers(true);
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

  it("warns once with the route when warnings are on, as in a story run", async () => {
    quietEmptyListAnswers(false);
    installFetchStub({});

    await fetch("/v1/activities/email-1");

    expect(consoleWarn).toHaveBeenCalledTimes(1);
    expect(consoleWarn).toHaveBeenCalledWith(
      `${UNROUTED_WARNING} GET /activities/email-1`,
    );
  });

  it("stays quiet under the unit-test setup", async () => {
    installFetchStub({});

    await fetch("/v1/activities/email-1");

    expect(consoleWarn).not.toHaveBeenCalled();
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

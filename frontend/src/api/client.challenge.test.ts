// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import type { paths } from "@composition/schema";
import { afterEach, describe, expect, it, vi } from "vitest";
import { connectivityNow } from "../lib/connectivity";
import { EmptySuccessError, unwrap } from "../lib/problem";
import { api } from "./client";

// What the client makes of a read an intermediary answered instead of the API.
// The exact answer is a bodiless 202 with an HTML type and a challenge header.
// openapi-fetch reads it as no data and no error at all.

afterEach(() => {
  vi.unstubAllGlobals();
});

function challenge(status = 202): Response {
  return new Response(null, {
    status,
    headers: {
      "content-type": "text/html; charset=UTF-8",
      "x-amzn-waf-action": "challenge",
    },
  });
}

function answering(response: () => Response) {
  const fetch = vi.fn(async () => response());
  vi.stubGlobal("fetch", fetch);
  return fetch;
}

// The statuses the client refuses on a read are statuses no read in the
// contract answers with. This fails to compile once a GET in crm.yaml gains one.
type ReadStatus = {
  [P in keyof paths]: paths[P] extends { get: { responses: infer R } }
    ? keyof R
    : never;
}[keyof paths];
const readsNeverAnswer: [Extract<ReadStatus, 202 | 205>] extends [never]
  ? true
  : false = true;

describe("a read an intermediary answered", () => {
  it("is no status any read in the contract answers with", () => {
    expect(readsNeverAnswer).toBe(true);
  });

  it("refuses the bodiless challenge rather than reading it as data", async () => {
    answering(() => challenge());

    const read = api.GET("/scheduled-sends", {});

    await expect(read).rejects.toBeInstanceOf(EmptySuccessError);
    await expect(read).rejects.toMatchObject({ status: 202 });
  });

  it("refuses a 205 on a read the same way", async () => {
    answering(() => challenge(205));

    await expect(api.GET("/scheduled-sends", {})).rejects.toBeInstanceOf(
      EmptySuccessError,
    );
  });

  // The connection answered, so the banner must not report an outage.
  it("does not count as the server being unreachable", async () => {
    answering(() => challenge());

    await expect(api.GET("/me/ai-activity", {})).rejects.toBeInstanceOf(
      EmptySuccessError,
    );

    expect(connectivityNow()).toBe("online");
  });

  // Not the same failure as the bodiless one, and not data either. The JSON
  // reader refuses the page before any screen can read a field off it.
  it("refuses an HTML page served with a 200 as unreadable", async () => {
    answering(
      () =>
        new Response("<html><body>Checking your browser</body></html>", {
          status: 200,
          headers: { "content-type": "text/html; charset=UTF-8" },
        }),
    );

    const read = api.GET("/worklist", {});

    await expect(read).rejects.toBeInstanceOf(SyntaxError);
    await expect(read).rejects.not.toBeInstanceOf(EmptySuccessError);
  });

  it("refuses a 200 with no body at the reader's unwrap", async () => {
    answering(() => new Response(null, { status: 200 }));

    const result = await api.GET("/worklist", {});

    expect(() => unwrap(result)).toThrow(EmptySuccessError);
  });
});

describe("the answers a challenge must not be confused with", () => {
  // An accepted job: the contract's own 202, on a write, with nothing in it.
  it("keeps a write's bodiless 202 as accepted", async () => {
    answering(() => new Response(null, { status: 202 }));

    const result = await api.POST("/companies/{id}/vat-check", {
      params: { path: { id: "01a0-4cd2" } },
    });

    expect(result.response.status).toBe(202);
    expect(result.error).toBeUndefined();
    expect(() => unwrap(result)).not.toThrow();
  });

  it("keeps a write's 204 as done", async () => {
    answering(() => new Response(null, { status: 204 }));

    const result = await api.DELETE("/passports/{id}", {
      params: { path: { id: "01a0-4cd2" } },
    });

    expect(() => unwrap(result)).not.toThrow();
  });

  it("keeps a binary read as its bytes", async () => {
    answering(
      () =>
        new Response(new Uint8Array([137, 80, 78, 71]), {
          status: 200,
          headers: { "content-type": "image/png" },
        }),
    );

    const { data } = await api.GET("/users/{id}/avatar", {
      params: { path: { id: "01a0-4cd2" } },
      parseAs: "blob",
    });

    expect(data).toBeInstanceOf(Blob);
  });

  it("keeps a problem body as the server's refusal", async () => {
    answering(
      () =>
        new Response(
          JSON.stringify({ status: 403, code: "permission_denied" }),
          {
            status: 403,
            headers: { "content-type": "application/problem+json" },
          },
        ),
    );

    const result = await api.GET("/scheduled-sends", {});

    expect(result.error).toMatchObject({ code: "permission_denied" });
  });
});

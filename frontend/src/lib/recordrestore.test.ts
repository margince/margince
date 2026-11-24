// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { afterEach, describe, expect, it, vi } from "vitest";
import { restoreLatestArchive } from "./recordrestore";

afterEach(() => {
  vi.unstubAllGlobals();
});

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// Every request the helper sends, with the version it pins the restore to.
function serveHistory(entries: { id: string }[]) {
  const sent: { method: string; url: string; ifMatch: string | null }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      sent.push({
        method: request.method,
        url: request.url,
        ifMatch: request.headers.get("If-Match"),
      });
      return request.method === "GET"
        ? json({ data: entries, page: { has_more: false } })
        : json({ id: "a-restore" });
    }),
  );
  return sent;
}

describe("restoreLatestArchive", () => {
  it("puts back the newest archive entry, behind the version the archive answered with", async () => {
    const sent = serveHistory([{ id: "a-archive" }]);

    expect(await restoreLatestArchive("contact", "p-1", 8)).toBe(true);

    const [read, restore] = sent;
    expect(read?.method).toBe("GET");
    const asked = new URL(read?.url ?? "");
    expect(asked.pathname).toMatch(/\/records\/contact\/p-1\/history$/);
    expect(asked.searchParams.get("action")).toBe("archive");
    expect(asked.searchParams.get("limit")).toBe("1");
    expect(restore?.method).toBe("POST");
    expect(restore?.url).toMatch(
      /\/records\/contact\/p-1\/history\/a-archive\/restore$/,
    );
    expect(restore?.ifMatch).toBe("8");
  });

  it("writes nothing for a record whose history holds no archive", async () => {
    const sent = serveHistory([]);

    expect(await restoreLatestArchive("contact", "p-1", 8)).toBe(false);
    expect(sent.map((request) => request.method)).toEqual(["GET"]);
  });
});

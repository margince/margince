/** @vitest-environment happy-dom */
import { focusManager } from "@tanstack/react-query";
import { cleanup, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { readStored, STORAGE_KEYS, writeStored } from "./lib/storage";
import { memoryStorage, renderApp, teardownApp } from "./testing/appharness";
import { meFixture } from "./testing/mefixture";

// Home's day and receipt are its slowest reads. On a browser that had a
// session they start with the session probe, not after the gates it opens. An
// answer that sends the reader elsewhere stops them.

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Seen = { path: string; signal: AbortSignal | undefined };

// A server that holds each answer named in held until the test releases it.
// The rest answer 503, so no screen draws past its error state.
function server(answers: Record<string, () => Response>, held: string[]) {
  const seen: Seen[] = [];
  const release = new Map<string, () => void>();
  const fetch = vi.fn(async (input: Request | string | URL) => {
    const request = input instanceof Request ? input : new Request(input);
    const path = new URL(request.url).pathname.replace(/^\/v1/, "");
    seen.push({ path, signal: request.signal });
    if (held.includes(path)) {
      await new Promise<void>((resolve) => {
        release.set(path, resolve);
        request.signal.addEventListener("abort", () => resolve());
      });
    }
    const answer = answers[path];
    return answer ? answer() : json({ code: "service_unavailable" }, 503);
  });
  vi.stubGlobal("fetch", fetch);
  return { seen, release: (path: string) => release.get(path)?.() };
}

const asked = (seen: Seen[], path: string) => seen.find((s) => s.path === path);

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  vi.stubGlobal("scrollTo", vi.fn());
  window.location.hash = "#/home";
});

afterEach(async () => {
  cleanup();
  await teardownApp();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.location.hash = "";
});

it("asks for Home's day and receipt while the session probe is still out", async () => {
  writeStored(STORAGE_KEYS.sessionSeen, "1");
  const { seen, release } = server(
    { "/me": () => json(meFixture({ roles: ["rep"] })) },
    ["/me"],
  );
  renderApp();
  await waitFor(() => {
    expect(asked(seen, "/worklist")).toBeDefined();
    expect(asked(seen, "/magic")).toBeDefined();
  });
  release("/me");
});

it("stops Home's reads once the gate sends the reader to describe the company", async () => {
  writeStored(STORAGE_KEYS.sessionSeen, "1");
  const { seen } = server(
    {
      "/me": () => json(meFixture({ roles: ["admin"] })),
      "/company": () => json({ detail: "no company yet" }, 404),
    },
    ["/worklist"],
  );
  renderApp();
  await waitFor(() =>
    expect(window.location.hash).toBe("#/onboarding/company"),
  );
  await waitFor(() =>
    expect(asked(seen, "/worklist")?.signal?.aborted).toBe(true),
  );
});

it("leaves a browser that had no session to its sign-in, asking for nothing of Home", async () => {
  const { seen } = server(
    { "/me": () => json({ title: "unauthorized" }, 401) },
    [],
  );
  renderApp();
  await waitFor(() => expect(asked(seen, "/me")).toBeDefined());
  await waitFor(() => expect(asked(seen, "/auth/capabilities")).toBeDefined());
  expect(asked(seen, "/worklist")).toBeUndefined();
  expect(asked(seen, "/magic")).toBeUndefined();
});

it("forgets the session it saw once the probe is refused", async () => {
  writeStored(STORAGE_KEYS.sessionSeen, "1");
  server({ "/me": () => json({ title: "unauthorized" }, 401) }, []);
  renderApp();
  await waitFor(() => expect(readStored(STORAGE_KEYS.sessionSeen)).toBeNull());
});

it("forgets the session when a later probe is refused", async () => {
  let probes = 0;
  server(
    {
      "/me": () =>
        ++probes === 1
          ? json(meFixture({ roles: ["rep"] }))
          : json({ title: "unauthorized" }, 401),
    },
    [],
  );
  renderApp();
  await waitFor(() => expect(readStored(STORAGE_KEYS.sessionSeen)).toBe("1"));
  // The reader comes back to the tab, and the probe is asked again.
  focusManager.setFocused(false);
  focusManager.setFocused(true);
  await waitFor(() => expect(probes).toBeGreaterThan(1));
  await waitFor(() => expect(readStored(STORAGE_KEYS.sessionSeen)).toBeNull());
});

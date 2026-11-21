/** @vitest-environment happy-dom */
import { focusManager } from "@tanstack/react-query";
import { cleanup, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  readStored,
  removeStored,
  STORAGE_KEYS,
  writeStored,
} from "./lib/storage";
import {
  memoryStorage,
  renderApp,
  teardownApp,
  wizardRow,
} from "./testing/appharness";
import { meFixture } from "./testing/mefixture";

// Home's day and receipt are its slowest reads. On a browser that saw its
// reader finish onboarding they start with the session probe, not after the
// gates it opens. An answer that sends the reader elsewhere stops them.

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

// A browser that had a session, and saw its reader finish onboarding.
function returningFinished() {
  writeStored(STORAGE_KEYS.sessionSeen, "1");
  writeStored(
    { family: STORAGE_KEYS.onboardingComplete, member: meFixture().user.id },
    "1",
  );
}

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

it("asks for Home's day while the session probe is out, and the deal reads once the grant is known", async () => {
  returningFinished();
  const { seen, release } = server(
    {
      "/me": () =>
        json(meFixture({ roles: ["rep"], allow: { deal: ["read"] } })),
    },
    ["/me"],
  );
  renderApp();
  await waitFor(() => expect(asked(seen, "/worklist")).toBeDefined());
  expect(asked(seen, "/magic")).toBeUndefined();
  release("/me");
  await waitFor(() => expect(asked(seen, "/magic")).toBeDefined());
});

// The day is the slow read, so it is still out when the probe answers. The
// grant in that answer starts the deal reads and must leave the day alone.
it("keeps the day's read in flight when the probe answers with the deal grant", async () => {
  returningFinished();
  const { seen } = server(
    {
      "/me": () =>
        json(meFixture({ roles: ["rep"], allow: { deal: ["read"] } })),
    },
    ["/worklist"],
  );
  renderApp();
  await waitFor(() => expect(asked(seen, "/magic")).toBeDefined());
  const days = seen.filter((s) => s.path === "/worklist");
  expect(days).toHaveLength(1);
  expect(days[0]?.signal?.aborted).toBe(false);
});

it("sends no deal read for a reader the probe answers without the deal grant", async () => {
  writeStored(STORAGE_KEYS.sessionSeen, "1");
  const { seen } = server(
    { "/me": () => json(meFixture({ roles: ["rep"] })) },
    [],
  );
  renderApp();
  await waitFor(() =>
    expect(asked(seen, "/installation/settings")).toBeDefined(),
  );
  expect(asked(seen, "/worklist")).toBeDefined();
  expect(asked(seen, "/brief")).toBeUndefined();
  expect(asked(seen, "/magic")).toBeUndefined();
});

it("stops Home's reads once the gate sends the reader to describe the company", async () => {
  returningFinished();
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

it("stops Home's reads when the session mark is erased, as a sign-out does", async () => {
  returningFinished();
  const { seen } = server({}, ["/me", "/worklist"]);
  renderApp();
  await waitFor(() => expect(asked(seen, "/worklist")).toBeDefined());
  removeStored(STORAGE_KEYS.sessionSeen);
  await waitFor(() =>
    expect(asked(seen, "/worklist")?.signal?.aborted).toBe(true),
  );
});

describe("a returning browser that never saw its reader finish onboarding", () => {
  const HOME_READS = ["/worklist", "/worklist/handled", "/brief", "/magic"];
  // A member with a step to take: setup holds only a seat that has one.
  const allow = {
    deal: ["read"],
    voice_profile: ["read", "create"],
    activity: ["create"],
  } as const;
  const journey = (row: ReturnType<typeof wizardRow>) => ({
    "/me": () => json(meFixture({ roles: ["rep"], allow })),
    "/company/context/capabilities": () =>
      json({ onboarding_enabled: true, read_enabled: true }),
    "/onboarding/state": () => json(row),
  });

  it("asks nothing of Home for an unfinished member, before or after the gate answers", async () => {
    writeStored(STORAGE_KEYS.sessionSeen, "1");
    const { seen, release } = server(journey(wizardRow("voice")), [
      "/onboarding/state",
    ]);
    renderApp();
    await waitFor(() => expect(asked(seen, "/onboarding/state")).toBeDefined());
    expect(HOME_READS.filter((path) => asked(seen, path))).toEqual([]);
    release("/onboarding/state");
    await waitFor(() =>
      expect(window.location.hash).toMatch(/^#\/onboarding\//),
    );
    expect(HOME_READS.filter((path) => asked(seen, path))).toEqual([]);
  });

  it("asks Home's reads once the gate admits a finished member", async () => {
    writeStored(STORAGE_KEYS.sessionSeen, "1");
    const { seen, release } = server(journey(wizardRow("complete")), [
      "/onboarding/state",
    ]);
    renderApp();
    await waitFor(() => expect(asked(seen, "/onboarding/state")).toBeDefined());
    expect(asked(seen, "/worklist")).toBeUndefined();
    release("/onboarding/state");
    await waitFor(() =>
      expect(HOME_READS.filter((path) => asked(seen, path))).toEqual(
        HOME_READS,
      ),
    );
  });
});

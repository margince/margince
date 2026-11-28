/** @vitest-environment happy-dom */
import { QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { StrictMode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import { createQueryClient } from "./app/queryclient";
import { LocaleProvider } from "./i18n";
import { memoryStorage, teardownApp, wizardRow } from "./testing/appharness";
import { meFixture } from "./testing/mefixture";
// Loaded while the file is collected, so no case races a screen's first import.
import "./features/brief/brief";
import "./features/onboarding/onboarding";
import "./App.settings";
import "./features/settings/settings";
import { en as englishCopy } from "./i18n/en";

// What a member held on onboarding asks of the server, split from the gate's
// own cases in App.onboardinggate.test.tsx.

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const GATE_TARGET = "#/onboarding/company";
const BRIEF = "#/home";

function renderApp() {
  render(
    <StrictMode>
      <QueryClientProvider client={createQueryClient()}>
        <LocaleProvider initial="en">
          <App />
        </LocaleProvider>
      </QueryClientProvider>
    </StrictMode>,
  );
}

beforeEach(() => {
  vi.stubGlobal("localStorage", memoryStorage());
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(async () => {
  cleanup();
  await teardownApp();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  window.location.hash = "";
});

describe("the reads of a member held on onboarding", () => {
  const SIDE_READS = [
    "/brief",
    "/worklist/handled",
    "/analytics/context",
    "/notices",
    "/scheduled-sends",
    "/magic",
  ];

  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    // The handoff after the last step resolves at once under reduced motion.
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query.includes("prefers-reduced-motion"),
      media: query,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
    }));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  function memberApi(row: ReturnType<typeof wizardRow> | null) {
    const seen: string[] = [];
    let version = row?.version ?? 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: Request | string | URL) => {
        const request = input instanceof Request ? input : null;
        const url = new URL(String(request?.url ?? input), "http://localhost");
        const path = url.pathname.replace(/^\/v1/, "");
        seen.push(`${request?.method ?? "GET"} ${path}`);
        if (path === "/me") {
          const grants = {
            deal: ["read"],
            forecast: ["read"],
            voice_profile: ["read", "create"],
            activity: ["create"],
          } as const;
          return json(meFixture({ roles: ["rep"], allow: grants }));
        }
        if (path === "/company/context/capabilities") {
          return json({ onboarding_enabled: true, read_enabled: true });
        }
        if (path === "/onboarding/state" && request?.method === "PUT") {
          version += 1;
          const body: unknown = await request.clone().json();
          return json({ ...wizardRow("voice"), ...Object(body), version });
        }
        if (path === "/onboarding/state") {
          return row === null ? new Response(null, { status: 204 }) : json(row);
        }
        // Home fails each of its reads on its own card rather than on a fixture.
        if (path === "/worklist" || SIDE_READS.includes(path)) {
          return json({ code: "service_unavailable" }, 503);
        }
        return json({ data: [], page: {} });
      }),
    );
    const count = (call: string) => seen.filter((it) => it === call).length;
    return {
      count,
      sideReads: () => SIDE_READS.filter((path) => count(`GET ${path}`) > 0),
    };
  }

  // Past the moment the shell's own background reads would have started.
  const restLong = () => act(() => vi.advanceTimersByTimeAsync(5_000));

  it.each([
    ["no row yet", null],
    ["saved progress", wizardRow("voice")],
  ])("reads the row once through the first save, with %s", async (_, row) => {
    window.location.hash = GATE_TARGET;
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const api = memberApi(row);
    renderApp();
    await user.click(
      await screen.findByRole("button", {
        name: englishCopy["ob.conv.team.skip"],
      }),
    );
    await waitFor(() => expect(api.count("PUT /onboarding/state")).toBe(1));
    await restLong();
    expect(api.count("GET /onboarding/state")).toBe(1);
    expect(api.sideReads()).toEqual([]);
  });

  it("starts none of Home's reads for a member the gate turns away from it", async () => {
    window.location.hash = BRIEF;
    const api = memberApi(wizardRow("voice"));
    renderApp();
    await screen.findByRole("button", {
      name: englishCopy["ob.conv.team.skip"],
    });
    // The gate's own address, which the voice step then names as its own.
    expect(window.location.hash).toBe("#/onboarding/voice");
    await restLong();
    expect(api.sideReads()).toEqual([]);
  });

  it("sends neither the outbox's nor the bell's read on Settings > Account until let in", async () => {
    window.location.hash = "#/settings/account";
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const api = memberApi(wizardRow("connect"));
    renderApp();
    await screen.findByRole("button", { name: "Sign out" });
    await restLong();
    expect(api.count("GET /notices")).toBe(0);
    expect(api.count("GET /scheduled-sends")).toBe(0);

    act(() => window.history.pushState(null, "", GATE_TARGET));
    act(() => window.dispatchEvent(new PopStateEvent("popstate")));
    const skip = await screen.findByRole("button", {
      name: englishCopy["ob.conv.team.skip"],
    });
    await waitFor(() => expect(skip).toHaveProperty("disabled", false));
    await user.click(skip);
    await waitFor(() => expect(window.location.hash).toBe(BRIEF));
    await restLong();
    expect(api.count("GET /notices")).toBeGreaterThan(0);
    expect(api.count("GET /scheduled-sends")).toBeGreaterThan(0);
  });

  it("starts the full app's reads once finishing lets the member in", async () => {
    window.location.hash = GATE_TARGET;
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    const api = memberApi(wizardRow("connect"));
    renderApp();
    const skip = await screen.findByRole("button", {
      name: englishCopy["ob.conv.team.skip"],
    });
    await waitFor(() => expect(skip).toHaveProperty("disabled", false));
    await restLong();
    expect(api.sideReads()).toEqual([]);

    await user.click(skip);
    await waitFor(() => expect(window.location.hash).toBe(BRIEF));
    await restLong();
    await waitFor(() => expect(api.sideReads()).toEqual(SIDE_READS));
  });
});

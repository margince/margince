// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import type { ConnectorsResult } from "./connectors";
import { type SettingsPageId, settingsReach } from "./settingscatalog";
import {
  deriveSetupSteps,
  type SetupReads,
  useSetupChecklist,
} from "./setup-checklist";
import { forgetSeat } from "./storage";

// The reminder and the settings home both draw these steps. These cases pin
// what each roster asks of the reader. Nothing is stored except the reader's
// own "hide until next sign-in".

type CaptureConnection = components["schemas"]["CaptureConnection"];
type Provider = CaptureConnection["provider"];
type MailProvider = ConnectorsResult["providers"][number]["provider"];
type Readiness = ConnectorsResult["providers"][number]["reason"];
type VoiceProfile = components["schemas"]["VoiceProfile"];
type SchedulingProfile = components["schemas"]["SchedulingProfile"];

const VOICE_WRITER: GrantSpec = { voice_profile: ["read", "create", "update"] };
const MAIL_PROVIDERS: readonly MailProvider[] = ["gmail", "graph", "imap"];

// A calendar can write bookings unless a test takes its grant away.
const WRITE_GRANT: Partial<Record<Provider, string>> = {
  gcal: "https://www.googleapis.com/auth/calendar.events",
  graphcal: "Calendars.ReadWrite",
};

function connection(
  provider: Provider,
  status: CaptureConnection["status"] = "connected",
  scopes: readonly string[] = [WRITE_GRANT[provider] ?? ""].filter(Boolean),
): CaptureConnection {
  return {
    id: `00000000-0000-4000-8000-${provider.padStart(12, "0")}`,
    provider,
    can_backfill: true,
    status,
    scopes: [...scopes],
    account_label: "ada@example.test",
  };
}

function roster(
  rows: readonly CaptureConnection[],
  readiness: Partial<Record<MailProvider, Readiness>> = {},
): ConnectorsResult {
  return {
    notConfigured: false,
    data: [...rows],
    providers: MAIL_PROVIDERS.map((provider) => ({
      provider,
      reason: readiness[provider] ?? "ready",
    })),
  };
}

function voiceProfile(profileVersion: number): VoiceProfile {
  return {
    id: "00000000-0000-4000-8000-0000000000a1",
    owner_id: "00000000-0000-4000-8000-000000000001",
    status: profileVersion > 0 ? "ready" : "collecting",
    maturity: profileVersion > 0 ? "building" : "collecting",
    quality_band: "thin",
    voice_profile_md: "",
    profile_version: profileVersion,
    personality_md: "",
    auto_learning_enabled: false,
    active_source_hash: null,
    candidate_version: null,
    last_built_at: null,
    source: "manual",
    captured_by: "00000000-0000-4000-8000-000000000001",
    version: 1,
    created_at: "2026-01-05T09:00:00Z",
    updated_at: "2026-01-05T09:00:00Z",
    archived_at: null,
  };
}

function scheduling(enabled: boolean): SchedulingProfile {
  return {
    enabled,
    provider: enabled ? "gcal" : "",
    calendar_id: enabled ? "primary" : "",
    duration_minutes: 30,
    notice_minutes: 240,
    horizon_days: 30,
    buffer_minutes: 0,
    title: "Intro call",
    location: "",
  };
}

function actsFor(allow: GrantSpec = VOICE_WRITER): ReadonlySet<SettingsPageId> {
  return new Set(settingsReach(meFixture({ allow })).acts.map((p) => p.id));
}

function reads(overrides: Partial<SetupReads> = {}): SetupReads {
  return {
    acts: actsFor(),
    connectors: roster([]),
    voice: { profile: null },
    scheduling: scheduling(false),
    ...overrides,
  };
}

function summary(input: SetupReads) {
  return deriveSetupSteps(input).map(({ id, state, blocked, reason }) => ({
    id,
    state,
    blocked,
    reason,
  }));
}

describe("deriveSetupSteps", () => {
  it("asks for all four steps from a reader with nothing set up, mail first", () => {
    expect(summary(reads())).toEqual([
      { id: "mail", state: "current", blocked: false, reason: null },
      { id: "calendar", state: "open", blocked: false, reason: null },
      { id: "voice", state: "open", blocked: false, reason: null },
      { id: "booking", state: "open", blocked: true, reason: "needsCalendar" },
    ]);
  });

  it("keeps asking for the calendar a Google mailbox came without", () => {
    const steps = deriveSetupSteps(
      reads({ connectors: roster([connection("gmail")]) }),
    );

    expect(steps[0]).toMatchObject({
      id: "mail",
      state: "done",
      account: { label: "ada@example.test", provider: "gmail" },
    });
    expect(steps[1]).toMatchObject({
      id: "calendar",
      state: "current",
      blocked: false,
      reason: "skipped",
    });
  });

  it("tells an IMAP reader their calendar comes from Google or Microsoft", () => {
    const steps = summary(reads({ connectors: roster([connection("imap")]) }));

    expect(steps[1]).toEqual({
      id: "calendar",
      state: "current",
      blocked: false,
      reason: "needsVendor",
    });
  });

  it("holds the calendar on an administrator while no company app is usable", () => {
    const steps = summary(
      reads({
        connectors: roster([connection("imap")], {
          gmail: "app_missing",
          graph: "app_unusable",
        }),
      }),
    );

    expect(steps.find((step) => step.id === "calendar")).toEqual({
      id: "calendar",
      state: "open",
      blocked: true,
      reason: "waitsAdmin",
    });
    // The blocked step is never the one to do next.
    expect(steps.find((step) => step.state === "current")?.id).toBe("voice");
  });

  it("offers nothing to do next while every connection waits on an administrator", () => {
    const steps = summary(
      reads({
        acts: actsFor({ voice_profile: ["read"] }),
        connectors: roster([], {
          gmail: "app_missing",
          graph: "app_missing",
          imap: "unsupported",
        }),
      }),
    );

    expect(steps).toEqual([
      { id: "mail", state: "open", blocked: true, reason: "waitsAdmin" },
      { id: "calendar", state: "open", blocked: true, reason: "waitsAdmin" },
      { id: "booking", state: "open", blocked: true, reason: "needsCalendar" },
    ]);
  });

  it("drops the calendar where this deployment serves neither vendor", () => {
    const steps = summary(
      reads({
        connectors: roster([], { gmail: "unsupported", graph: "unsupported" }),
      }),
    );

    expect(steps.map((step) => step.id)).toEqual(["mail", "voice"]);
  });

  it("opens the booking link only once a calendar is connected", () => {
    const before = summary(
      reads({ connectors: roster([connection("gmail")]) }),
    );
    const after = summary(
      reads({ connectors: roster([connection("gmail"), connection("gcal")]) }),
    );

    expect(before[3]).toMatchObject({ blocked: true, reason: "needsCalendar" });
    expect(after[3]).toEqual({
      id: "booking",
      state: "open",
      blocked: false,
      reason: null,
    });
    expect(after.filter((step) => step.state === "current")).toEqual([
      expect.objectContaining({ id: "voice" }),
    ]);
  });

  it("does not count a calendar that needs reconnecting as connected", () => {
    const steps = summary(
      reads({
        connectors: roster([
          connection("graph"),
          connection("graphcal", "reauth_required"),
        ]),
      }),
    );

    expect(steps[1]).toMatchObject({
      id: "calendar",
      state: "current",
      reason: "needsReconnect",
    });
  });

  // The row still says "connected" while its sync keeps failing. The
  // Connections page calls that needs attention, so the checklist must not
  // call the step done.
  it("does not finish a calendar whose sync keeps failing", () => {
    const steps = summary(
      reads({
        connectors: roster([
          connection("gmail"),
          { ...connection("gcal"), last_sync_error_class: "transient" },
        ]),
      }),
    );

    expect(steps[1]).toMatchObject({
      id: "calendar",
      state: "current",
      reason: "needsReconnect",
    });
  });

  it("asks to reconnect a calendar in error rather than offer a new one", () => {
    const steps = summary(
      reads({
        connectors: roster([connection("gmail"), connection("gcal", "error")]),
      }),
    );

    expect(steps[1]).toMatchObject({ id: "calendar", reason: "needsReconnect" });
  });

  it("asks to reconnect a calendar that can read but not write bookings", () => {
    const steps = summary(
      reads({
        connectors: roster([
          connection("gmail"),
          connection("gcal", "connected", []),
        ]),
      }),
    );

    expect(steps[1]).toMatchObject({
      id: "calendar",
      reason: "needsReconnect",
    });
    expect(steps[3]).toMatchObject({ id: "booking", blocked: true });
  });

  it("does not call a calendar disconnected later skipped", () => {
    const steps = summary(
      reads({
        connectors: roster([
          connection("gmail"),
          connection("gcal", "disconnected"),
        ]),
      }),
    );

    expect(steps[1]).toMatchObject({ id: "calendar", reason: null });
  });

  it("asks to reconnect an expired mailbox rather than connect a new one", () => {
    const steps = summary(
      reads({ connectors: roster([connection("gmail", "reauth_required")]) }),
    );

    expect(steps[0]).toEqual({
      id: "mail",
      state: "current",
      blocked: false,
      reason: "needsReconnect",
    });
  });

  it("counts a voice only once a version of it has been built", () => {
    const collecting = summary(reads({ voice: { profile: voiceProfile(0) } }));
    const built = summary(reads({ voice: { profile: voiceProfile(2) } }));

    expect(collecting[2]).toMatchObject({ id: "voice", state: "open" });
    expect(built[2]).toMatchObject({ id: "voice", state: "done" });
  });

  it("leaves out a step whose page the reader cannot act on", () => {
    const steps = summary(
      reads({ acts: actsFor({ voice_profile: ["read"] }) }),
    );

    expect(steps.map((step) => step.id)).toEqual([
      "mail",
      "calendar",
      "booking",
    ]);
  });

  it("leaves out the steps a refused read would have fed", () => {
    expect(summary(reads({ connectors: null })).map((s) => s.id)).toEqual([
      "voice",
    ]);
    expect(
      summary(reads({ voice: null, scheduling: null })).map((s) => s.id),
    ).toEqual(["mail", "calendar"]);
  });

  it("still counts a booking link that is on, whatever the roster says", () => {
    const steps = summary(
      reads({
        connectors: { notConfigured: true, data: [], providers: [] },
        scheduling: scheduling(true),
      }),
    );

    expect(steps).toEqual([
      { id: "voice", state: "current", blocked: false, reason: null },
      { id: "booking", state: "done", blocked: false, reason: null },
    ]);
  });
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function serve(answers: Readonly<Record<string, () => Response>>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      const route = Object.keys(answers).find((end) => path.endsWith(end));
      return route ? answers[route]() : jsonResponse({}, 404);
    }),
  );
}

function wrapper({ children }: Readonly<{ children: ReactNode }>) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

const REACH = settingsReach(meFixture({ allow: VOICE_WRITER }));

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

describe("useSetupChecklist", () => {
  it("settles once every read answers, and drops the step a refusal feeds", async () => {
    serve({
      "/connectors": () =>
        jsonResponse({
          data: [connection("gmail")],
          providers: MAIL_PROVIDERS.map((provider) => ({
            provider,
            reason: "ready",
          })),
        }),
      "/voice-profiles": () =>
        jsonResponse(
          { status: 403, code: "permission_denied", title: "Forbidden" },
          403,
        ),
      "/scheduling/profile": () => jsonResponse(scheduling(false)),
    });

    const { result } = renderHook(() => useSetupChecklist(REACH), { wrapper });
    expect(result.current.settled).toBe(false);

    await waitFor(() => expect(result.current.settled).toBe(true));
    expect(result.current.steps.map((step) => step.id)).toEqual([
      "mail",
      "calendar",
      "booking",
    ]);
    expect([result.current.done, result.current.total]).toEqual([1, 3]);
  });

  it("hides the reminder until the seat is forgotten at sign-out", async () => {
    serve({
      "/connectors": () => jsonResponse({ data: [], providers: [] }),
      "/voice-profiles": () => jsonResponse({ data: [] }),
      "/scheduling/profile": () => jsonResponse(scheduling(false)),
    });

    const first = renderHook(() => useSetupChecklist(REACH), { wrapper });
    expect(first.result.current.hidden).toBe(false);
    act(() => first.result.current.hide());
    expect(first.result.current.hidden).toBe(true);
    first.unmount();

    const reopened = renderHook(() => useSetupChecklist(REACH), { wrapper });
    expect(reopened.result.current.hidden).toBe(true);
    reopened.unmount();

    forgetSeat();
    const signedInAgain = renderHook(() => useSetupChecklist(REACH), {
      wrapper,
    });
    expect(signedInAgain.result.current.hidden).toBe(false);
  });
});

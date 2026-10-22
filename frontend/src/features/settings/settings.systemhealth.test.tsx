/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { formatDateTime } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { SystemHealthPage } from "./settings.systemhealth";

const OPERATOR: GrantSpec = {
  job_health: ["read"],
  ai_diagnostics: ["read"],
  embedding_reindex: ["read", "update"],
  installation_settings: ["read", "update"],
};

const CLEAN: Record<string, unknown> = {
  "/me": meFixture({ roles: ["admin"], allow: OPERATOR }),
  "/embeddings/reindex/status": {
    status: "idle",
    reindex_needed: false,
    entities_pending: 0,
    configured_identity: "embed-v1",
    populated_identity: "embed-v1",
    updated_at: "2026-10-14T09:00:00Z",
  },
  "/admin/job-health": {
    generated_at: "2026-10-14T09:31:00Z",
    dead_window_hours: 24,
    kinds: [],
    recent_failures: [],
  },
  "/admin/extension-ingest-health": {
    generated_at: "2026-10-14T09:30:00Z",
    window_days: 7,
    units: [],
  },
  "/ai/provider-health": { providers: [], configured: ["anthropic"] },
  "/admin/capture-health": {
    generated_at: "2026-10-14T09:32:00Z",
    mailboxes: [],
    classifier: { pending: 0, unsure: 0, exhausted: 0 },
    held_meetings: { count: 0 },
    sweeps: [],
  },
  "/admin/automatic-work-health": {
    generated_at: "2026-10-14T09:33:00Z",
    days: 7,
    first_day: "2026-10-08",
    last_day: "2026-10-14",
    share_threshold: 0.1,
    small_sample_below: 10,
    kinds: [],
    colleagues: [],
    unattributed: 0,
  },
  "/admin/recovery-health": {
    generated_at: "2026-10-14T09:33:00Z",
    recovery_target_seconds: 14_400,
    data_loss_target_seconds: 3_600,
    last_drill: null,
  },
  "/installation/settings": { operations: defaultOperations },
};

function stubRoutes(overrides: Record<string, unknown> = {}) {
  const routes = { ...CLEAN, ...overrides };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test.local",
      );
      const path = url.pathname.replace(/^\/v1/, "");
      return jsonResponse(routes[path] ?? {});
    }),
  );
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-14T10:00:00Z"));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

const strip = () => screen.findByRole("region", { name: "Status" });

it("answers each clean reading in its slot and draws no card for it", async () => {
  stubRoutes();
  render(<SystemHealthPage />);

  const row = await strip();
  expect(await within(row).findByText("Up to date")).toBeInTheDocument();
  expect(await within(row).findByText("Idle")).toBeInTheDocument();
  expect(await within(row).findByText("None")).toBeInTheDocument();
  expect(await within(row).findByText("Answering")).toBeInTheDocument();
  for (const title of [
    "Search index",
    "Background jobs",
    "Refused connector records",
    "AI provider status",
  ]) {
    expect(screen.queryByRole("heading", { name: title })).toBeNull();
  }
  // The reports without a slot keep their cards.
  expect(
    screen.getByRole("heading", { name: "Mail capture checks" }),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("heading", { name: "Restore drills" }),
  ).toBeInTheDocument();
});

it("dates the page once, by the stalest report it read", async () => {
  stubRoutes();
  render(<SystemHealthPage />);

  await strip();
  const stalest = formatDateTime("2026-10-14T09:30:00Z", "en", viewerZone());
  expect(await screen.findByText(`As of ${stalest}`)).toBeInTheDocument();
  expect(screen.getAllByText(/^As of /)).toHaveLength(1);
});

it("dates a report that carries no stamp by when it was read", async () => {
  vi.setSystemTime(new Date("2026-10-14T09:00:00Z"));
  stubRoutes();
  render(<SystemHealthPage />);

  await strip();
  const read = formatDateTime("2026-10-14T09:00:00Z", "en", viewerZone());
  expect(await screen.findByText(`As of ${read}`)).toBeInTheDocument();
});

it("says a malformed provider report could not be checked, and keeps the page", async () => {
  stubRoutes({ "/ai/provider-health": { providers: null } });
  render(<SystemHealthPage />);

  const row = await strip();
  expect(await within(row).findByText("Could not check")).toBeInTheDocument();
  expect(within(row).getByText("Up to date")).toBeInTheDocument();
  expect(
    await screen.findByRole("region", { name: "AI provider status" }),
  ).toBeInTheDocument();
});

it("brings a reading's card back under the row once it finds something", async () => {
  stubRoutes({
    "/admin/job-health": {
      generated_at: "2026-10-14T09:31:00Z",
      dead_window_hours: 24,
      kinds: [
        {
          kind: "webhook_retry",
          queue: "default",
          fleet_wide: false,
          waiting: 0,
          running: 0,
          retrying: 0,
          dead: 2,
          dead_recent: 2,
          oldest_waiting_age_seconds: null,
        },
      ],
      recent_failures: [],
    },
  });
  render(<SystemHealthPage />);

  expect(await within(await strip()).findByText("2 dead")).toBeInTheDocument();
  expect(
    await screen.findByRole("heading", { name: "Background jobs" }),
  ).toBeInTheDocument();
});

it("gives a withheld report no slot and keeps its card saying why", async () => {
  stubRoutes({
    "/me": meFixture({
      roles: ["ops"],
      allow: { embedding_reindex: ["read"], ai_diagnostics: ["read"] },
    }),
  });
  render(<SystemHealthPage />);

  const row = await strip();
  expect(await within(row).findByText("Up to date")).toBeInTheDocument();
  expect(within(row).queryByText("Background jobs")).toBeNull();
  expect(
    await screen.findByText(
      /background job health covers the whole installation/i,
    ),
  ).toBeInTheDocument();
  // Dated by the reads this reader holds, never by a withheld report.
  const read = formatDateTime("2026-10-14T10:00:00Z", "en", viewerZone());
  expect(await screen.findByText(`As of ${read}`)).toBeInTheDocument();
});

it("keeps a failed reading's card in place while it asks again", async () => {
  stubRoutes();
  const answered = vi.mocked(fetch).getMockImplementation();
  const intakeCalls = vi.fn();
  vi.mocked(fetch).mockImplementation(async (input, init) => {
    const url = input instanceof Request ? input.url : String(input);
    if (url.includes("/admin/extension-ingest-health")) {
      intakeCalls();
      return jsonResponse({ code: "internal", message: "Down." }, 500);
    }
    return answered ? answered(input, init) : jsonResponse({});
  });
  render(<SystemHealthPage />);

  expect(
    await within(await strip()).findByText("Could not check"),
  ).toBeInTheDocument();
  const card = await screen.findByRole("region", {
    name: "Refused connector records",
  });
  expect(
    await within(card).findByRole("button", { name: /retry/i }),
  ).toBeInTheDocument();
  // The card's mount asks once more. A card that hid during that read would
  // mount again and ask forever.
  await waitFor(() => expect(intakeCalls).toHaveBeenCalledTimes(2));
  await act(async () => {});
  expect(intakeCalls).toHaveBeenCalledTimes(2);
});

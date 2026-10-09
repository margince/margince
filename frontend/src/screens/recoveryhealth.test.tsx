/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { RecoveryHealthCard } from "./recoveryhealth";
import { jsonResponse, render } from "./settings.testkit";

function stubRoutes(overrides: Record<string, () => Response> = {}) {
  const sent: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      sent.push(key);
      const override = overrides[key];
      if (override) return override();
      if (key === "GET /admin/recovery-health") return jsonResponse(HEALTH);
      if (key === "GET /me")
        return jsonResponse(
          meFixture({ roles: ["admin"], allow: { job_health: ["read"] } }),
        );
      return jsonResponse({});
    }),
  );
  return sent;
}

const TARGETS = {
  generated_at: "2026-10-09T09:30:00Z",
  recovery_target_seconds: 14_400,
  data_loss_target_seconds: 3_600,
};

const HEALTH = {
  ...TARGETS,
  last_drill: {
    started_at: "2026-10-01T08:00:00Z",
    finished_at: "2026-10-01T10:15:00Z",
    restored_to: "2026-10-01T07:20:00Z",
    outcome: "passed",
    operator: "Ops On Call",
    notes: "restored copy opened; counts matched",
    recovery_seconds: 8_100,
    data_loss_seconds: 2_400,
  },
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("shows the last drill's measured windows against the published targets", async () => {
  stubRoutes();

  render(<RecoveryHealthCard />);

  expect(await screen.findByText("Passed")).toBeInTheDocument();
  expect(screen.getByText("2 h 15 min")).toBeInTheDocument();
  expect(screen.getByText("within the 4 h target")).toBeInTheDocument();
  expect(screen.getByText("0 h 40 min")).toBeInTheDocument();
  expect(screen.getByText("within the 1 h target")).toBeInTheDocument();
  expect(screen.getByText("Ops On Call")).toBeInTheDocument();
});

it("says a drill over the target is over it", async () => {
  stubRoutes({
    "GET /admin/recovery-health": () =>
      jsonResponse({
        ...HEALTH,
        last_drill: { ...HEALTH.last_drill, recovery_seconds: 18_000 },
      }),
  });

  render(<RecoveryHealthCard />);

  expect(await screen.findByText("over the 4 h target")).toBeInTheDocument();
});

it("says outright that no drill was ever recorded", async () => {
  stubRoutes({
    "GET /admin/recovery-health": () =>
      jsonResponse({ ...TARGETS, last_drill: null }),
  });

  render(<RecoveryHealthCard />);

  expect(
    await screen.findByText(/No restore drill has been recorded/),
  ).toBeInTheDocument();
  // Backups are not something Margince can see, and the card says so rather
  // than inventing a time.
  expect(screen.getByText("Not observed by Margince")).toBeInTheDocument();
});

it("reads a report with no last_drill field as an error, not as never drilled", async () => {
  stubRoutes({
    "GET /admin/recovery-health": () => jsonResponse(TARGETS),
  });

  render(<RecoveryHealthCard />);

  expect(await screen.findByRole("alert")).toBeInTheDocument();
  expect(
    screen.queryByText(/No restore drill has been recorded/),
  ).not.toBeInTheDocument();
});

it("withholds the card from a seat without the grant, and asks the server nothing", async () => {
  const sent = stubRoutes({
    "GET /me": () => jsonResponse(meFixture({ roles: ["rep"] })),
  });

  render(<RecoveryHealthCard />);

  expect(
    await screen.findByText(/need a permission your role does not have/),
  ).toBeInTheDocument();
  expect(sent.includes("GET /admin/recovery-health")).toBe(false);
});

it.each([
  ["no data-loss target", { ...HEALTH, data_loss_target_seconds: undefined }],
  [
    "a drill without its data-loss window",
    {
      ...HEALTH,
      last_drill: { ...HEALTH.last_drill, data_loss_seconds: null },
    },
  ],
  [
    "a drill with no restore point",
    { ...HEALTH, last_drill: { ...HEALTH.last_drill, restored_to: undefined } },
  ],
  [
    "an outcome the card has no words for",
    { ...HEALTH, last_drill: { ...HEALTH.last_drill, outcome: "skipped" } },
  ],
])("shows the error state for %s instead of drawing NaN", async (_, body) => {
  stubRoutes({ "GET /admin/recovery-health": () => jsonResponse(body) });

  render(<RecoveryHealthCard />);

  expect(await screen.findByRole("alert")).toBeInTheDocument();
  expect(screen.queryByText(/NaN/)).not.toBeInTheDocument();
});

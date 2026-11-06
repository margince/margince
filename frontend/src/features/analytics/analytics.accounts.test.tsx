// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
} from "../../testing/story-utils";
import { AnalyticsScreen } from "./analytics";
import type { AccountCoverage } from "./analytics.accounts";
import { render } from "./analytics.testkit";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;

afterEach(() => {
  answered = "single_threaded";
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  window.location.hash = "";
});

const DEAL = "22222222-2222-4222-8222-222222222222";
const WON = "33333333-3333-4333-8333-333333333333";
const COMPANY = "44444444-4444-4444-8444-444444444444";

function coverage(view: AccountCoverage["view"]): AccountCoverage {
  const base = {
    scope: { kind: "workspace" as const, label: "Whole company" },
    as_of: "2026-11-03T09:00:00Z",
    thresholds: {
      max_engaged_seats: 1,
      untouched_days: 21,
      silent_days: 30,
      won_within_days: 180,
    },
    framework_revision: 0,
    counts: { single_threaded: 1, untouched: 2, silent: 1 },
    activity_readable: true,
    view,
    withheld: 1,
  };
  if (view === "single_threaded") {
    return {
      ...base,
      rows: [
        {
          deal: {
            id: DEAL,
            name: "Tern portal",
            amount_minor: 12000000,
            currency: "EUR",
            stage: "Proposal",
          },
          company: { id: COMPANY, name: "Tern Digital" },
          engaged_seats: 0,
          named_seats: 1,
        },
      ],
    };
  }
  return {
    ...base,
    rows: [
      {
        company: { id: COMPANY, name: "Acme" },
        won_deal: {
          id: WON,
          name: "Acme rollout",
          closed_at: "2026-10-24T10:00:00Z",
        },
        last_outbound: "2026-10-20T10:00:00Z",
        days_silent: 10,
      },
    ],
  };
}

// The view the stub answers: the screen asks per view, and the test moves
// this before it clicks.
let answered: AccountCoverage["view"] = "single_threaded";

function routes(): RouteMap {
  return {
    ...reportingStoryRoutes(),
    "GET /me": meRoute({
      report_definition: ["read"],
      reporting_framework: ["read"],
      deal: ["read"],
      company: ["read"],
      forecast: ["read"],
      pipeline: ["read"],
    }),
    "GET /analytics/account-coverage": () => jsonResponse(coverage(answered)),
  };
}

it("lists the single-contact deals, prints the thresholds and opens the facts", async () => {
  const user = userEvent.setup({ delay: null });
  window.location.hash = "#/analytics/accounts";
  installFetchStub(routes());
  render(<AnalyticsScreen />);

  expect(await screen.findByText("Tern portal")).toBeInTheDocument();
  expect(
    screen.getByText(/At risk means: at most 1 engaged/),
  ).toBeInTheDocument();
  expect(
    screen.getByText(
      "1 more deal has contacts you may not see, so it is not judged.",
    ),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: /Won but silent/ }),
  ).toBeInTheDocument();

  await user.click(screen.getByText("Tern portal"));
  const drawer = await screen.findByRole("dialog");
  expect(
    within(drawer).getByText(
      "0 of 1 contacts named on the deal exchanged mail or met with you in the last 90 days.",
    ),
  ).toBeInTheDocument();
  expect(
    within(drawer).getByText("The deal is in stage Proposal."),
  ).toBeInTheDocument();
});

it("switches to the silent view and shows the won deal and its silence", async () => {
  const user = userEvent.setup({ delay: null });
  window.location.hash = "#/analytics/accounts";
  installFetchStub(routes());
  render(<AnalyticsScreen />);
  await screen.findByText("Tern portal");

  answered = "silent";
  await user.click(screen.getByRole("button", { name: /Won but silent/ }));
  expect(await screen.findByText("Acme rollout")).toBeInTheDocument();
  expect(screen.getByText(/10 days ago/)).toBeInTheDocument();
});

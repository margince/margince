// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ForecastReview } from "./analytics.forecast.review";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// What should be checked before the forecast call: the coverage line first and
// apart, then a table of findings in the server's order, severity first. The
// money at stake is a figure in a column, so it keeps tabular digits and lines
// up down the table in the body face.
//
// Read both frames in BOTH themes with the toolbar's Theme control.

type Assurance = components["schemas"]["ForecastAssurance"];
type InputCheck = components["schemas"]["InputCheck"];

const RUN: Assurance = {
  run_id: "33333333-3333-4333-8333-333333333333",
  as_of: "2026-07-14T06:00:00Z",
  status: "complete",
  readiness: "needs_review",
  eligible_deals: 52,
  sources: [
    { source: "mail", state: "checked" },
    { source: "offers", state: "checked" },
    { source: "calendar", state: "not_connected" },
  ],
};

function check(over: Partial<InputCheck>): InputCheck {
  return {
    id: "44444444-4444-4444-8444-444444444441",
    type: "close_past",
    subject_kind: "deal",
    subject_id: "55555555-5555-4555-8555-555555555551",
    subject: {
      type: "deal",
      id: "55555555-5555-4555-8555-555555555551",
      label: "Nordwind renewal",
    },
    severity: "high",
    affected_minor: 48_500_00,
    currency: "EUR",
    status: "open",
    first_seen_at: "2026-07-12T06:00:00Z",
    last_seen_at: "2026-07-14T06:00:00Z",
    ...over,
  };
}

const FINDINGS: InputCheck[] = [
  check({}),
  check({
    id: "44444444-4444-4444-8444-444444444442",
    type: "amount_vs_offer",
    subject_id: "55555555-5555-4555-8555-555555555552",
    subject: {
      type: "deal",
      id: "55555555-5555-4555-8555-555555555552",
      label: "Harbor Logistics expansion",
    },
    severity: "medium",
    affected_minor: 7_200_00,
  }),
  check({
    id: "44444444-4444-4444-8444-444444444443",
    type: "commit_unpriced",
    subject_id: "55555555-5555-4555-8555-555555555553",
    subject: {
      type: "deal",
      id: "55555555-5555-4555-8555-555555555553",
      label: "Alder pilot",
    },
    severity: "low",
    affected_minor: undefined,
    currency: undefined,
  }),
];

function routes(
  run: Assurance | null,
  findings: readonly InputCheck[],
): RouteMap {
  return {
    "GET /me": meRoute({}),
    "GET /forecast/assurance": () =>
      run === null ? jsonResponse({}, 404) : jsonResponse(run),
    "GET /forecast/assurance/exceptions": () =>
      jsonResponse({
        data: findings,
        page: { next_cursor: null, has_more: false },
      }),
  };
}

const meta: Meta<typeof ForecastReview> = {
  title: "Records/Forecast section/Review",
  component: ForecastReview,
};
export default meta;

type Story = StoryObj<typeof ForecastReview>;

// Three findings, one per severity and one unpriced: the amounts line up down
// the column, and the unpriced one says so rather than drawing a zero.
export const FindingsToCheck: Story = {
  render: () => {
    installFetchStub(routes(RUN, FINDINGS));
    return (
      <StoryProviders>
        <ForecastReview />
      </StoryProviders>
    );
  },
};

// A run that found nothing and could read every source.
export const NothingToCheck: Story = {
  render: () => {
    installFetchStub(
      routes(
        {
          ...RUN,
          readiness: "ready",
          sources: [
            { source: "mail", state: "checked" },
            { source: "offers", state: "checked" },
            { source: "calendar", state: "checked" },
          ],
        },
        [],
      ),
    );
    return (
      <StoryProviders>
        <ForecastReview />
      </StoryProviders>
    );
  },
};

// No run has completed: the endpoint's 404 is its answer, and the panel says
// the pipeline has not been checked rather than calling it clean.
export const NotCheckedYet: Story = {
  render: () => {
    installFetchStub(routes(null, []));
    return (
      <StoryProviders>
        <ForecastReview />
      </StoryProviders>
    );
  },
};

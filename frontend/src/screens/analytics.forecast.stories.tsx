// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { viewerZone } from "../format/timezone";
import type { AnalyticsSelection } from "./analytics.context";
import { ForecastView } from "./analytics.forecast";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The forecast section, whole: the period control, the screen's own answer, the
// three readings and the receipt they were drawn from.
//
// The answer is a PANEL and not a notice — the question is its title and the
// sentence is its body — because it is the screen's content rather than a
// remark about the screen. The frames are the two shapes above the readings: an
// answer on its own, and an answer with the unpriced-pipeline caveat under it,
// which is the one notice this section draws and the only thing between the
// answer and the tiles.
//
// Each of the three readings says what it RESTS ON under its figure — when the
// call was made and how far it sits over the evidence, what the evidence is,
// what "already won" counts — because a figure with nothing under it is a
// number a reader has to trust. The money is compact: a slot is about a hundred
// points wide and a full amount clips there, while the sentence above keeps the
// amount as it was authored.
//
// Read both frames in BOTH themes with the toolbar's Theme control.

type Readings = components["schemas"]["ForecastReadings"];

const SELECTION: AnalyticsSelection = {
  scope: { kind: "workspace", label: "Whole workspace" },
};

function readings(over: Partial<Readings> = {}): Readings {
  return {
    period_start: "2026-07-01",
    period_end: "2026-09-30",
    scope_kind: "workspace",
    won_minor: 92_000_00,
    evidence_minor: 148_000_00,
    best_case_minor: 260_000_00,
    open_minor: 240_000_00,
    weighted_minor: 132_000_00,
    eligible_count: 52,
    priced_count: 52,
    confirmed_date_count: 31,
    fx_missing_count: 0,
    as_of: "2026-07-14T06:00:00Z",
    // The zone the period's days were cut in. Nothing this section draws reads
    // it, and the reader's own is what the field would carry on a real
    // installation — so the fixture names no zone of its own.
    timezone: viewerZone(),
    base_currency: "EUR",
    current_call: {
      id: "22222222-2222-4222-8222-222222222222",
      amount_minor: 200_000_00,
      currency: "EUR",
      scope_kind: "workspace",
      period_start: "2026-07-01",
      period_end: "2026-09-30",
      author_id: "11111111-1111-4111-8111-111111111111",
      created_at: "2026-07-10T09:00:00Z",
    },
    ...over,
  };
}

// The section's own two reads, plus the session probe every screen makes. The
// assurance read answers 404 — "nothing has looked yet" is that endpoint's
// answer rather than its failure, and it is the ordinary state of a story.
function routes(data: Readings): RouteMap {
  return {
    "GET /me": meRoute({}),
    "GET /forecast": () => jsonResponse(data),
    "GET /forecast/assurance": () => jsonResponse({}, 404),
  };
}

const meta: Meta<typeof ForecastView> = {
  title: "Records/Forecast section",
  component: ForecastView,
};
export default meta;

type Story = StoryObj<typeof ForecastView>;

// Every eligible deal carries an amount, so the answer stands alone.
export const EveryDealPriced: Story = {
  render: () => {
    installFetchStub(routes(readings()));
    return (
      <StoryProviders>
        <ForecastView selection={SELECTION} canSubmit={false} />
      </StoryProviders>
    );
  },
};

// Eleven of the fifty-two carry no amount. The caveat is the frame's point:
// they are real deals contributing zero money to every figure below them, said
// beside the total rather than left in the receipt for somebody to find.
export const SomeDealsUnpriced: Story = {
  render: () => {
    installFetchStub(
      routes(readings({ eligible_count: 52, priced_count: 41 })),
    );
    return (
      <StoryProviders>
        <ForecastView selection={SELECTION} canSubmit={false} />
      </StoryProviders>
    );
  },
};

// Recording what somebody believes will close, opened. The editor is a panel of
// its own: the lead sentence says a call moves no deal, the two fields are its
// body, and cancel and save stand in the action band under them.
export const RecordingACall: Story = {
  render: () => {
    installFetchStub(routes(readings()));
    return (
      <StoryProviders>
        <ForecastView selection={SELECTION} canSubmit />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Update the current call",
      }),
    );
  },
};

// Nobody has called the period. The call slot answers in WORDS rather than with
// a glyph — a slot in a row compared across must not answer with a dash — and
// carries no detail, because there is no call to say anything about.
export const NobodyHasCalled: Story = {
  render: () => {
    installFetchStub(routes(readings({ current_call: undefined })));
    return (
      <StoryProviders>
        <ForecastView selection={SELECTION} canSubmit={false} />
      </StoryProviders>
    );
  },
};

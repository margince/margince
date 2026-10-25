// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import type { components } from "../../../api/schema";
import { en } from "../../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealFigures, DealTimeline } from "./dealfigures";
import type { DealStandingInputs } from "./dealstanding";

const meta = {
  title: "Records/Deal 360/Figures",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;
type Activity = components["schemas"]["Activity"];
type Stage = components["schemas"]["Stage"];

const DEAL_ID = "01a03000-0000-7000-8000-000000000002";

const STAGES: Stage[] = ["Qualification", "Proposal sent", "Negotiation"].map(
  (name, index) => ({
    id: `s${index + 1}`,
    pipeline_id: "pl",
    name,
    position: index + 1,
    semantic: "open",
    win_probability: 20 + index * 20,
  }),
);

function activity(
  row: Pick<Activity, "id" | "kind" | "occurred_at"> & Partial<Activity>,
): Activity {
  return {
    occurred_precision: "instant",
    is_done: false,
    source: "manual",
    captured_by: "human:u1",
    created_at: row.occurred_at,
    updated_at: row.occurred_at,
    ...row,
  };
}

// Read in the afternoon, so a day-only row's stored noon is already past.
const AS_OF = "2026-08-13T15:00:00Z";

function inputs(activities: Activity[]): DealStandingInputs {
  return {
    deal: {
      id: DEAL_ID,
      name: "Fleet retrofit",
      amount_minor: 4_800_000,
      currency: "EUR",
      pipeline_id: "pl",
      stage_id: "s2",
      status: "open",
      stalled: false,
      expected_close_date: "2026-09-30",
      source: "manual",
      captured_by: "human:u1",
      created_at: "2026-06-02T09:00:00Z",
      updated_at: "2026-08-01T10:00:00Z",
    },
    stages: STAGES,
    asOf: AS_OF,
    activities,
    offers: [],
    coverage: { withheld: false, pending: false },
    ladder: { advancing: false, advanceRefused: false, onAdvance: () => {} },
  };
}

function render(read: DealStandingInputs) {
  return () => {
    installFetchStub({
      "GET /field-history": () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <DealFigures inputs={read} />
        <DealTimeline inputs={read} onOpenHistory={() => undefined} />
      </StoryProviders>
    );
  };
}

export const Booked: Story = {
  render: render(
    inputs([
      activity({
        id: "a-walk",
        kind: "meeting",
        meeting_status: "booked",
        subject: "Walkthrough",
        occurred_at: "2026-08-20T09:00:00Z",
      }),
    ]),
  ),
};

// A meeting booked for today by its day alone stays the next step all day.
// A visit logged the same way is on the line, named by its day.
export const DayOnlyToday: Story = {
  render: render(
    inputs([
      activity({
        id: "a-walk",
        kind: "meeting",
        meeting_status: "booked",
        subject: "Walkthrough",
        occurred_at: "2026-08-13T12:00:00Z",
        occurred_precision: "day",
      }),
      activity({
        id: "a-visit",
        kind: "note",
        subject: "Site visit",
        occurred_at: "2026-08-13T12:00:00Z",
        occurred_precision: "day",
        is_done: true,
      }),
    ]),
  ),
  play: async () => {
    const next = (
      await screen.findByText(en["deal.standing.figure.next"])
    ).closest("section");
    await expect(next?.textContent).toMatch(/Walkthrough/);
    await expect(next?.textContent).not.toMatch(en["standing.noneYet"]);
    await userEvent.click(
      await screen.findByRole("button", { name: en["timeline.kind.note"] }),
    );
    const row = (await screen.findByText("Site visit")).parentElement;
    await expect(row?.textContent).not.toMatch(/\d{1,2}:\d{2}/);
  },
};

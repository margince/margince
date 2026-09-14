// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { installFetchStub, StoryProviders } from "../story-utils";
import { briefModelPlan, briefWithPlan } from "./fixtures";
import {
  AccountArc,
  AdvancePanel,
  LikelyAsks,
  ObjectivePanel,
  Scenarios,
  TopRisk,
  Unknowns,
} from "./plan";
import "./meetingbrief.css";

// The preparation plan's seven panels, out of the drawer — what to DO in the
// room, rather than what the record says. Rendered inline for the reason the
// sections' frames are: the connected drawer captures the whole sheet, and the
// panels are what changed.
//
// The pair below is the claim worth checking. A plan the model wrote is indigo
// on all seven, with the disclosure said once on the objective; the
// deterministic plan keeps the accent on the two cards that ASK for a move and
// leaves the other five untinted. Five tinted panels would be five leads, which
// is no lead — and an indigo one would say a model weighed a meeting nothing
// weighed.

type MeetingBrief = components["schemas"]["MeetingBrief"];
type MeetingPlan = NonNullable<MeetingBrief["plan"]>;

// Read through a function rather than a cast: a fixture that stops carrying a
// plan fails here instead of quietly rendering an empty frame.
function planOf(brief: MeetingBrief): MeetingPlan {
  const { plan } = brief;
  if (!plan) {
    throw new Error("this fixture carries no preparation plan");
  }
  return plan;
}

const meta: Meta<typeof ObjectivePanel> = {
  title: "Records/Contact record/Meeting brief/Plan",
  component: ObjectivePanel,
};
export default meta;
type Story = StoryObj<typeof ObjectivePanel>;

function plan(brief: MeetingBrief) {
  return () => {
    // The panels take their plan as a prop and fetch nothing; the stub keeps a
    // stray read from leaving the iframe.
    installFetchStub({});
    const it = planOf(brief);
    return (
      <StoryProviders>
        <div className="mb-stack" style={{ maxWidth: 665 }}>
          <ObjectivePanel plan={it} onOpenRecord={() => {}} />
          <TopRisk plan={it} onOpenRecord={() => {}} />
          <LikelyAsks plan={it} onOpenRecord={() => {}} />
          <Scenarios plan={it} />
          <AccountArc
            plan={it}
            onOpenRecord={() => {}}
            // A fixed day, never a formatted clock: `make fe-clock-drift` runs
            // the suite at +200 days and requires the same verdict.
            formatDay={(iso) => iso.slice(0, 10)}
          />
          <AdvancePanel plan={it} onOpenRecord={() => {}} />
          <Unknowns plan={it} />
        </div>
      </StoryProviders>
    );
  };
}

/** The deterministic plan: the objective and the close in the accent, the arc,
 *  the asks, the branches and the gaps plain. */
export const Composed: Story = { render: plan(briefWithPlan) };

/** The model's own plan: indigo throughout, disclosed once on the objective. */
export const ModelWritten: Story = { render: plan(briefModelPlan) };

/** The model's plan in the dark theme — the indigo head band and the opener's
 *  recessed quotation are two elevations a darker palette compresses. */
export const ModelWrittenDark: Story = {
  globals: { theme: "dark" },
  render: plan(briefModelPlan),
};

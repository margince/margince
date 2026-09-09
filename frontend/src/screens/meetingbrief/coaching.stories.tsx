// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { installFetchStub, StoryProviders } from "../story-utils";
import { CoachPanel, MeetingPaths } from "./coaching";
import { briefManager } from "./fixtures";
import "./meetingbrief.css";

// The coaching layer, out of the drawer: what a LEAD reads over a teammate's
// meeting. Its presence is the server's answer and never a client's question,
// so there is no "off" frame to draw — a rep simply receives no layer.
//
// The two panels are ONE reading and take one tint between them. The frames
// below are the pair: written by a model, both are indigo; composed, both are
// plain. A lead meeting one tinted card beside an untinted one would take the
// two for two different readings, which is the defect these exist to hold.

type MeetingBrief = components["schemas"]["MeetingBrief"];
type Coaching = NonNullable<
  NonNullable<MeetingBrief["plan"]>["manager_coaching"]
>;

// Read through a function rather than a cast: a fixture that stops carrying the
// layer fails here instead of quietly rendering nothing.
function coachingOf(brief: MeetingBrief): Coaching {
  const layer = brief.plan?.manager_coaching;
  if (!layer) {
    throw new Error("this fixture carries no coaching layer");
  }
  return layer;
}

const coaching = coachingOf(briefManager);

const meta: Meta<typeof CoachPanel> = {
  title: "Records/Person record/Meeting brief/Coaching",
  component: CoachPanel,
};
export default meta;
type Story = StoryObj<typeof CoachPanel>;

function layer(writtenByModel: boolean) {
  return () => {
    // Both panels take the layer as a prop and fetch nothing; the stub keeps a
    // stray read from leaving the iframe.
    installFetchStub({});
    return (
      <StoryProviders>
        <div className="mb-stack" style={{ maxWidth: 665 }}>
          <CoachPanel coaching={coaching} writtenByModel={writtenByModel} />
          <MeetingPaths coaching={coaching} writtenByModel={writtenByModel} />
        </div>
      </StoryProviders>
    );
  };
}

/** The model's coaching: both panels indigo, and the head still says which view
 *  a lead is in rather than who wrote it. */
export const ModelWritten: Story = { render: layer(true) };

/** The same layer composed over the records: the lead takes the accent and the
 *  branches beside it take nothing. */
export const Composed: Story = { render: layer(false) };

/** The model's coaching in the dark theme, where the indigo band and the
 *  panel's own ground sit closest together. */
export const ModelWrittenDark: Story = {
  globals: { theme: "dark" },
  render: layer(true),
};

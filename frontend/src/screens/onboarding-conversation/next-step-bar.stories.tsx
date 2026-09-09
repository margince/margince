// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../story-utils";
import { NextStepBar } from "./next-step-bar";
import "./conversation.css";

// The bar exists only while the step it names is scrolled out of view, so a
// story has to build that: a short thread with the card pushed below its fold.
// Scroll the thread down and the bar retires itself, which is the half of the
// contract a screenshot cannot show.

function Thread({ target }: Readonly<{ target: boolean }>) {
  return (
    <StoryProviders>
      <div className="ob-conv-thread" style={{ height: "8rem" }}>
        <p>Nothing here yet — the step is further down.</p>
        <div style={{ height: "16rem" }} />
        {target ? (
          <p id="story-next-step">The step the bar points at.</p>
        ) : null}
      </div>
      <NextStepBar
        label="1 decision open"
        targetSelector="#story-next-step"
        revision={0}
      />
    </StoryProviders>
  );
}

const meta: Meta<typeof Thread> = {
  title: "Onboarding/Next step bar",
  component: Thread,
};
export default meta;

type Story = StoryObj<typeof Thread>;

export const PointsAtTheStep: Story = {
  args: { target: true },
};

// Nothing to point at, so no bar: the line never stands there empty.
export const NothingToPointAt: Story = {
  args: { target: false },
};

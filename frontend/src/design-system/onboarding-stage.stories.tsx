// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button } from "./atoms";
import { OnboardingStage, StageNeeds } from "./onboarding-stage";

// The room, in the three states that are decisions rather than content: unlit
// because no model is bound yet, lit because one is, and top-anchored because
// what stands in it is going to grow while somebody reads it.
//
// Worth flipping the Theme control on each. The Core goes opaque on paper and
// emissive on a dark ground, and the room's two washes are alpha tints that
// have to read on both.
const meta: Meta<typeof OnboardingStage> = {
  title: "Onboarding/Stage",
  component: OnboardingStage,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof OnboardingStage>;

// The one screen in the product whose room is dark: nothing has been bound, so
// there is nothing an agent has done for the indigo to be about.
export const Unlit: Story = {
  args: {
    lit: false,
    coreStateLabel: "core · at rest",
    progress: { steps: ["The model", "Your platform"], at: 0 },
    eyebrow: "First run · 1 of 2",
    title: "Choose a model provider",
    sub: "Margince provides no inference of its own, so it works through your vendor account.",
    children: <Button variant="primary">Continue</Button>,
  },
};

// A model is bound. The room carries it from here on, and it means that and
// nothing else.
export const Lit: Story = {
  args: {
    lit: true,
    coreStateLabel: "core · at rest",
    progress: { steps: ["The model", "Your platform"], at: 1 },
    eyebrow: "First run · 2 of 2",
    title: "Connect a Google app",
    sub: "Mailboxes are connected through a Google OAuth app you own, so mail is read with your organization’s own credentials.",
    children: <Button variant="primary">Continue</Button>,
  },
};

// The read theatre's anchor. A surface that gains a tile per page cannot be
// centred: the column would re-centre on every arrival and carry the line
// somebody is reading upward while they read it.
export const GrowingAndTopAnchored: Story = {
  args: {
    lit: true,
    anchor: "start",
    coreState: "ingest",
    coreFeed: true,
    coreProgress: 0.4,
    // The orb steps back once the reader has work of their own to watch. Same
    // element, same place, less room.
    coreScale: "work",
    coreStateLabel: "core · taking it in",
    title: "Reading gradion.com",
    sub: "Following the pages that say what this company does.",
    children: <p className="t-body">Nine pages read, four still to reach.</p>,
  },
};

// No eyebrow and no band, because not every flow numbers itself — the gate does
// not, and inventing a step counter for it would be copy nobody wrote. The band
// disappears entirely rather than drawing an empty rule: a frame with nothing in
// it is chrome describing itself.
export const WithoutAnEyebrow: Story = {
  args: {
    lit: true,
    title: "Where does your company live on the web?",
    sub: "One address is enough. Everything after this starts from what is on it.",
    children: <Button variant="primary">Read the site</Button>,
  },
};

// The way onward pressed early. The button never greys; what is missing is
// said beside it, in the same words the fields carry.
export const PressedEarly: Story = {
  args: {
    lit: false,
    coreStateLabel: "core · at rest",
    progress: { steps: ["The model", "Your platform"], at: 0 },
    eyebrow: "First run · 1 of 2",
    title: "Choose a model provider",
    children: (
      <>
        <StageNeeds attempted missing={["API key", "Model"]} />
        <Button variant="primary">Continue</Button>
      </>
    ),
  },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { userEvent, waitFor, within } from "storybook/test";
import { Button } from "../design-system/atoms";
import { OnboardingStage } from "../design-system/onboarding-stage";
import { LocaleProvider } from "../i18n";
import { Ignition, useIgnitionCore } from "./onboarding-ignition";

// The four seconds after a model is bound, in the room they happen in — the
// wash comes from the orb and the light fades up under it, so the sequence
// cannot be reviewed without the stage around it.
//
// The two stories are the same sequence and differ only in when they are looked
// at: `Running` presses the button and hands back immediately, `Settled` waits
// for the last line, which is what a still frame should show. Flip the Theme
// control on both — the wash is `--orbGlow` on a radial, and it reads
// differently on paper than on a dark ground.
const meta: Meta<typeof Ignition> = {
  title: "Onboarding/Ignition",
  component: Ignition,
  parameters: { layout: "fullscreen" },
};
export default meta;

type Story = StoryObj<typeof Ignition>;

/**
 * The stage as the real screen assembles it, with a press to start the sequence
 * rather than a prop already set: the wash and the room's light are transitions,
 * and a story that mounts them already-true shows neither.
 */
function Scene() {
  const [ignited, setIgnited] = useState(false);
  const core = useIgnitionCore(ignited);
  return (
    <LocaleProvider initial="en">
      <OnboardingStage
        lit={ignited}
        coreState={ignited ? core.state : "idle"}
        coreProgress={core.progress}
        coreFlash={ignited}
        coreStateLabel={ignited ? "core · taking it in" : "core · at rest"}
        progress={{ steps: ["The model", "Your platform"], at: 0 }}
        eyebrow="First run · 1 of 2"
        title={ignited ? "It has a pulse." : "Choose a model provider"}
        sub={
          ignited
            ? "The key is sealed and the model answered. Here is what that changes."
            : "Margince provides no inference of its own, so it works through your vendor account."
        }
      >
        {ignited ? (
          <Ignition vendor="Google Gemini" onDone={() => setIgnited(false)} />
        ) : (
          <Button variant="primary" onClick={() => setIgnited(true)}>
            Give it a pulse
          </Button>
        )}
      </OnboardingStage>
    </LocaleProvider>
  );
}

// Pressed, and handed straight back: the frame lands early in the sequence, with
// the room still coming up. Useful for seeing where the wash starts from.
export const Running: Story = {
  render: () => <Scene />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Give it a pulse" }),
    );
  },
};

// The same sequence, waited out to its end.
//
// ITS CAPTURED FRAME IS STILL MID-SEQUENCE, and that is a fact about the render
// gate rather than about this story: `fe-uat` screenshots a play story 1.5s
// after mount, and this settles at about 4s. There is no fixing that from here
// and it should not be fixed by shortening the sequence — reviewing this one
// means watching it in Storybook, which is what the wait below makes reliable.
//
// What the wait IS good for: it asserts the timeline completes at all. It reads
// the last element's own opacity rather than sleeping, so the day a beat moves
// the check still knows what "finished" means.
export const Settled: Story = {
  render: () => <Scene />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Give it a pulse" }),
    );
    const carryOn = await canvas.findByRole("button", { name: "Carry on" });
    await waitFor(
      () => {
        const go = carryOn.closest(".ob-ig-go");
        if (go === null || getComputedStyle(go).opacity !== "1") {
          throw new Error("the ignition has not settled yet");
        }
      },
      { timeout: 8000 },
    );
  },
};

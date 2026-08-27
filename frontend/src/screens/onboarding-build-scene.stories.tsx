// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { BUILD_SCENE_DURATION_MS, BuildScene } from "./onboarding-build-scene";

const meta: Meta<typeof BuildScene> = {
  title: "Onboarding/Build scene",
  component: BuildScene,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof BuildScene>;

/**
 * The handoff into the app: the letters land, the ghost cards settle, and the
 * whole thing dissolves rather than cutting. The ghosts are bar widths only —
 * silhouettes of the app behind the wordmark, carrying no text, no data and no
 * promise about what the workspace contains.
 *
 * A long duration so the frame can actually be looked at; the product uses
 * `BUILD_SCENE_DURATION_MS`.
 */
export const Assembling: Story = {
  args: { durationMs: 60_000, onDone: () => undefined },
};

/**
 * The real beat, which ends by calling `onDone` and leaving. Watch it rather
 * than read it: the exit is a fraction of the SAME duration, so a caller that
 * shortens the scene shortens the dissolve with it instead of keeping a second
 * number in step.
 */
export const AtProductDuration: Story = {
  args: { durationMs: BUILD_SCENE_DURATION_MS, onDone: () => undefined },
};

/**
 * Under `prefers-reduced-motion` there is NO scene: the end state of a
 * decorative delay is being past it, so the callback fires at once and nothing
 * renders. Anything else would make the people who asked for less motion wait
 * longest. This frame is blank on purpose — set the OS reduced-motion
 * preference to see it.
 */
export const ReducedMotionRendersNothing: Story = {
  args: { durationMs: 60_000, onDone: () => undefined },
};

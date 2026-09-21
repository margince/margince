// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { Panel } from "../../design-system/panel";
import { StoryProviders } from "../story-utils";
import { ThreadFailed } from "./threadfailed";

// A THREAD THAT COULD NOT BE READ, in the place the thread would have been.
//
// The state this exists to prevent is the one next to it: a failed read handed
// the spine an empty page, and an empty spine draws exactly like a record nobody
// has ever written to. So the two frames worth looking at are this body and the
// spine's own `NothingWritten` — the same box, one saying "missing" and one
// saying "empty" — and the only thing separating them is this sentence and the
// verb under it.
//
// Framed in the `Panel` the record pages give the thread (`spine.stories.tsx`
// uses the same one), because the body is a `PanelBody`: on its own in the
// canvas it has no header band to sit under and no edge to be inset from, which
// is most of what tells a reader this is the call's own box rather than a
// notice floating on the page.
//
// The retry is the reader's whole answer here, so it is a `fn()` rather than a
// bare arrow: the action panel then shows that the press reached the refetch,
// which is the one thing this component wires.

const meta: Meta<typeof ThreadFailed> = {
  title: "Records/Company 360/Thread could not be read",
  component: ThreadFailed,
  parameters: { layout: "padded" },
  args: { onRetry: fn() },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div style={{ maxWidth: 720 }}>
          <Panel title="Company 360" tone="accent">
            <Story />
          </Panel>
        </div>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof ThreadFailed>;

/**
 * The read failed: the call above still stands, and under it the thread says it
 * is MISSING rather than empty, with the one move a reader has.
 */
export const TheThreadIsMissing: Story = {};

/**
 * The same body in dark, where the caption is the thing at risk.
 *
 * The sentence is `--textMeta` on the panel's own ground and the verb beside it
 * is a ghost — a hairline box with no fill — so nothing here carries a tint that
 * would survive the ground inverting on its own. Both steps are derived, and
 * this is the frame that says whether the sentence still reads as a statement
 * about the record rather than as disabled text.
 */
export const TheThreadIsMissingDark: Story = { globals: { theme: "dark" } };

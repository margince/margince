// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../screens/story-utils";
import { OverlayFallback } from "./overlayfallback";

// What stands where a whole 360 page would be when the workspace reads from an
// incumbent mirror. The read REFUSED to assemble rather than failing, so this
// is a statement and not an error — an error plate here would send a reader
// looking for a fault that does not exist.
const meta: Meta<typeof OverlayFallback> = {
  title: "Design System/Overlay fallback",
  component: OverlayFallback,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <div style={{ maxWidth: 720 }}>
          <Story />
        </div>
      </StoryProviders>
    ),
  ],
};
export default meta;

export const Surface: StoryObj<typeof OverlayFallback> = {};

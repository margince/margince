// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CarriageViolation } from "./carriage";
import { CarriageNotice } from "./composeattachments";
import { StoryProviders } from "./story-utils";

// The attachment shelf's standing refusal, rendered off the same violation
// shapes carriage.test.ts asserts rather than a sentence typed here — so the
// captured frame shows the wording the catalog actually ships.

// Every bound one message breaks at once, which is what the pre-check returns:
// it reports all of them because a rep fixing them one send at a time is the
// late-and-correct experience it exists to end.
const BLOCKS: readonly CarriageViolation[] = [
  { kind: "count", limit: 3, named: 5 },
  { kind: "perFile", filename: "offer-final.pdf", limit: 5_000_000 },
  { kind: "aggregate", total: 24_000_000, limit: 16_000_000, count: 5 },
];

const meta: Meta = {
  title: "Patterns/Compose attachments",
};
export default meta;

type Story = StoryObj;

// The warn callout as the shelf draws it: the claim in the heading, one broken
// bound per line under it. No live region — it is true as the shelf renders.
export const CarriageBlocked: Story = {
  render: () => (
    <StoryProviders>
      <CarriageNotice channel="WhatsApp" blocks={BLOCKS} />
    </StoryProviders>
  ),
};

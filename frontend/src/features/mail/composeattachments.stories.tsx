// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type { CarriageViolation } from "./carriage";
import { AttachAction, CarriageNotice } from "./composeattachments";

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
  title: "Patterns/Compose mail/Attachments",
};
export default meta;

type Story = StoryObj;

// The warning callout as the shelf draws it: the claim in the heading, one broken
// bound per line under it. No live region — it is true as the shelf renders.
export const CarriageBlocked: Story = {
  render: () => (
    <StoryProviders>
      <CarriageNotice channel="WhatsApp" blocks={BLOCKS} />
    </StoryProviders>
  ),
};

// A channel that carries no files at all, with one attachment: the singular.
export const CarriesNoFiles: Story = {
  render: () => (
    <StoryProviders>
      <CarriageNotice channel="SMS" blocks={[{ kind: "carries", count: 1 }]} />
    </StoryProviders>
  ),
};

// Ten files on the message: the picker takes no upload, since the record would
// keep a file the message cannot carry.
export const FullMessageTakesNoUpload: Story = {
  render: () => {
    installFetchStub({ "GET /attachments": () => jsonResponse(emptyPage) });
    const ten = Array.from({ length: 10 }, (_, at) => ({
      id: `f-${at}`,
      filename: `drawing-${at + 1}.pdf`,
    }));
    return (
      <StoryProviders>
        <AttachAction
          entityType="deal"
          entityId="deal-1"
          chosen={ten}
          onChange={() => {}}
        />
      </StoryProviders>
    );
  },
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: "Attach" }),
    );
    await screen.findByLabelText(/Upload file/);
  },
};

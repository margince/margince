// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../screens/story-utils";
import { CaptureChip } from "./capture-chip";

// The chip has one state — mail is arriving — and two ways of saying how far:
// as a share when the import was previewed, as a count when it was not. Each
// story is a connections answer, because that is the one read it draws from.
// It is mounted inside a column the height of the shell's content, positioned
// against it exactly as `.main` positions it, so the story reviews the same
// placement that ships.

function importing(scanned: number, estimated: number | null) {
  return () =>
    jsonResponse({
      data: [
        {
          id: "018f3a1b-0000-7000-8000-0000000000c1",
          provider: "gmail",
          status: "connected",
          scopes: [],
          account_label: "ada@acme.test",
          backfill: {
            state: "running",
            estimated_messages: estimated,
            counts: { messages_scanned: scanned },
          },
        },
      ],
    });
}

function story(connectors: () => Response) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /connectors": connectors,
    });
    return (
      <StoryProviders>
        <div
          className="main"
          style={{ position: "relative", minHeight: "40vh" }}
        >
          <CaptureChip route={{ screen: "deals" }} />
        </div>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CaptureChip> = {
  title: "Shell/Capture chip",
  component: CaptureChip,
};
export default meta;
type Story = StoryObj<typeof CaptureChip>;

/** Mid-import, with the share the preview gave it as the ring. */
export const Importing: Story = { render: story(importing(1_204, 2_900)) };

/** Just started: queued, nothing scanned, the ring empty rather than absent. */
export const Queued: Story = { render: story(importing(0, 2_900)) };

/** An import with no estimate counts what it has scanned and draws no ring. */
export const Unpreviewed: Story = { render: story(importing(37, null)) };

/** Nearly done: a full ring is the last thing drawn before the chip leaves. */
export const AlmostDone: Story = { render: story(importing(2_880, 2_900)) };

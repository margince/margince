// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LinkSuggestionsPanel } from "./worklist.linksuggestions";

// The review queue of links an extension suggests for a message. Nothing is
// filed until the reader accepts.

const queue = [
  {
    id: "01a0e9f1-0000-7000-8000-0000000000d1",
    activity_id: "01a0e9f1-0000-7000-8000-0000000000e1",
    activity_subject: "Which project is this?",
    entity_type: "deal",
    entity_id: "01a0e9f1-0000-7000-8000-0000000000f1",
    entity_name: "Renewal",
    unit: "finance-demo",
    reason: "The subject names this deal.",
    score: 0.8,
    status: "pending",
    created_at: "2026-10-08T09:00:00Z",
  },
];

function Served() {
  installFetchStub({
    "GET /me": meRoute({ activity: ["update"] } as never),
    "GET /link-suggestions": () => jsonResponse({ data: queue }),
  });
  return (
    <StoryProviders>
      <LinkSuggestionsPanel />
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Worklist/Suggested links",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** One suggestion with the extension's reason and the two answers. */
export const Pending: Story = {
  render: () => <Served />,
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("button", {
        name: "File it there",
      }),
    ).toBeInTheDocument();
  },
};

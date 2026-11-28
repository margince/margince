// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { vi } from "../../i18n/vi";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ReceiptNote } from "./worklist.receiptnote";

const id = "01a05500-0000-7000-8000-0000000000a1";
export default {
  title: "Components/AI and provenance/Receipt note",
  component: ReceiptNote,
} satisfies Meta<typeof ReceiptNote>;

export const VietnameseReader: StoryObj<typeof ReceiptNote> = {
  render: () => {
    installFetchStub({
      [`GET /activities/${id}`]: () =>
        jsonResponse({
          id,
          kind: "note",
          subject: "Risk assessment",
          body: "No reply in three weeks.",
          language: "en",
          version: 4,
          occurred_at: "2026-10-20T10:00:00Z",
          occurred_precision: "instant",
          links: [],
          source: "manual",
          is_done: false,
          offers_email: false,
          created_at: "2026-10-20T10:00:00Z",
          updated_at: "2026-10-20T10:00:00Z",
          captured_by: "agent:passport",
          recorded_by_agent: true,
          created_by: {
            actor_type: "agent",
            actor_id: "agent:passport",
            on_behalf_of_name: "Anna Weber",
          },
        }),
    });
    return (
      <StoryProviders locale="vi">
        <ReceiptNote activityId={id}>
          <a href="/#/activities/note">Risk assessment</a>
        </ReceiptNote>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByText(
        vi["trust.agentUnnamedFor"].replace("{name}", "Anna Weber"),
      ),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: vi["readerRendering.show"] }),
    ).toBeVisible();
  },
};

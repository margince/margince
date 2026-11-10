// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, within } from "storybook/test";
import { OpenSourceProvider } from "../lib/opensource";
import { ViewerZoneProvider } from "../lib/viewerzone";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import {
  INQUIRY_FIRST_SOURCE,
  INQUIRY_GRANTS,
} from "./websiteinquiry.fixtures";
import { WebsiteInquiryFacts } from "./websiteinquiryfacts";

const meta: Meta<typeof WebsiteInquiryFacts> = {
  title: "Worklist/Website inquiry facts",
  component: WebsiteInquiryFacts,
  decorators: [
    (Story) => {
      installFetchStub({ "GET /me": meRoute(INQUIRY_GRANTS) });
      return (
        <StoryProviders>
          <ViewerZoneProvider zone="Europe/Berlin">
            <OpenSourceProvider openSource={fn()}>
              <Story />
            </OpenSourceProvider>
          </ViewerZoneProvider>
        </StoryProviders>
      );
    },
  ],
};
export default meta;
type Story = StoryObj<typeof WebsiteInquiryFacts>;

export const AcceptedSubmission: Story = {
  args: {
    facts: {
      received_at: INQUIRY_FIRST_SOURCE.created_at,
      source_activity_id: INQUIRY_FIRST_SOURCE.id,
      source_label: "Website contact",
      request_type: "Quote",
      assignment_reason: "coordinator_review",
    },
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Request type: Quote")).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Open submission" }),
    ).toBeVisible();
  },
};
export const SourceUnavailable: Story = {
  args: { facts: {} },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(canvas.getByText("Receipt details unavailable")).toBeVisible();
    await expect(
      canvas.queryByRole("button", { name: "Open submission" }),
    ).toBeNull();
  },
};

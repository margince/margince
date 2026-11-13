// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import {
  INQUIRY_AUTOMATION,
  INQUIRY_COORDINATOR,
  INQUIRY_FIRST_SOURCE,
  INQUIRY_FORM,
  INQUIRY_GRANTS,
  INQUIRY_MEMBERS,
  INQUIRY_SECOND_SOURCE,
} from "./websiteinquiry.fixtures";
import { WebsiteInquiryExistingDialog } from "./websiteinquiryexisting";

const meta: Meta<typeof WebsiteInquiryExistingDialog> = {
  title: "Settings/Data/Website forms/Handle existing inquiries",
  component: WebsiteInquiryExistingDialog,
};
export default meta;
type Story = StoryObj<typeof WebsiteInquiryExistingDialog>;

function picker(failedPreview = false) {
  return () => {
    const base = `/web-forms/${INQUIRY_FORM.id}/handling`;
    installFetchStub({
      "GET /me": meRoute(INQUIRY_GRANTS),
      "GET /activities": () =>
        jsonResponse({
          data: [INQUIRY_FIRST_SOURCE, INQUIRY_SECOND_SOURCE],
          page: { has_more: false, next_cursor: null },
        }),
      "GET /users/names": () => jsonResponse({ data: INQUIRY_MEMBERS }),
      [`POST ${base}/preview`]: () =>
        failedPreview
          ? jsonResponse(
              {
                code: "permission_denied",
                status: 403,
                detail: "You cannot read these submissions.",
              },
              403,
            )
          : jsonResponse({
              received_at: INQUIRY_FIRST_SOURCE.created_at,
              example_due_at: "2026-09-02T21:59:59.999999Z",
              items: [
                {
                  id: INQUIRY_FIRST_SOURCE.id,
                  version: 3,
                  status: "eligible",
                  recipient_id: INQUIRY_COORDINATOR,
                  due_at: "2026-09-02T21:59:59.999999Z",
                  reason: "coordinator_review",
                },
                {
                  id: INQUIRY_SECOND_SOURCE.id,
                  version: 5,
                  status: "already_handled",
                },
              ],
            }),
      [`POST ${base}/existing`]: () =>
        jsonResponse({
          items: [
            { id: INQUIRY_FIRST_SOURCE.id, version: 4, status: "task_created" },
          ],
        }),
    });
    return (
      <StoryProviders>
        <WebsiteInquiryExistingDialog
          form={INQUIRY_FORM}
          automation={INQUIRY_AUTOMATION}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

export const ChooseSubmissions: Story = { render: picker() };
export const ReviewSelection: Story = {
  render: picker(),
  play: async () => {
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", {
        name: "Select A website question",
      }),
    );
    await user.click(
      screen.getByRole("checkbox", { name: "Select An older question" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Review selected inquiries" }),
    );
    await expect(screen.findByText("Ready to handle")).resolves.toBeVisible();
  },
};
export const CreatedTask: Story = {
  render: picker(),
  play: async () => {
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", {
        name: "Select A website question",
      }),
    );
    await user.click(
      screen.getByRole("button", { name: "Review selected inquiries" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Create the reviewed tasks" }),
    );
    await expect(screen.findByText("Task created")).resolves.toBeVisible();
  },
};
export const PreviewRefused: Story = {
  render: picker(true),
  play: async () => {
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", {
        name: "Select A website question",
      }),
    );
    await user.click(
      screen.getByRole("button", { name: "Review selected inquiries" }),
    );
    await expect(
      screen.findByRole("button", { name: "Create the reviewed tasks" }),
    ).resolves.toBeDisabled();
  },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { MailReviewRequests } from "./mailreviewrequests";
import { HEALTH_AT } from "./systemhealth.fixtures";

function Owners() {
  installFetchStub({
    "GET /me": meRoute({ job_health: ["read"] }),
    "GET /admin/capture-health": () =>
      jsonResponse({
        generated_at: HEALTH_AT,
        mailboxes: [],
        classifier: { pending: 0, unsure: 3, exhausted: 0 },
        held_meetings: { count: 0 },
        sweeps: [],
        connection_concerns: [],
        owner_reviews: [
          {
            user_id: "00000000-0000-4000-8000-000000000001",
            display_name: "Alex",
            sender_decisions: 2,
            thread_decisions: 0,
          },
          {
            user_id: "00000000-0000-4000-8000-000000000002",
            display_name: "Maya",
            sender_decisions: 1,
            thread_decisions: 0,
          },
        ],
      }),
  });
  return (
    <StoryProviders>
      <MailReviewRequests review="senders" />
    </StoryProviders>
  );
}
const meta = {
  title: "Settings/Governance/System health/Mailbox review requests",
  component: MailReviewRequests,
  args: { review: "senders" },
} satisfies Meta<typeof MailReviewRequests>;
export default meta;
type Story = StoryObj<typeof meta>;
export const MailboxOwners: Story = {
  render: Owners,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Ask a mailbox owner",
      }),
    );
    const dialog = await within(canvasElement.ownerDocument.body).findByRole(
      "dialog",
      { name: "Mailbox decisions" },
    );
    await waitFor(() => expect(within(dialog).getByText("Maya")).toBeVisible());
  },
};
export const Dark: Story = { ...MailboxOwners, globals: { theme: "dark" } };
export const Phone: Story = {
  ...MailboxOwners,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

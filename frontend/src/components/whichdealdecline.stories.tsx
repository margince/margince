// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { WhichDealDecline } from "./whichdealdecline";

// The second answer to "Which deal is this about?": none of them. The first
// answer is a relink from the email itself.

type Activity = components["schemas"]["Activity"];

const QUESTION: Activity = {
  occurred_precision: "instant",
  offers_email: false,
  id: "task-1",
  kind: "task",
  writable: true,
  subject: "Which deal is this about?",
  occurred_at: "2026-09-01T09:12:00Z",
  created_at: "2026-09-01T09:12:00Z",
  updated_at: "2026-09-01T09:12:00Z",
  captured_by: "system:capture-which-deal",
  source: "which_deal",
  source_system: "which_deal",
  source_activity_id: "mail-1",
  is_done: false,
  version: 1,
  content_state: "available",
  audience: "workspace",
};

function Frame({ task }: Readonly<{ task: Activity }>) {
  installFetchStub({
    "POST /activities/mail-1/deal-decline": () =>
      jsonResponse({
        declined_deal_ids: ["deal-1", "deal-2"],
        unlinked: false,
        project_unfiled: false,
        project_kept: false,
        questions_closed: 1,
      }),
  });
  return (
    <StoryProviders>
      <WhichDealDecline task={task} />
    </StoryProviders>
  );
}

const meta: Meta<typeof Frame> = {
  title: "Patterns/Task none of these deals",
  component: Frame,
};
export default meta;
type Story = StoryObj<typeof Frame>;

/** A deal question: the control is offered. */
export const Offered: Story = { args: { task: QUESTION } };
export const OfferedDark: Story = {
  ...Offered,
  globals: { theme: "dark" },
};

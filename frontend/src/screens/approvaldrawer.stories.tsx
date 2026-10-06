// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import { en } from "../i18n/en";
import { ApprovalDecisionDrawer } from "./approvaldrawer";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The drawer a notice or a receipt line opens onto one pending decision.

const meta: Meta = {
  title: "Patterns/Decision drawer",
};
export default meta;

type Story = StoryObj;

export const Open: Story = {
  render: () => {
    installFetchStub({
      "GET /me": meRoute({ deal: ["read", "update"] }),
      "GET /approvals/ap-1": () =>
        jsonResponse({
          id: "ap-1",
          kind: "close_date_correction",
          status: "pending",
          summary: "Confirm the real close date",
          proposed_by: "system:close-date",
          proposed_change: {
            deal_id: "d1",
            expected_close_date: "2026-11-15",
            basis: "Nobody has answered since 5 August.",
          },
          created_at: "2026-08-20T09:00:00Z",
          target_entity_type: "deal",
          target_entity_id: "d1",
        }),
    });
    return (
      <StoryProviders>
        <ApprovalDecisionDrawer approvalId="ap-1" open onClose={() => {}} />
      </StoryProviders>
    );
  },
  play: async () => {
    const drawer = await within(document.body).findByRole("dialog", {
      name: en["worklist.decision.title"],
    });
    await within(drawer).findByText("Confirm the real close date");
  },
};

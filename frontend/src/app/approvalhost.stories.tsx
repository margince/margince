// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Button } from "../design-system/atoms";
import { useOpenApproval } from "../lib/openapproval";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
  visibleDialog,
} from "../testing/story-utils";
import { ApprovalHost } from "./approvalhost";

function OpenDecision() {
  const open = useOpenApproval();
  return <Button onClick={() => open?.("ap-1")}>Open pending decision</Button>;
}
const meta: Meta = { title: "Shell/Decision host" };
export default meta;
type Story = StoryObj;
export const OpensFromRecord: Story = {
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
          proposed_change: { deal_id: "d1", expected_close_date: "2026-11-15" },
          created_at: "2026-08-20T09:00:00Z",
          target_entity_type: "deal",
          target_entity_id: "d1",
        }),
    });
    return (
      <StoryProviders>
        <ApprovalHost>
          <OpenDecision />
        </ApprovalHost>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Open pending decision",
      }),
    );
    const dialog = await visibleDialog();
    await expect(
      within(dialog).findByText("Confirm the real close date"),
    ).resolves.toBeVisible();
  },
};
export const OpensFromRecordDark: Story = {
  ...OpensFromRecord,
  globals: { theme: "dark" },
};

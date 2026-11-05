// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Button } from "../../design-system/atoms";
import { Panel } from "../../design-system/panel";
import { en } from "../../i18n/en";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { ContactClaimsReview } from "./contactclaimsreview";

const meta: Meta<typeof ContactClaimsReview> = {
  title: "Records/Contact 360/Conversation findings",
  component: ContactClaimsReview,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ContactClaimsReview>;

export const HistoricalFinding: Story = {
  render: () => {
    stubWithSession({}, {});
    return (
      <StoryProviders>
        <Panel title={en["record.commitments"]}>
          <ContactClaimsReview
            dismiss={() => <Button>{en["commitment.dismiss"]}</Button>}
            claims={[
              {
                id: "claim-old",
                kind: "commitment_ours",
                status: "open",
                needs_review: true,
                body: "Colleague agreed to arrange the rollout meeting",
                source_activity_id: "mail-old",
                occurred_precision: "instant",
                source_quote: "I will arrange the rollout meeting.",
                source_label: "Rollout planning",
                occurred_at: "2024-07-05T09:00:00Z",
                due_at: "2024-07-08T09:00:00Z",
              },
            ]}
          />
        </Panel>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(canvas.getByText(en["contact.loops.review"]));
    await expect(
      canvas.getByText("Colleague agreed to arrange the rollout meeting"),
    ).toBeVisible();
    await expect(canvas.queryByText(/overdue|You:/i)).toBeNull();
  },
};
export const HistoricalFindingDark: Story = {
  ...HistoricalFinding,
  globals: { theme: "dark" },
};
export const HistoricalFindingPhone: Story = {
  ...HistoricalFinding,
  tags: ["uat-phone"],
};

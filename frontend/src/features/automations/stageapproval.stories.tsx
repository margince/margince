// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { ApprovalRowStory } from "../../testing/approval-row-story";
import { commercialApproval } from "../../testing/commercial-approval";

type Preview = components["schemas"]["ApprovalPreview"];
function proposal(state: Preview["state"] = "ready") {
  const base = commercialApproval();
  return commercialApproval({
    kind: "stage_progression",
    proposed_by: "system:stage-progression",
    summary:
      "Move the service renewal to Negotiation after the agreement was signed.",
    proposed_change: {
      deal_id: base.target_entity_id,
      from_stage_id: "44444444-4444-4444-8444-444444444444",
      to_stage_id: "33333333-3333-4333-8333-333333333333",
      because: "The buyer confirmed the scope and the agreement is signed.",
      confirm_first: true,
      criteria: [
        {
          criterion_id: "55555555-5555-4555-8555-555555555555",
          version: 1,
          key: "signed",
          label: "The agreement is signed",
          required: true,
          met: true,
          evidence_ids: ["66666666-6666-4666-8666-666666666666"],
          cited_source_ids: ["77777777-7777-4777-8777-777777777777"],
        },
      ],
    },
    preview:
      state === "ready"
        ? {
            version: 1,
            state,
            current_version: 3,
            reviewer_reason: "requested_decider",
            changes: [
              {
                field: "stage_id",
                before: { kind: "stage", text: "Discovery" },
                after: { kind: "stage", text: "Negotiation" },
              },
            ],
            details: [
              {
                field: "stage_required_criterion_met",
                value: { kind: "text", text: "The agreement is signed" },
              },
            ],
            evidence: [
              {
                evidence_snippet: "The agreement is signed",
                source_type: "contract",
                source_id: "77777777-7777-4777-8777-777777777777",
              },
            ],
          }
        : {
            version: 1,
            state,
            changes: [],
            evidence: [],
            reviewer_reason: "requested_decider",
          },
  });
}
const meta: Meta = {
  title: "Patterns/Automatic stage decision",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
export const Ready: Story = {
  render: () => <ApprovalRowStory subject={proposal()} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText("Negotiation", { exact: true }),
    ).toBeVisible();
    await expect(canvas.getByText("Discovery", { exact: true })).toBeVisible();
    await expect(
      canvas.getByText("Required criterion met", { exact: true }),
    ).toBeVisible();
    await expect(
      canvas.getByText(
        "The buyer confirmed the scope and the agreement is signed.",
      ),
    ).toBeVisible();
  },
};
export const German: Story = {
  render: () => <ApprovalRowStory subject={proposal()} locale="de" />,
};
export const Vietnamese: Story = {
  render: () => <ApprovalRowStory subject={proposal()} locale="vi" />,
};
export const DestinationChanged: Story = {
  render: () => <ApprovalRowStory subject={proposal("stale")} />,
};
export const EvidenceUnavailable: Story = {
  render: () => <ApprovalRowStory subject={proposal("unavailable")} />,
};
export const LegacyNeedsRestaging: Story = {
  render: () => <ApprovalRowStory subject={proposal("incomplete")} />,
};

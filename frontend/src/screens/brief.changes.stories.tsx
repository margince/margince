// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { BriefChanges } from "./brief.changes";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";
import {
  automaticDateReceipt,
  automaticStageReceipt,
} from "./worklist.receiptreview.fixtures";

const meta: Meta<typeof BriefChanges> = {
  title: "Shell/Brief changes",
  component: BriefChanges,
};
export default meta;
type Story = StoryObj<typeof BriefChanges>;
export const AppliedChanges: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /worklist/handled": () =>
          jsonResponse({
            as_of: "2026-09-13T08:00:00Z",
            receipts: [automaticDateReceipt, automaticStageReceipt],
            truncated: false,
          }),
        [`GET /deals/${automaticStageReceipt.subject?.id}`]: () =>
          jsonResponse({ name: "PIM Rollout" }),
      },
      { deal: ["read", "update"] },
    );
    return (
      <StoryProviders>
        <BriefChanges />
      </StoryProviders>
    );
  },
};
export const QuietNight: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /worklist/handled": () =>
          jsonResponse({
            as_of: "2026-09-13T08:00:00Z",
            receipts: [],
            truncated: false,
          }),
      },
      {},
    );
    return (
      <StoryProviders>
        <BriefChanges />
      </StoryProviders>
    );
  },
};

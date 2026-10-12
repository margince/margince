// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel } from "../design-system/panel";
import { BriefChanges } from "./brief.changes";
import { jsonResponse, StoryProviders, stubWithSession } from "./story-utils";
import {
  automaticDateReceipt,
  automaticStageReceipt,
} from "./worklist.receiptreview.fixtures";

const meta: Meta<typeof BriefChanges> = {
  title: "Shell/Home/Changes",
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
    // A group of Home's receipt, so it is drawn inside a pane the way the
    // receipt draws it.
    return (
      <StoryProviders>
        <Panel>
          <BriefChanges />
        </Panel>
      </StoryProviders>
    );
  },
};

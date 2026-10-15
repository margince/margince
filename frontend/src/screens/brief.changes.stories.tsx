// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel } from "../design-system/panel";
import {
  automaticDateReceipt,
  automaticStageReceipt,
} from "../features/worklist/worklist.receiptreview.fixtures";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../testing/story-utils";
import { BriefChanges } from "./brief.changes";

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

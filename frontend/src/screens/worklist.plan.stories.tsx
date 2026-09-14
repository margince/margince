// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { Panel } from "../design-system/panel";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";
import { WorklistRow } from "./worklist.row";
import "./worklist.css";

export default { title: "Records/Worklist/Weekly commitment" } satisfies Meta;

export const DueCommitment: StoryObj = {
  render: () => {
    const me = meFixture();
    installFetchStub({ "GET /me": () => jsonResponse(me) });
    return (
      <StoryProviders>
        <Panel title="Today">
          <WorklistRow
            density="compact"
            owner=""
            onReview={() => {}}
            item={{
              id: "commitment-1",
              source: "weekly_commitment",
              category: "tasks",
              level: 2,
              title: "Confirm the renewal decision with Weber",
              due_at: "2026-09-12T21:59:59Z",
              overdue: true,
              urgent: true,
              consequence: "promise_breaks",
              because: [],
              actions: [],
              owner: {
                kind: "user",
                id: me.user.id,
                label: me.user.display_name,
              },
            }}
          />
        </Panel>
      </StoryProviders>
    );
  },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import { jsonResponse, StoryProviders } from "./story-utils";
import { WorklistRow } from "./worklist.row";

// One row of the queue, standing on its own — the unit worklist.stories.tsx
// exercises through the whole screen. Covered here directly because fe-uat
// (frontend/AGENTS.md) credits a story only for the file it imports, and
// nothing else in the tree imports WorklistRow.

type WorklistItem = components["schemas"]["WorklistItem"];

function noticeItem(): WorklistItem {
  return {
    id: "n1",
    source: "notice",
    category: "tasks",
    level: 4,
    consequence: "task_slips",
    title: "A deal you own changed stage",
    detail: "Acme Renewal moved to a new pipeline stage.",
    because: [],
    actions: ["acknowledge"],
  };
}

function stubRow(readPending: boolean) {
  globalThis.fetch = (async (input: RequestInfo | URL): Promise<Response> => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.includes("/notices/n1/read")) {
      return readPending
        ? new Promise(() => {})
        : new Response(null, { status: 204 });
    }
    return jsonResponse({ data: [] });
  }) as typeof fetch;
}

const meta: Meta<typeof WorklistRow> = {
  title: "Records/Worklist/Row",
  component: WorklistRow,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Panel title="What to do next">
          <ol className="worklist-list">
            <li>
              <Story />
            </li>
          </ol>
        </Panel>
      </StoryProviders>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof WorklistRow>;

const baseArgs = {
  position: 1,
  owner: "",
  selected: false,
  onSelect: () => undefined,
  onReview: () => undefined,
};

// A notice, and the one verb the row draws inline rather than as a link — see
// NoticeAcknowledge in worklist.row.tsx for why.
export const ANoticeToSettle: Story = {
  args: { ...baseArgs, item: noticeItem() },
  render: (args) => {
    stubRow(false);
    return <WorklistRow {...args} />;
  },
};

// Pending: "Got it" is clicked and the read call never resolves.
export const ANoticeSettling: Story = {
  args: { ...baseArgs, item: noticeItem() },
  render: (args) => {
    stubRow(true);
    return <WorklistRow {...args} />;
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Got it" }),
    );
  },
};

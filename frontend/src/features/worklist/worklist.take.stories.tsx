// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { WorklistItem } from "./worklist.queries";
import { TakeVerb } from "./worklist.take";

function unheldTask(): WorklistItem {
  return {
    occurred_precision: "instant",
    id: "01a05500-0000-7000-8000-0000000000t2",
    source: "task",
    category: "tasks",
    level: 3,
    consequence: "task_slips",
    title: "Send the renewal terms",
    because: [{ kind: "unassigned" }],
    actions: ["complete", "snooze", "take"],
    version: 2,
  };
}

const meta = {
  title: "Records/Worklist/Take verb",
  component: TakeVerb,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      installFetchStub({ "GET /me": meRoute({}, { roles: ["rep"] }) });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
} satisfies Meta<typeof TakeVerb>;
export default meta;

type Story = StoryObj<typeof meta>;

/** A task nobody holds that the reader may change. */
export const Offered: Story = {
  args: { item: unheldTask() },
};

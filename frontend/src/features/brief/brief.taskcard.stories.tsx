// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { taskRow } from "../../testing/brief.fixtures";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import type { WorklistItem } from "../worklist/worklist.queries";
import { TaskCard } from "./brief.taskcard";

const meta: Meta<typeof TaskCard> = {
  title: "Shell/Home/Task card",
  component: TaskCard,
};
export default meta;

type Story = StoryObj<typeof TaskCard>;

function card(item: WorklistItem) {
  return () => {
    installFetchStub({ "GET /me": meRoute({}, { roles: ["rep"] }) });
    return (
      <StoryProviders>
        <TaskCard item={item} onOpen={() => undefined} />
      </StoryProviders>
    );
  };
}

// The round tick completes the task.
export const Completable: Story = {
  render: card(taskRow("call-alice", "Call Alice back about the retrofit")),
};

// A task the reader may not complete keeps the tick's column empty.
export const ReadOnly: Story = {
  render: card({
    ...taskRow("call-alice", "Call Alice back about the retrofit"),
    actions: ["open"],
  }),
};

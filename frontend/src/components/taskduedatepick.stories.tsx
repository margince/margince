// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { en } from "../i18n/en";
import { taskWriteKeys } from "../lib/activitykeys";
import { ViewerZoneProvider } from "../lib/viewerzone";
import { FIXTURE_TIMEZONE } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { TaskDueDatePick } from "./taskduedatepick";
import { useTaskUpdate } from "./taskwrite";

// The day and time a task moves to, sent together by Move, as the task's
// detail panel draws it.

const meta: Meta = {
  title: "Patterns/Task due date pick",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// A deadline far enough ahead that a picked day starts out in the future.
const DUE = "2027-09-01T13:00:00.000Z";

function PickBody({ dueAt }: Readonly<{ dueAt: string | null }>) {
  const update = useTaskUpdate(taskWriteKeys("company", "o-1"));
  return (
    <div className="task-detail-verbs">
      <TaskDueDatePick
        activityId="a-1"
        version={3}
        dueAt={dueAt}
        dueHasTime={dueAt !== null}
        update={update}
      />
    </div>
  );
}

function Pick({ dueAt = DUE }: Readonly<{ dueAt?: string | null }>) {
  installFetchStub({
    "PATCH /activities/a-1": () => jsonResponse({ id: "a-1", version: 4 }),
  });
  return (
    <StoryProviders>
      <ViewerZoneProvider zone={FIXTURE_TIMEZONE}>
        <ToastProvider>
          <PickBody dueAt={dueAt} />
          <ToastRegion />
        </ToastProvider>
      </ViewerZoneProvider>
    </StoryProviders>
  );
}

/** A dated task: the picker opens on its day and time. */
export const Dated: Story = { render: () => <Pick /> };

/** A day already past: the warning under the fields, and Move anyway. */
export const MovePastDate: Story = {
  render: () => <Pick />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.tripleClick(
      canvas.getByRole("textbox", { name: en["tasks.moveTo"] }),
    );
    await user.keyboard("01/01/2026{Enter}");
    await canvas.findByText(en["tasks.movePastTitle"]);
    await canvas.findByRole("button", { name: en["tasks.moveAnyway"] });
  },
};

/** Move on an undated task with no day picked asks for the day. */
export const NoDayChosen: Story = {
  render: () => <Pick dueAt={null} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent
      .setup()
      .click(canvas.getByRole("button", { name: en["tasks.move"] }));
    await canvas.findByText(/is required/);
  },
};

/** Move with the deadline unchanged says the task is already due then. */
export const NothingChanged: Story = {
  render: () => <Pick />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent
      .setup()
      .click(canvas.getByRole("button", { name: en["tasks.move"] }));
    await canvas.findByText(en["tasks.moveSame"]);
  },
};

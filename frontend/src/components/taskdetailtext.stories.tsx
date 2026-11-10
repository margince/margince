import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { Heading } from "../design-system/heading";
import { meFixture } from "../testing/mefixture";
import { StoryProviders, storyQueryClient } from "../testing/story-utils";
import { systemTask } from "../testing/systemtext.fixture";
import { TaskDetailReading, TaskDetailTitle } from "./taskdetailtext";

export default {
  title: "Patterns/Task reading",
} satisfies Meta;
export const Vietnamese: StoryObj = {
  render: () => {
    const client = storyQueryClient();
    client.setQueryData(["me"], meFixture());
    return (
      <StoryProviders client={client} locale="vi">
        <Heading size="large" as="h2">
          <TaskDetailTitle task={systemTask} />
        </Heading>
        <TaskDetailReading task={systemTask} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByText(systemTask.system_display?.subject ?? ""),
    ).toBeVisible();
    await expect(
      canvas.getByText(systemTask.system_display?.body ?? ""),
    ).toBeVisible();
  },
};

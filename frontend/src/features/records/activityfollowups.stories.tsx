import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ActivityFollowUps } from "./activityfollowups";

const meta: Meta<typeof ActivityFollowUps> = {
  title: "Records/Record 360/Follow-up tasks",
  component: ActivityFollowUps,
  args: { sourceId: "source-note", archived: false, onOpen: () => {} },
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof ActivityFollowUps>;
export const Tasks: Story = {
  render: (args) => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "create"] }),
      "GET /activities": () =>
        jsonResponse({
          data: [
            {
              id: "task-one",
              kind: "task",
              subject: "Send revised scope",
              is_done: false,
            },
            {
              id: "task-two",
              kind: "task",
              subject: "Confirm receipt",
              is_done: true,
            },
          ],
          page: { has_more: false },
        }),
    });
    return <ActivityFollowUps {...args} />;
  },
};
export const TasksDark: Story = { ...Tasks, globals: { theme: "dark" } };
export const AddFollowUp: Story = {
  ...Tasks,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: en["log.addFollowUp"] }),
    );
    await userEvent.type(
      canvas.getByRole("textbox", { name: en["log.subject"] }),
      "Send revised pricing",
    );
  },
};
export const AddFollowUpDark: Story = {
  ...AddFollowUp,
  globals: { theme: "dark" },
};

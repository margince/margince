import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHost } from "../story-hosts";
import { approvalQueueFixture } from "./fixture";
import { render } from "./main";

const meta: Meta<typeof ViewHost> = {
  title: "MCP Apps/Approval queue",
  component: ViewHost,
  args: { render },
};
export default meta;

type Story = StoryObj<typeof ViewHost>;

export const Waiting: Story = { args: { data: approvalQueueFixture.data } };

/** An empty queue says nothing is waiting. */
export const NothingWaiting: Story = { args: { data: { approvals: [] } } };

/** The same queue under the dark tokens. */
export const WaitingDark: Story = {
  args: { data: approvalQueueFixture.data },
  globals: { theme: "dark" },
};

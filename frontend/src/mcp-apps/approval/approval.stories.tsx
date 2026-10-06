import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHost } from "../story-hosts";
import { approvalFixture } from "./fixture";
import { render } from "./main";

const meta: Meta<typeof ViewHost> = {
  title: "MCP Apps/Approval",
  component: ViewHost,
  args: { render },
};
export default meta;

type Story = StoryObj<typeof ViewHost>;

export const Pending: Story = { args: { data: approvalFixture.data } };

/** An answer that is not a staged action draws nothing. */
export const NotAStagedAction: Story = { args: { data: {} } };

/** A lapsed proposal offers no choice, as the inbox's own card does not. */
export const Lapsed: Story = {
  args: {
    data: {
      ...(approvalFixture.data as object),
      expires_at: "2020-01-01T00:00:00Z",
    },
  },
};

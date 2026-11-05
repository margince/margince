import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHost } from "../story-hosts";
import { briefFixture, catchUpFixture, meetingFixture } from "./fixture";
import { render } from "./main";

const meta: Meta<typeof ViewHost> = {
  title: "MCP Apps/Context",
  component: ViewHost,
  args: { render },
};
export default meta;

type Story = StoryObj<typeof ViewHost>;

export const Meeting: Story = { args: { data: meetingFixture.data } };

export const CatchUp: Story = { args: { data: catchUpFixture.data } };

export const MorningBrief: Story = { args: { data: briefFixture.data } };

/** The same card under the dark tokens. */
export const MeetingDark: Story = {
  args: { data: meetingFixture.data },
  globals: { theme: "dark" },
};

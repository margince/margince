import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHost } from "../story-hosts";
import { checksFixture, movementFixture, readingsFixture } from "./fixture";
import { render } from "./main";

const meta: Meta<typeof ViewHost> = {
  title: "MCP Apps/Forecast",
  component: ViewHost,
  args: { render },
};
export default meta;

type Story = StoryObj<typeof ViewHost>;

export const Readings: Story = { args: { data: readingsFixture.data } };

export const Movement: Story = { args: { data: movementFixture.data } };

export const InputChecks: Story = { args: { data: checksFixture.data } };

/** The same card under the dark tokens. */
export const ReadingsDark: Story = {
  args: { data: readingsFixture.data },
  globals: { theme: "dark" },
};

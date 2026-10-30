import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHost } from "../story-hosts";
import { updateFixture } from "./fixture";
import { render } from "./main";

const meta: Meta<typeof ViewHost> = {
  title: "MCP Apps/Update",
  component: ViewHost,
  args: { render },
};
export default meta;

type Story = StoryObj<typeof ViewHost>;

export const Changed: Story = { args: { data: updateFixture.data } };

/** An update that reports no changes says it is saved. */
export const NoChangesListed: Story = {
  args: { data: { record_type: "contact", id: "x", fields: {} } },
};

/** The same card under the dark tokens. */
export const ChangedDark: Story = {
  args: { data: updateFixture.data },
  globals: { theme: "dark" },
};

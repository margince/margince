import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHost } from "../story-hosts";
import { fieldConflictFixture } from "./fixture";
import { render } from "./main";

const meta: Meta<typeof ViewHost> = {
  title: "MCP Apps/Field conflict",
  component: ViewHost,
  args: { render },
};
export default meta;

type Story = StoryObj<typeof ViewHost>;

export const Held: Story = { args: { data: fieldConflictFixture.data } };

/** An update that held nothing back draws nothing. */
export const NothingHeld: Story = {
  args: { data: { record_type: "contact", id: "x", fields: {} } },
};

/** The same card under the dark tokens. */
export const HeldDark: Story = {
  args: { data: fieldConflictFixture.data },
  globals: { theme: "dark" },
};

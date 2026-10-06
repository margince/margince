import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHost } from "../story-hosts";
import { duplicateFixture } from "./fixture";
import { render } from "./main";

const meta: Meta<typeof ViewHost> = {
  title: "MCP Apps/Duplicate",
  component: ViewHost,
  args: { render },
};
export default meta;

type Story = StoryObj<typeof ViewHost>;

export const Filed: Story = { args: { data: duplicateFixture.data } };

/** A create that filed nothing draws nothing: the card is bound to the tool. */
export const NothingFiled: Story = {
  args: { data: { record_type: "contact", id: "x", fields: {} } },
};

/** A lead has no merge, so the pair offers only the not-the-same choice. */
export const Lead: Story = {
  args: { data: { ...(duplicateFixture.data as object), record_type: "lead" } },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { builtDocument, DocumentHost } from "../story-hosts";
import { fieldConflictFixture } from "./fixture";

// import.meta.glob, NOT a static `?raw` import — see the account brief's
// document story for why, and why `eager` is beside the point.
const built = import.meta.glob("/dist/mcp-apps/*.html", {
  query: "?raw",
  import: "default",
  eager: true,
});

const meta: Meta<typeof DocumentHost> = {
  title: "MCP Apps/Field conflict (document)",
  component: DocumentHost,
  args: {
    html: builtDocument(built, "field-conflict"),
    answer: fieldConflictFixture,
    title: "Edited by hand",
  },
};
export default meta;

type Story = StoryObj<typeof DocumentHost>;

export const InALightHost: Story = { args: { theme: "light" } };

export const InADarkHost: Story = { args: { theme: "dark" } };

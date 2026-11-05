import type { Meta, StoryObj } from "@storybook/react-vite";
import { builtDocument, DocumentHost } from "../story-hosts";
import { meetingFixture } from "./fixture";

// import.meta.glob rather than a static `?raw` import: an absent built file
// then renders the "run pnpm build" panel instead of failing the build.
const built = import.meta.glob("/dist/mcp-apps/*.html", {
  query: "?raw",
  import: "default",
  eager: true,
});

const meta: Meta<typeof DocumentHost> = {
  title: "MCP Apps/Context (document)",
  component: DocumentHost,
  args: {
    html: builtDocument(built, "context"),
    answer: meetingFixture,
    title: "The story so far",
  },
};
export default meta;

type Story = StoryObj<typeof DocumentHost>;

export const InALightHost: Story = { args: { theme: "light" } };

export const InADarkHost: Story = { args: { theme: "dark" } };

import type { Meta, StoryObj } from "@storybook/react-vite";
import { ViewHost } from "../story-hosts";
import { createFollowupsFixture } from "./fixture";
import { render } from "./main";

const meta: Meta<typeof ViewHost> = {
  title: "MCP Apps/Create followups",
  component: ViewHost,
  args: { render },
};
export default meta;

type Story = StoryObj<typeof ViewHost>;

export const Filed: Story = { args: { data: createFollowupsFixture.data } };

/** A create that filed nothing draws nothing: the card is bound to the tool. */
export const NothingFiled: Story = {
  args: { data: { record_type: "contact", id: "x", fields: {} } },
};

/** A lead has no merge, so the pair offers only the not-the-same choice. */
export const Lead: Story = {
  args: {
    data: { ...(createFollowupsFixture.data as object), record_type: "lead" },
  },
};

/** A word the assistant proposed that the workspace already has. */
export const TagOffer: Story = {
  args: {
    data: {
      record_type: "contact",
      id: "0195c3a0-0000-7000-8000-000000000002",
      fields: { full_name: "Anna Meyer" },
      tag_offer: {
        name: "K5 Conference 2026",
        tag_id: "0195c3a0-0000-7000-8000-0000000000e1",
        exists: true,
        may_create: false,
      },
    },
  },
};

/** A word the workspace lacks and this seat cannot coin. */
export const TagOfferRefused: Story = {
  args: {
    data: {
      record_type: "contact",
      id: "x",
      fields: {},
      tag_offer: {
        name: "K5 Conference 2026",
        exists: false,
        may_create: false,
      },
    },
  },
};

/** The same card under the dark tokens. */
export const FiledDark: Story = {
  args: { data: createFollowupsFixture.data },
  globals: { theme: "dark" },
};

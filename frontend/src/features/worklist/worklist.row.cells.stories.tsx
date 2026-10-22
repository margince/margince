import type { Meta, StoryObj } from "@storybook/react-vite";
import { viewerZone } from "../../format/timezone";
import { StoryProviders } from "../../testing/story-utils";
import { row } from "./worklist.fixtures";
import { RowText } from "./worklist.row.cells";

const meta: Meta<typeof RowText> = {
  title: "Records/Worklist source availability",
  component: RowText,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  args: {
    emailOpener: undefined,
    badged: false,
    readings: {
      item: row({ source: "conversation_claim", actions: [] }),
      title: "Send revised pricing · Mira",
      href: "#/contacts/buyer",
      sourceUnavailable: true,
      when: null,
      facts: null,
      said: [],

      sample: [],
      touch: [],
      zone: viewerZone(),
    },
  },
};
export default meta;
type Story = StoryObj<typeof RowText>;
export const CounterpartyWithUnavailableEvidence: Story = {};
export const CounterpartyWithUnavailableEvidenceDark: Story = {
  globals: { theme: "dark" },
};

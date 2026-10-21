import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import { MagicUndoButton } from "./magic.undo";

const meta: Meta<typeof MagicUndoButton> = {
  title: "Shell/Home/Undo a change",
  component: MagicUndoButton,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof MagicUndoButton>;
export const Available: Story = {
  args: {
    entityType: "deal",
    entityId: "00000000-0000-7000-8000-000000000001",
    undo: {
      undoable: true,
      audit_id: "00000000-0000-7000-8000-000000000002",
      version: 2,
    },
  },
};
export const AvailableDark: Story = {
  ...Available,
  globals: { theme: "dark" },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { MessengerPicker } from "./messenger.picker";
import { MessengerStory } from "./messenger.stories.testkit";
import { messengerScope } from "./messenger.testkit";

const meta = {
  title: "Records/Team chat/Conversation picker",
  component: MessengerPicker,
  decorators: [
    (Story) => (
      <MessengerStory>
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof MessengerPicker>;
export default meta;
type Story = StoryObj<typeof meta>;
export const RecordTeam: Story = {
  args: { scope: messengerScope, onChoose: () => {} },
};

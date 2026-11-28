import type { Meta, StoryObj } from "@storybook/react-vite";
import { MessengerMessages } from "./messenger.messages";
import { MessengerStory } from "./messenger.stories.testkit";
import { messengerContext, messengerScope } from "./messenger.testkit";

const meta = {
  title: "Records/Team chat/Conversation",
  component: MessengerMessages,
  decorators: [
    (Story) => (
      <MessengerStory>
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof MessengerMessages>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Channel: Story = {
  args: { scope: messengerScope, context: messengerContext, seat: "demo-seat" },
};
export const Direct: Story = {
  args: {
    scope: messengerScope,
    context: {
      ...messengerContext,
      conversation: {
        ...messengerContext.conversation,
        kind: "direct",
        name: "Jordan Lee",
      },
    },
    seat: "demo-seat",
  },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { MessengerAudience } from "./messenger.audience";
import { MessengerStory } from "./messenger.stories.testkit";
import { messengerContext, messengerScope } from "./messenger.testkit";

const meta = {
  title: "Records/Team chat/Recipients",
  component: MessengerAudience,
  decorators: [
    (Story) => (
      <MessengerStory>
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof MessengerAudience>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Current: Story = {
  args: {
    scope: messengerScope,
    context: messengerContext,
    changed: false,
    onReview: () => {},
  },
};
export const Changed: Story = {
  args: {
    scope: messengerScope,
    context: messengerContext,
    changed: true,
    onReview: () => {},
  },
};
export const External: Story = {
  args: {
    scope: messengerScope,
    context: {
      ...messengerContext,
      conversation: { ...messengerContext.conversation, external: true },
    },
    changed: true,
    onReview: () => {},
  },
};

export const WorkspaceChannel: Story = {
  ...Current,
  args: {
    ...Current.args,
    context: {
      ...messengerContext,
      conversation: { ...messengerContext.conversation, access: "workspace" },
    },
  },
};
export const AppInChannel: Story = {
  ...Current,
  args: {
    ...Current.args,
    context: {
      ...messengerContext,
      conversation: {
        ...messengerContext.conversation,
        access: "private",
        members: [
          ...messengerContext.conversation.members,
          {
            id: "U_APP",
            name: "Calendar app",
            bot: true,
            active: true,
            external: false,
          },
        ],
      },
    },
  },
};

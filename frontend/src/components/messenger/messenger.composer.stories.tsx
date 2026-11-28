import type { Meta, StoryObj } from "@storybook/react-vite";
import { MessengerComposer } from "./messenger.composer";
import { MessengerStory } from "./messenger.stories.testkit";
import { messengerContext, messengerScope } from "./messenger.testkit";

const meta = {
  title: "Records/Team chat/Message composer",
  component: MessengerComposer,
  decorators: [
    (Story) => (
      <MessengerStory>
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof MessengerComposer>;
export default meta;
type Story = StoryObj<typeof meta>;
export const ChannelReply: Story = {
  args: {
    scope: messengerScope,
    context: messengerContext,
    root: "",
    draftKey: "story-channel",
    onSent: () => {},
  },
};

export const ReopenedUnknownDelivery: Story = {
  ...ChannelReply,
  args: {
    ...ChannelReply.args,
    draftKey: "story-reopened-unknown",
    context: {
      ...messengerContext,
      grant_version: 2,
      unresolved_attempt: {
        id: "018f3a1b-0000-7000-8000-00000000a009",
        request_id: "018f3a1b-0000-7000-8000-00000000a009",
        conversation_id: messengerContext.conversation.id,
        provider_message_id: "",
        state: "unknown",
        updated_at: "2026-06-01T10:00:00Z",
      },
    },
  },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { ChannelConversationPicker } from "./channelconversationpicker";
import { MessengerStory } from "./messenger/messenger.stories.testkit";
import { messengerScope } from "./messenger/messenger.testkit";

const meta = {
  title: "Records/Team chat/Conversation assignment",
  component: ChannelConversationPicker,
  decorators: [
    (Story) => (
      <MessengerStory>
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof ChannelConversationPicker>;
export default meta;
type Story = StoryObj<typeof meta>;
export const SharedChannel: Story = {
  args: {
    provider: "slack",
    recordType: "deal",
    recordId: messengerScope.id,
    channel: "C_PILOT",
    value: "",
    onChange: () => {},
  },
  render: (args) => {
    const [value, Value] = useState("");
    return (
      <ChannelConversationPicker {...args} value={value} onChange={Value} />
    );
  },
};

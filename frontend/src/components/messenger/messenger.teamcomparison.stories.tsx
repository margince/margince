import type { Meta, StoryObj } from "@storybook/react-vite";
import { MessengerStory } from "./messenger.stories.testkit";
import { MessengerTeamComparison } from "./messenger.teamcomparison";
import { messengerContext, messengerScope } from "./messenger.testkit";

const meta = {
  title: "Records/Team chat/Assigned team comparison",
  component: MessengerTeamComparison,
  decorators: [
    (Story) => (
      <MessengerStory>
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof MessengerTeamComparison>;
export default meta;
type Story = StoryObj<typeof meta>;
export const AssignedAndActual: Story = {
  args: {
    members: messengerContext.conversation.members,
    access: messengerContext.conversation.access,
    scope: messengerScope,
  },
};

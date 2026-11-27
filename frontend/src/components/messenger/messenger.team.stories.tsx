import type { Meta, StoryObj } from "@storybook/react-vite";
import { MessengerStory } from "./messenger.stories.testkit";
import { MessengerTeamPicker } from "./messenger.team";
import { messengerScope } from "./messenger.testkit";

const meta = {
  title: "Records/Team chat/Record team",
  component: MessengerTeamPicker,
  decorators: [
    (Story) => (
      <MessengerStory>
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof MessengerTeamPicker>;
export default meta;
type Story = StoryObj<typeof meta>;
export const MatchedTeam: Story = {
  args: { scope: messengerScope, onChoose: () => {} },
};

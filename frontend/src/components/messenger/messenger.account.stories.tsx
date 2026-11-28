import type { Meta, StoryObj } from "@storybook/react-vite";
import { MessengerAccountActions } from "./messenger.account";
import { MessengerStory } from "./messenger.stories.testkit";

const meta = {
  title: "Records/Team chat/Participant account",
  component: MessengerAccountActions,
  decorators: [
    (Story, context) => (
      <MessengerStory
        manageProviders={context.parameters.manageProviders === true}
        dispactConfigured={context.parameters.dispactConfigured !== false}
      >
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof MessengerAccountActions>;
export default meta;
type Story = StoryObj<typeof meta>;
export const NeedsConsent: Story = {
  args: { provider: "slack", state: "needs_consent" },
};
export const Connected: Story = {
  args: { provider: "slack", state: "connected" },
};
export const Unavailable: Story = {
  args: { provider: "teams", state: "unavailable" },
};

export const AdministratorNeedsDispactSetup: Story = {
  args: { provider: "dispact", state: "unavailable" },
  parameters: { manageProviders: true, dispactConfigured: false },
};

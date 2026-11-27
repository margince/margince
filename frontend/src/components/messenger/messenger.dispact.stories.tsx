import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import { DispactParticipantConnect } from "./messenger.dispact";
import { MessengerStory } from "./messenger.stories.testkit";

const meta = {
  title: "Records/Team chat/Dispact account",
  component: DispactParticipantConnect,
  decorators: [
    (Story) => (
      <MessengerStory>
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof DispactParticipantConnect>;
export default meta;
type Story = StoryObj<typeof meta>;
export const PersonalConsent: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: englishCopy["messenger.connect"],
      }),
    );
    const dialog = await within(document.body).findByRole("dialog");
    await expect(
      within(dialog).getByLabelText(englishCopy["messenger.dispactToken"]),
    ).toHaveAttribute("type", "password");
  },
};

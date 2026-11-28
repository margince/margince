import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import { MessengerOutcome } from "./messenger.outcome";
import { MessengerStory } from "./messenger.stories.testkit";
import { messengerScope } from "./messenger.testkit";

const meta = {
  title: "Records/Team chat/Share outcome",
  component: MessengerOutcome,
  decorators: [
    (Story) => (
      <MessengerStory>
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof MessengerOutcome>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Review: Story = {
  args: { scope: messengerScope },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(
        canvas.getByRole("button", {
          name: englishCopy["messenger.shareOutcome"],
        }),
      ).toBeEnabled(),
    );
    await userEvent.click(
      canvas.getByRole("button", {
        name: englishCopy["messenger.shareOutcome"],
      }),
    );
    const dialog = await within(document.body).findByRole("dialog", {
      name: englishCopy["messenger.shareOutcome"],
    });
    await waitFor(() =>
      expect(
        within(dialog).getByText(/Members who can read the destination record/),
      ).toBeVisible(),
    );
  },
};

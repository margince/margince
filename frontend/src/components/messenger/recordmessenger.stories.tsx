import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import { MessengerStory } from "./messenger.stories.testkit";
import { messengerScope } from "./messenger.testkit";
import { RecordMessenger } from "./recordmessenger";

const meta = {
  title: "Records/Team chat/Messenger",
  component: RecordMessenger,
  decorators: [
    (Story, context) => (
      <MessengerStory
        provider={context.name === "Account Consent" ? "dispact" : "slack"}
      >
        <Story />
      </MessengerStory>
    ),
  ],
} satisfies Meta<typeof RecordMessenger>;
export default meta;
type Story = StoryObj<typeof meta>;
export const RecordAction: Story = {
  args: {
    recordType: "deal",
    recordId: messengerScope.id,
    name: "Pilot rollout",
  },
};
export const AccountConsent: Story = {
  ...RecordAction,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "Messenger" }),
    );
    await expect(
      within(document.body).findByText(englishCopy["messenger.dispactConsent"]),
    ).resolves.toBeVisible();
  },
};

export const PrivateTeamConversation: Story = {
  ...RecordAction,
  play: async ({ canvasElement }) => {
    const page = within(document.body);
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "Messenger" }),
    );
    await userEvent.click(
      await page.findByRole("button", {
        name: englishCopy["messenger.messageTeam"],
      }),
    );
    await expect(
      page.findByRole("textbox", { name: "Message Jordan Lee" }),
    ).resolves.toBeVisible();
    await expect(page.getByText("Opened from Pilot rollout")).toBeVisible();
    await expect(
      page.getByText(englishCopy["messenger.private"]),
    ).toBeVisible();
  },
};

export const PrivateTeamPhone: Story = {
  ...PrivateTeamConversation,
  tags: ["uat-phone"],
};

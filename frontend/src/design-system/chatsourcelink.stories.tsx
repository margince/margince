import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { en } from "../i18n/en";
import { meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ChatSourceLink } from "./chatsourcelink";

const activity = "018f3a1b-0000-7000-8000-00000000a001";
const reader = meFixture({ allow: {} });
const meta = {
  title: "Components/AI and provenance/Chat source access",
  component: ChatSourceLink,
  decorators: [
    (Story, context) => {
      installFetchStub({
        "GET /me": () => jsonResponse(reader),
        [`GET /activities/${activity}/chat-source`]: () =>
          jsonResponse({
            available: context.parameters.sourceAvailable !== false,
            user_id: reader.user.id,
            url: "https://fixture.slack.com/archives/C_PILOT/p1700000000000001",
            valid_until: "2099-06-01T10:00:00Z",
          }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: { activityId: activity, provider: "Slack" },
} satisfies Meta<typeof ChatSourceLink>;
export default meta;
type Story = StoryObj<typeof meta>;
export const NotChecked: Story = {};
export const Verified: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() => expect(canvas.getByRole("button")).toBeEnabled());
    await userEvent.click(canvas.getByRole("button"));
    await expect(await canvas.findByRole("link")).toHaveAttribute(
      "href",
      "https://fixture.slack.com/archives/C_PILOT/p1700000000000001",
    );
  },
};
export const Unavailable: Story = {
  parameters: { sourceAvailable: false },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() => expect(canvas.getByRole("button")).toBeEnabled());
    await userEvent.click(canvas.getByRole("button"));
    await expect(
      await canvas.findByText(en["timeline.chatSummary.sourceUnavailable"]),
    ).toBeVisible();
    await expect(canvas.queryByRole("link")).toBeNull();
  },
};
export const VerifiedDark: Story = { ...Verified, globals: { theme: "dark" } };

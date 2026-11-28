import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { MessagingServiceSwitch } from "./messagingserviceswitch";

const meta = {
  title: "Settings/Data/Integrations/Messaging service",
  component: MessagingServiceSwitch,
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /me": meRoute(
          { channel_connection: ["read", "delete"] },
          { seat: "full" },
        ),
        "GET /messenger/service": () =>
          jsonResponse({ active_provider: "dispact", providers: ["dispact"] }),
        "PUT /messenger/service": () => new Response(null, { status: 204 }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
} satisfies Meta<typeof MessagingServiceSwitch>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Connected: Story = {};
export const ConfirmSwitch: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const select = await canvas.findByRole("combobox", {
      name: en["messenger.serviceTitle"],
    });
    await waitFor(() => expect(select).toBeEnabled());
    await userEvent.click(select);
    await userEvent.click(
      await within(document.body).findByRole("option", { name: "Slack" }),
    );
    await userEvent.click(
      canvas.getByRole("combobox", { name: en["channelProfile.distribution"] }),
    );
    await userEvent.click(
      await within(document.body).findByRole("option", {
        name: en["channelProfile.internal"],
      }),
    );
    await userEvent.click(
      canvas.getByRole("button", { name: en["messenger.switchService"] }),
    );
    const dialog = await within(document.body).findByRole("dialog");
    await waitFor(() =>
      expect(within(dialog).getByText(/Slack/)).toBeVisible(),
    );
  },
};

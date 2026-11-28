import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ChannelInstallations } from "./channelinstallations";
import { MessagingProvidersPanel } from "./messagingproviders";
import { UNUSED_INSTALLATION_ROUTES } from "./settingspages.fixtures";

const meta = {
  title: "Settings/Data/Integrations/Messaging providers",
  component: MessagingProvidersPanel,
  decorators: [
    (Story) => {
      installFetchStub({
        ...UNUSED_INSTALLATION_ROUTES,
        "GET /me": meRoute({
          capture_settings: ["read", "update"],
          channel_connection: ["read", "create", "update"],
        }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
} satisfies Meta<typeof MessagingProvidersPanel>;
export default meta;
type Story = StoryObj<typeof meta>;
export const SetupNeeded: Story = {};

export const SharedChannelSetup: Story = {
  render: () => <ChannelInstallations />,
};

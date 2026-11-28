import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import {
  BusinessReadersDialog,
  ConnectBusinessDialog,
} from "./connectors.businessdialogs";

const meta: Meta<typeof ConnectBusinessDialog> = {
  title: "Settings/Data/Integrations/Business account setup",
  component: ConnectBusinessDialog,
};
export default meta;
type Story = StoryObj<typeof ConnectBusinessDialog>;
const teams = [
  { id: "018f3a1b-0000-7000-8000-0000000000e1", name: "Sales desk" },
];
export const WhatsApp: Story = {
  render: () => (
    <StoryProviders>
      <ConnectBusinessDialog
        provider="whatsapp"
        teams={teams}
        onClose={() => undefined}
        onDone={() => undefined}
      />
    </StoryProviders>
  ),
};
export const Zalo: Story = {
  render: () => (
    <StoryProviders>
      <ConnectBusinessDialog
        provider="zalo-oa"
        teams={teams}
        onClose={() => undefined}
        onDone={() => undefined}
      />
    </StoryProviders>
  ),
};
export const Readers: Story = {
  render: () => (
    <StoryProviders>
      <BusinessReadersDialog
        provider="whatsapp"
        account={{
          id: "wa-demo",
          name: "Sales desk",
          status: "connected",
          mailVisibility: "held",
        }}
        onClose={() => undefined}
        onDone={() => undefined}
      />
    </StoryProviders>
  ),
};

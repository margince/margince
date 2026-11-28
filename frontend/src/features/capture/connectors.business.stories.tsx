import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { BusinessAccountsPanel } from "./connectors.business";

const meta: Meta<typeof BusinessAccountsPanel> = {
  title: "Settings/Data/Integrations/Business accounts",
  component: BusinessAccountsPanel,
};
export default meta;
type Story = StoryObj<typeof BusinessAccountsPanel>;
export const Accounts: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /teams": () =>
          jsonResponse({
            data: [
              {
                id: "018f3a1b-0000-7000-8000-0000000000e1",
                name: "Sales desk",
              },
            ],
            page: {},
          }),
        "GET /channel-connections/whatsapp": () =>
          jsonResponse({
            data: [
              {
                id: "wa-demo",
                displayPhoneNumber: "+49 170 1234567",
                status: "connected",
                readerTeamId: "018f3a1b-0000-7000-8000-0000000000e1",
                mailVisibility: "held",
              },
            ],
          }),
        "GET /channel-connections/zalo-oa": () =>
          jsonResponse({
            data: [
              {
                id: "oa-demo",
                displayName: "Customer desk",
                status: "reauth_required",
                mailVisibility: "held",
              },
            ],
          }),
      },
      { channel_connection: ["read", "create", "update", "delete"] },
    );
    return (
      <StoryProviders>
        <BusinessAccountsPanel provider="whatsapp" />
        <BusinessAccountsPanel provider="zalo-oa" />
      </StoryProviders>
    );
  },
};
export const Empty: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /teams": () => jsonResponse({ data: [], page: {} }),
        "GET /channel-connections/whatsapp": () => jsonResponse({ data: [] }),
      },
      { channel_connection: ["read", "create", "update", "delete"] },
    );
    return (
      <StoryProviders>
        <BusinessAccountsPanel provider="whatsapp" />
      </StoryProviders>
    );
  },
};

export const NewerServerStatus: Story = {
  render: () => {
    stubWithSession(
      {
        "GET /teams": () => jsonResponse({ data: [], page: {} }),
        "GET /channel-connections/whatsapp": () =>
          jsonResponse({
            data: [
              {
                id: "wa-future",
                displayPhoneNumber: "+49 170 1234567",
                status: "awaiting_review",
                mailVisibility: "held",
              },
            ],
          }),
      },
      { channel_connection: ["read", "create", "update", "delete"] },
    );
    return (
      <StoryProviders>
        <BusinessAccountsPanel provider="whatsapp" />
      </StoryProviders>
    );
  },
};

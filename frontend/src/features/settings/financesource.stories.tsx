import type { Meta, StoryObj } from "@storybook/react-vite";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { FinanceSourceCard } from "./financesource";

const page = { next_cursor: null, has_more: false };

const available = [
  {
    provider: "offline_demo",
    description:
      "Demonstration accounting data for the finance card, generated rather than read from a real system.",
  },
];

const connection = {
  id: "0190a5f0-0000-7000-8000-000000000001",
  provider: "offline_demo",
  status: "active",
  connected_at: "2026-08-01T09:00:00Z",
  last_success_at: "2026-08-02T09:00:00Z",
};

const customers = [
  {
    external_id: "DEMO-0001",
    display_name: "Demo customer 01",
    company: { id: "co-1", display_name: "Acme GmbH" },
  },
  { external_id: "DEMO-0002", display_name: "Demo customer 02" },
];

const ADMIN: GrantSpec = {
  integrations: ["read", "create", "update", "delete"],
  company: ["read", "update"],
};

function cardStory(source: unknown, directory: unknown[] = customers) {
  return () => {
    installFetchStub({
      "GET /me": () => jsonResponse(meFixture({ allow: ADMIN })),
      "GET /finance/source": () => jsonResponse(source),
      "GET /finance/source/customers": () =>
        jsonResponse({ data: directory, page }),
    });
    return (
      <StoryProviders>
        <FinanceSourceCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof FinanceSourceCard> = {
  title: "Settings/Data/Integrations/Accounting source",
  component: FinanceSourceCard,
};
export default meta;
type Story = StoryObj<typeof FinanceSourceCard>;

export const Connected: Story = {
  render: cardStory({ available, connection }),
};

export const ConnectedNoCustomersYet: Story = {
  render: cardStory(
    {
      available,
      connection: {
        ...connection,
        status: "connecting",
        last_success_at: null,
      },
    },
    [],
  ),
};

export const SyncFailed: Story = {
  render: cardStory({
    available,
    connection: {
      ...connection,
      status: "error",
      last_error_code: "sync_failed",
    },
  }),
};

export const NotConnected: Story = {
  render: cardStory({ available }),
};

export const NoneInstalled: Story = {
  render: cardStory({ available: [] }),
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../../testing/story-utils";
import {
  ApproveBusinessDialog,
  BusinessVisibilityDialog,
} from "./connectors.telegrambusiness.dialogs";

const meta: Meta<typeof BusinessVisibilityDialog> = {
  title: "Settings/Data/Integrations/Business account readers",
  component: BusinessVisibilityDialog,
};
export default meta;
type Story = StoryObj<typeof BusinessVisibilityDialog>;
export const Held: Story = {
  render: () => (
    <StoryProviders>
      <BusinessVisibilityDialog
        owner="Sales desk"
        mailVisibility="held"
        patch={async () => undefined}
        onClose={() => undefined}
        onDone={() => undefined}
      />
    </StoryProviders>
  ),
};
export const Shared: Story = {
  render: () => (
    <StoryProviders>
      <BusinessVisibilityDialog
        owner="Sales desk"
        mailVisibility="shared"
        patch={async () => undefined}
        onClose={() => undefined}
        onDone={() => undefined}
      />
    </StoryProviders>
  ),
};
export const Approval: Story = {
  render: () => (
    <StoryProviders>
      <ApproveBusinessDialog
        connection={{
          id: "018f3a1b-0000-7000-8000-0000000000b1",
          accountId: "demo-account",
          ownerLabel: "Sales desk",
          status: "pending_approval",
          connectedAt: "2026-03-02T10:00:00Z",
        }}
        teams={[
          { id: "018f3a1b-0000-7000-8000-0000000000e1", name: "Sales desk" },
        ]}
        onClose={() => undefined}
        onDone={() => undefined}
        onSwitchedOff={() => undefined}
      />
    </StoryProviders>
  ),
};

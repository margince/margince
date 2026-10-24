// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  jsonResponse,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { PublicOriginCard } from "./public-origin";

type PublicOrigin = NonNullable<
  components["schemas"]["CaptureConnectionListResponse"]["public_origin"]
>;

// The address emailed links use, as System health reports it: answering, not
// answering, and not asked yet. The badge tone is the whole difference.
function originStory(publicOrigin: PublicOrigin) {
  return () => {
    stubWithSession(
      {
        "GET /connectors": () =>
          jsonResponse({ data: [], public_origin: publicOrigin }),
      },
      { job_health: ["read"] },
    );
    return (
      <StoryProviders>
        <PublicOriginCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof PublicOriginCard> = {
  title: "Settings/Governance/System health/Links in outgoing mail",
  component: PublicOriginCard,
};
export default meta;
type Story = StoryObj<typeof PublicOriginCard>;

const reachable = originStory({
  origin: "https://crm.example.com",
  reachable: true,
  detail: "http 200",
});
const unreachable = originStory({
  origin: "http://localhost:8080",
  reachable: false,
});

export const Reachable: Story = { render: reachable };
export const ReachableDark: Story = {
  globals: { theme: "dark" },
  render: reachable,
};
export const Unreachable: Story = { render: unreachable };
export const UnreachableDark: Story = {
  globals: { theme: "dark" },
  render: unreachable,
};
export const NotChecked: Story = {
  render: originStory({ origin: "https://crm.example.com", reachable: null }),
};

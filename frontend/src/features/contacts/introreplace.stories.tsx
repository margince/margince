// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, waitFor, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { IntroReplacement } from "./introreplace";
import type { IntroRequest } from "./introrequests";

const contact = "018f3a1b-0000-7000-8000-000000000010";
const colleague = "018f3a1b-0000-7000-8000-000000000021";
const request: IntroRequest = {
  id: "018f3a1b-0000-7000-8000-0000000000a1",
  contact_id: contact,
  origin: "asked",
  requester_user_id: "018f3a1b-0000-7000-8000-000000000031",
  introducer_user_id: "018f3a1b-0000-7000-8000-000000000022",
  route_type: "direct",
  status: "requested",
  requested_at: "2026-09-01T09:00:00Z",
  due_at: "2026-09-08T09:00:00Z",
  version: 1,
  note_generated_by: "human",
  note_ai_generated: false,
  name_drop_allowed: false,
  fallback_policy: "none",
};
function chooseRoute() {
  installFetchStub({
    "GET /me": meRoute({
      contact: ["read"],
      introduction: ["read", "create", "update"],
    }),
    [`GET /contacts/${contact}/graph`]: () =>
      jsonResponse({
        contact_id: contact,
        nodes: [],
        edges: [],
        groups_omitted: [],
        routes: [
          {
            route_id: `direct:${colleague}`,
            route_type: "direct",
            via_user_id: colleague,
            via_display_name: "Sofia Meier",
            strength_bucket: "strong",
            availability: "available",
            evidence: {
              interactions_90d: 14,
              inbound_90d: 6,
              outbound_90d: 8,
              two_way: true,
              days_since_last: 4,
            },
          },
        ],
      }),
  });
  return (
    <StoryProviders>
      <IntroReplacement
        contactId={contact}
        contactName="Dana Buyer"
        request={request}
        onClose={() => {}}
      />
    </StoryProviders>
  );
}
const meta: Meta<typeof IntroReplacement> = {
  title: "Records/Contact 360/Network/Replace introduction",
  component: IntroReplacement,
};
export default meta;
type Story = StoryObj<typeof IntroReplacement>;
export const ChooseAFreshRoute: Story = {
  render: chooseRoute,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    const ask = await dialog.findByRole("button", {
      name: "Ask Sofia Meier to introduce you",
    });
    await waitFor(() => expect(ask).toBeVisible());
  },
};
export const ChooseAFreshRouteDark: Story = {
  ...ChooseAFreshRoute,
  globals: { theme: "dark" },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyRail } from "./companyrail";

// Ownership's "Visible to" row on a closed account: who may read it, counting the shares.

const meta: Meta = {
  title: "Records/Company 360/Ownership",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Company = components["schemas"]["Company"];

// The signed-in member of meFixture.
const YOU = "00000000-0000-4000-8000-000000000001";

const company: Company = {
  writable: true,
  id: "o-1",
  workspace_id: "w-1",
  display_name: "Brandt Automotive GmbH",
  status: "customer",
  owner_id: "u-2",
  visibility: "owner",
  captured_by: "human:u-2",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

function Ownership({
  owner,
  shares,
  more = false,
}: Readonly<{ owner: string; shares: number; more?: boolean }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: "u-2", display_name: "Dana Fischer" }],
        page: { has_more: false, next_cursor: null },
      }),
    "GET /users/names": () =>
      jsonResponse({ data: [{ id: "u-2", display_name: "Dana Fischer" }] }),
    "GET /record-grants": () =>
      jsonResponse({
        data: Array.from({ length: shares }, (_, index) => ({
          id: `g-${index}`,
        })),
        page: { has_more: more, next_cursor: more ? "next" : null },
      }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 340 }}>
        <CompanyRail
          company={{ ...company, owner_id: owner }}
          loading={false}
        />
      </div>
    </StoryProviders>
  );
}

async function visibleTo(canvasElement: HTMLElement, reading: RegExp) {
  const ownership = await within(canvasElement).findByRole("region", {
    name: "Ownership",
  });
  await expect(await within(ownership).findByText(reading)).toBeVisible();
}

export const YoursAndShared: Story = {
  render: () => <Ownership owner={YOU} shares={2} />,
  play: async ({ canvasElement }) => {
    await visibleTo(canvasElement, /^You · 2 shares$/);
  },
};

export const SomeoneElsesAlone: Story = {
  render: () => <Ownership owner="u-2" shares={0} />,
  play: async ({ canvasElement }) => {
    await visibleTo(canvasElement, /^Only Dana Fischer$/);
  },
};

// A page the server cut reads as "100+", not as the whole audience.
export const SharesPastOnePage: Story = {
  render: () => <Ownership owner="u-2" shares={100} more />,
  play: async ({ canvasElement }) => {
    await visibleTo(canvasElement, /^Dana Fischer · 100\+ shares$/);
  },
};

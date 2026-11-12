// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { DealLinksCard } from "./deallinkscard";

// The deal's typed links, read from either end, on its Overview.

const DEAL = "01a0e9f1-0000-7000-8000-0000000000a1";

function Served({ writable }: Readonly<{ writable: boolean }>) {
  installFetchStub({
    "GET /me": meRoute({ deal: ["read", "update"] } as never),
    "GET /link-types": () =>
      jsonResponse({
        data: [
          { name: "renews", label: "Renews", from: ["deal"], to: ["deal"] },
        ],
      }),
    "GET /record-links": () =>
      jsonResponse({
        data: [
          {
            id: "01a0e9f1-0000-7000-8000-0000000000b1",
            from_type: "deal",
            from_id: DEAL,
            from_name: "Warehouse rollout",
            to_type: "deal",
            to_id: "01a0e9f1-0000-7000-8000-0000000000a2",
            to_name: "Warehouse renewal",
            link_type: "renews",
            created_at: "2026-10-08T09:00:00Z",
          },
        ],
      }),
  });
  return (
    <StoryProviders>
      <DealLinksCard deal={{ id: DEAL, writable }} />
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Deal/Links",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

/** A reader who may change the deal can add and remove links. */
export const Editable: Story = {
  render: () => <Served writable />,
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByText("Warehouse renewal"),
    ).toBeInTheDocument();
  },
};

/** A read-only reader sees the links and no verbs. */
export const ReadOnly: Story = {
  render: () => <Served writable={false} />,
};

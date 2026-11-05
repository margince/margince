// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { LeadScreen } from "./leads";

// The lead's Deals tab, where the deal it opened and the project it names
// live. It also covers the tags its details reveal behind "Show N empty fields".

type Lead = components["schemas"]["Lead"];

const meta: Meta = {
  title: "Records/Leads/Deals and tags",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const lead = (over: Partial<Lead> = {}): Lead =>
  ({
    id: "l-1",
    full_name: "Jonas Petersen",
    company_name: "Nordwind Logistik",
    status: "contacted",
    score: 72,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...over,
  }) as Lead;

/** The project verb's picker, a search list anchored to the verb. */
export const AttachingAProject: Story = {
  render: () => {
    installFetchStub({
      "GET /leads/l-1": () => jsonResponse(lead({ writable: true })),
      "GET /me": meRoute({ lead: ["read", "update"], project: ["read"] }),
      "GET /projects": () =>
        jsonResponse({
          data: [
            { id: "pr-2", name: "Beacon rollout", key: "BEA" },
            { id: "pr-3", name: "Beacon phase two", key: "BE2" },
          ],
          page: { has_more: false, next_cursor: null },
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByRole("button", { name: "Deals" }));
    const [attach] = await canvas.findAllByRole("button", {
      name: "Attach project",
    });
    if (!attach) throw new Error("the rail shows no Attach project button");
    await userEvent.click(attach);
    const panel = within(await within(document.body).findByRole("dialog"));
    await userEvent.type(panel.getByRole("combobox"), "Beacon");
    await panel.findByRole("option", { name: /Beacon rollout/ });
  },
};

/**
 * The lead's tags, behind "Show N empty fields", applied and removable by a
 * reader who may change the lead.
 */
export const Tagged: Story = {
  render: () => {
    installFetchStub({
      "GET /leads/l-1": () => jsonResponse(lead({ writable: true })),
      "GET /me": meRoute({ lead: ["read", "update"], tag: ["read"] }),
      "GET /records/lead/l-1/tags": () =>
        jsonResponse({
          data: [
            {
              tag_id: "t-1",
              name: "Product A",
              color: "teal",
              archived: false,
              assigned_at: "2026-09-01T08:00:00Z",
            },
          ],
          withheld: false,
        }),
    });
    return (
      <StoryProviders>
        <LeadScreen id="l-1" />
      </StoryProviders>
    );
  },
};

/** The same tags in dark, where the tag pills are re-derived. */
export const TaggedDark: Story = {
  ...Tagged,
  globals: { theme: "dark" },
};

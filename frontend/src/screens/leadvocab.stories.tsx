// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { LeadHandlingCard } from "./leadvocab";
import { LeadDisqualifyReasonsCard } from "./leadvocab.reasons";
import { VocabRowMenu } from "./leadvocab.rows";
import { LeadSourcesCard } from "./leadvocab.sources";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Settings › Data model: where leads come from, why they get dropped, and
// whether the first-response target is tracked. Every role reads the lists;
// the custom_field write verbs decide who may change them.

function source(
  key: string,
  label: string,
  intent: string,
  extra: Record<string, unknown> = {},
) {
  return {
    id: `src-${key}`,
    key,
    label,
    intent,
    sort_order: 10,
    active: true,
    system: false,
    lead_count: 0,
    version: 1,
    created_at: "2026-08-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
    ...extra,
  };
}

const SOURCES = {
  data: [
    source("manual", "Created manually", "neutral", {
      system: true,
      lead_count: 12,
    }),
    source("inbound", "Inbound", "high", { system: true, lead_count: 31 }),
    source("webform", "Web form", "high", { system: true, lead_count: 4 }),
    source("referral", "Referral", "high", { system: true, lead_count: 9 }),
    source("import", "Import", "low", {
      system: true,
      lead_count: 140,
      active: false,
    }),
    source("crawl", "Web research", "low", { system: true, lead_count: 2 }),
    source("trade_show", "Trade show", "high", { lead_count: 3 }),
  ],
  discovered: [{ key: "connector:apollo", lead_count: 27 }],
};

const REASONS = {
  data: [
    ["r1", "Not a good fit", 6],
    ["r2", "Bad timing", 11],
    ["r3", "No budget", 2],
    ["r4", "No decision power", 0],
    ["r5", "Chose a competitor", 1],
    ["r6", "No interest", 4],
    ["r7", "Not reachable", 8],
    ["r8", "Duplicate or spam", 3],
  ].map(([id, label, count]) => ({
    id,
    label,
    sort_order: 10,
    active: true,
    system: true,
    lead_count: count,
    version: 1,
    created_at: "2026-08-01T08:00:00Z",
    updated_at: "2026-08-01T08:00:00Z",
  })),
};

const ADMIN = { custom_field: ["read", "create", "update", "delete"] } as const;
const READER = { custom_field: ["read"] } as const;

const DUPLICATE = {
  type: "about:blank",
  title: "Conflict",
  status: 409,
  code: "conflict",
  detail: "conflict",
};

function story(
  allow: Parameters<typeof meRoute>[0],
  slaOn: boolean,
  refuseReasonName = false,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      ...(refuseReasonName && {
        "POST /lead-disqualify-reasons": () => jsonResponse(DUPLICATE, 409),
      }),
      "GET /lead-sources": () => jsonResponse(SOURCES),
      "GET /lead-disqualify-reasons": () => jsonResponse(REASONS),
      "GET /leads/settings": () =>
        jsonResponse({
          first_response_enabled: slaOn,
          first_response_target_minutes: 240,
        }),
    });
    return (
      <StoryProviders>
        <LeadSourcesCard />
        <LeadDisqualifyReasonsCard />
        <LeadHandlingCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof LeadSourcesCard> = {
  title: "Settings/Sales/Lead handling/Lead vocabularies",
  component: LeadSourcesCard,
};
export default meta;
type Story = StoryObj<typeof LeadSourcesCard>;

export const Admin: Story = { render: story(ADMIN, false) };
export const AdminWithTargetOn: Story = { render: story(ADMIN, true) };
export const Reader: Story = { render: story(READER, false) };

export const AdminDark: Story = {
  globals: { theme: "dark" },
  render: story(ADMIN, true),
};

// A label and a weight, so its form is a dialog behind the header verb.
export const AddingSource: Story = {
  render: story(ADMIN, false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New source" }),
    );
  },
};

// A target outside the 15-minutes-to-7-days window the server enforces.
export const TargetRefused: Story = {
  render: story(ADMIN, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const minutes = await canvas.findByTestId("lead-first-response-target");
    await userEvent.clear(minutes);
    await userEvent.type(minutes, "2");
    await userEvent.tab();
    await canvas.findByRole("alert");
  },
};

// At 390 each row folds: name and key over the count, intent and switch.
export const AdminPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(ADMIN, false),
};

// A built-in keeps Remove in its menu, refused with the reason in words.
export const RemoveRefused: Story = {
  render: story(ADMIN, false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Inbound" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await expect(page.getByRole("button", { name: "Remove" })).toBeDisabled();
  },
};

export const RenamingSource: Story = {
  render: story(ADMIN, false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Trade show" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(page.getByRole("button", { name: "Rename" }));
    await page.findByRole("dialog");
  },
};

// The server refuses a reason that already exists: the dialog says so on the field.
export const DuplicateReason: Story = {
  render: story(ADMIN, false, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "New reason" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    const dialog = within(await page.findByRole("dialog"));
    await userEvent.type(dialog.getByLabelText("Reason"), "Bad timing{Enter}");
    await dialog.findByText(
      "A reason with this name already exists. Choose another name.",
    );
  },
};

export const DuplicateReasonPhone: Story = {
  ...DuplicateReason,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// The shared row menu alone, with an in-use entry's refusal under Remove.
export const RowMenu: Story = {
  render: () => (
    <StoryProviders>
      <VocabRowMenu
        label="Webinar"
        verbs={{ canEdit: true, canRemove: true }}
        refusal="2 leads use this source. Deactivate it instead."
        onRename={() => undefined}
        onRemove={() => undefined}
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Actions for Webinar",
      }),
    );
  },
};

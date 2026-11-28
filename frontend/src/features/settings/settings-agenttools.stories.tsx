// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { pickOption } from "../../design-system/select-testing";
import { en as englishCopy } from "../../i18n/en";
import { vi as vietnameseCopy } from "../../i18n/vi";
import {
  expectFoldedRowsUnderSummary,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AgentToolsCard } from "./settings-agenttools";

// Served descriptions end with the governance clause the server appends, so the
// fixtures carry it. The clamped line and the opened one both have to hold it.
const TOOLS = [
  {
    name: "search_records",
    title: "Search records",
    description: "Find contacts, companies, deals, leads and projects by name.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "send_email",
    title: "Send email",
    description:
      'Put a mail on the wire to a real recipient, exactly as it is given. (Governance: a human approves every call before it runs; requires passport scope "send".)',
    required_scope: "send",
    tier: "confirmation_required",
    egress: true,
  },
  {
    name: "list_pipelines",
    title: "List pipelines and their stages",
    description: "Every pipeline with its live stages.",
    tier: "auto_execute",
    egress: false,
  },
];

const SCOUT = {
  id: "pp-1",
  label: "Scout",
  scopes: ["read"],
  created_at: "2026-07-01T08:00:00Z",
  expires_at: null,
  revoked_at: null,
};

function story(
  passports: readonly Record<string, unknown>[] = [],
  locale: "en" | "vi" = "en",
  tools = TOOLS,
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /agent-tools": () => jsonResponse({ data: tools }),
      "GET /passports": () =>
        jsonResponse({
          data: passports,
          page: { next_cursor: null, has_more: false },
        }),
    });
    return (
      <StoryProviders locale={locale}>
        <AgentToolsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof AgentToolsCard> = {
  title: "Settings/You/AI assistants/Agent tools",
  component: AgentToolsCard,
};
export default meta;
type Story = StoryObj<typeof AgentToolsCard>;

// The groups at rest: one summary per permission, each with its count.
export const Tools: Story = { render: story() };

const openSendGroup = async ({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) => {
  await userEvent.click(
    await within(canvasElement).findByText(englishCopy["passport.scope.send"]),
  );
};

export const ToolsDark: Story = {
  globals: { theme: "dark" },
  render: story(),
};

// Folded, each row's first cell starts where its group's summary starts.
export const ToolsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(),
  play: async (context) => {
    await openSendGroup(context);
    await expectFoldedRowsUnderSummary(context.canvasElement);
  },
};

// Found by its key alone, a closed row shows the key it matched on.
export const SearchByKey: Story = {
  render: story(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("searchbox", {
        name: englishCopy["tools.search"],
      }),
      "search_rec",
    );
    await canvas.findByText("search_records");
  },
};

// A row opened: its whole description, governance clause included, and the key.
export const DescriptionOpen: Story = {
  render: story(),
  play: async (context) => {
    await openSendGroup(context);
    const canvas = within(context.canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", {
        name: "Full description of Send email",
      }),
    );
  },
};

export const SearchNarrows: Story = {
  render: story(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("searchbox", {
        name: englishCopy["tools.search"],
      }),
      "mail",
    );
  },
};

export const SearchFindsNothing: Story = {
  render: story(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(
      await canvas.findByRole("searchbox", {
        name: englishCopy["tools.search"],
      }),
      "invoice",
    );
  },
};

// Scoped to a read-only passport, the send tool is struck and says why.
export const ScopedToPassport: Story = {
  render: story([SCOUT]),
  play: async (context) => {
    const canvas = within(context.canvasElement);
    await pickOption(
      userEvent.setup(),
      await canvas.findByRole("combobox", {
        name: englishCopy["tools.scopeAll"],
      }),
      "Reachable by Scout",
    );
    await openSendGroup(context);
  },
};

const VI_TOOLS = [
  {
    name: "annotate_brief",
    title: "Write findings onto the morning brief",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "archive_record",
    title: "Archive a record",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "bulk_update_records",
    title: "Change many records at once",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "change_access",
    title: "Share records and set up members and teams",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "change_activities",
    title: "Log, plan and file activities",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "change_attention",
    title: "Act on proposals, notices and the weekly plan",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "change_capture",
    title: "Decide what comes into the CRM",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "change_compliance",
    title: "Record consent, stops, holds and privacy work",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "change_lists",
    title: "Make and change lists",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
  {
    name: "change_my_settings",
    title: "Change the user's own preferences and voice",
    description: "Source description supplied to the AI client.",
    required_scope: "read",
    tier: "auto_execute",
    egress: false,
  },
];
export const VietnameseTools: Story = {
  render: story([], "vi", VI_TOOLS),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByText(vietnameseCopy["passport.scope.read"]),
    );
    await canvas.findByText(vietnameseCopy["tools.name.annotate_brief"]);
    await canvas.findByText(vietnameseCopy["tools.name.archive_record"]);
    await canvas.findByText(vietnameseCopy["tools.name.bulk_update_records"]);
    await canvas.findByText(vietnameseCopy["tools.name.change_access"]);
    await canvas.findByText(vietnameseCopy["tools.name.change_activities"]);
    await canvas.findByText(vietnameseCopy["tools.name.change_attention"]);
    await canvas.findByText(vietnameseCopy["tools.name.change_capture"]);
    await canvas.findByText(vietnameseCopy["tools.name.change_compliance"]);
    await canvas.findByText(vietnameseCopy["tools.name.change_lists"]);
    await canvas.findByText(vietnameseCopy["tools.name.change_my_settings"]);
  },
};
export const VietnameseSearch: Story = {
  render: story([], "vi", [
    {
      description: "Find records by name.",
      required_scope: "read",
      tier: "auto_execute",
      egress: false,
      name: "find_records",
      title: "Find records",
    },
  ]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.type(await canvas.findByRole("searchbox"), "find_records");
    await userEvent.click(
      await canvas.findByRole("button", {
        name: vietnameseCopy["tools.descriptionToggle"].replace(
          "{tool}",
          vietnameseCopy["tools.name.find_records"],
        ),
      }),
    );
    await canvas.findByText(
      `${vietnameseCopy["tools.sourceTitle"]}: Find records`,
      { exact: true },
    );
  },
};

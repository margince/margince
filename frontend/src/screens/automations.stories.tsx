// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { AutomationsAdmin } from "./automations";
import {
  AUTOMATION_CATALOG,
  configuredAutomations,
} from "./automations.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The whole automations card: the configured rules as one table, then the
// starter library, each a group of the one pane.

const meta: Meta = {
  title: "Settings/AI/Automations/Automations",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const CARD_ROUTES = {
  "GET /me": meRoute({ automation: ["create", "read", "update", "delete"] }),
  "GET /automations/catalog": () => jsonResponse({ data: AUTOMATION_CATALOG }),
  "GET /automations": () =>
    jsonResponse({
      data: configuredAutomations(Date.now()),
      page: { next_cursor: null },
    }),
};

function Card({ readOnly = false }: Readonly<{ readOnly?: boolean }>) {
  installFetchStub(
    readOnly
      ? { ...CARD_ROUTES, "GET /me": meRoute({ automation: ["read"] }) }
      : CARD_ROUTES,
  );
  return (
    <StoryProviders>
      <AutomationsAdmin />
    </StoryProviders>
  );
}

const cardRendered: NonNullable<Story["play"]> = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await canvas.findByTestId("automation-au-6");
  await canvas.findByTestId("template-no_activity_reminder");
};

export const AdminCard: Story = {
  play: cardRendered,
  render: () => <Card />,
};

export const AdminCardDark: Story = {
  globals: { theme: "dark" },
  play: cardRendered,
  render: () => <Card />,
};

export const AdminCardPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: cardRendered,
  render: () => <Card />,
};

// The library stays readable without the create grant; its rows stop opening,
// and the table's switches give way to words.
export const AdminCardReadOnly: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByTestId("template-no_activity_reminder");
    await expect(
      canvas.queryByRole("button", { name: "Use template" }),
    ).toBeNull();
  },
  render: () => <Card readOnly />,
};

// A press anywhere on a template's row opens the create dialog seeded from it.
export const CreatingFromATemplate: Story = {
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    const row = await canvas.findByTestId("template-renewal_reminder");
    await user.click(within(row).getByText("Renewal reminder"));
    await within(canvasElement.ownerDocument.body).findByRole("dialog");
  },
  render: () => <Card />,
};

// A rule's Edit verb opens its definition in a dialog over the card.
export const EditingInADialog: Story = {
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", {
        name: "Actions for Renewal reminder",
      }),
    );
    const body = within(canvasElement.ownerDocument.body);
    await user.click(await body.findByRole("button", { name: "Edit" }));
    await body.findByRole("dialog");
  },
  render: () => <Card />,
};

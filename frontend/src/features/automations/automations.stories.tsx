// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AutomationsAdmin, type AutomationsView } from "./automations";
import {
  AUTOMATION_CATALOG,
  configuredAutomations,
} from "./automations.fixtures";

// The automations page, one tab at a time: the configured rules, or the
// starter library.

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

function Card({
  readOnly = false,
  none = false,
  view = "mine",
}: Readonly<{ readOnly?: boolean; none?: boolean; view?: AutomationsView }>) {
  const routes = none
    ? {
        ...CARD_ROUTES,
        "GET /automations": () =>
          jsonResponse({ data: [], page: { next_cursor: null } }),
      }
    : CARD_ROUTES;
  installFetchStub(
    readOnly
      ? { ...routes, "GET /me": meRoute({ automation: ["read"] }) }
      : routes,
  );
  return (
    <StoryProviders>
      <AutomationsAdmin view={view} />
    </StoryProviders>
  );
}

const cardRendered: NonNullable<Story["play"]> = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await canvas.findByTestId("automation-au-6");
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

// Nothing configured yet: the intro and one empty line share one body.
export const NoneConfigured: Story = {
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByText(en["auto.instances.empty"]);
  },
  render: () => <Card none />,
};

export const AdminCardPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: cardRendered,
  render: () => <Card />,
};

export const Library: Story = {
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByTestId("template-no_activity_reminder");
  },
  render: () => <Card view="library" />,
};

// The library stays readable without the create grant; its rows stop opening.
export const AdminCardReadOnly: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByTestId("template-no_activity_reminder");
    await expect(
      canvas.queryByRole("button", { name: /^Use template/ }),
    ).toBeNull();
  },
  render: () => <Card readOnly view="library" />,
};

export const CreatingFromATemplate: Story = {
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    const row = await canvas.findByTestId("template-renewal_reminder");
    await user.click(within(row).getByText("Renewal reminder"));
    await within(canvasElement.ownerDocument.body).findByRole("dialog");
  },
  render: () => <Card view="library" />,
};

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

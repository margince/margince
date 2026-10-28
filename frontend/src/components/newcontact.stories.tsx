// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import type { HomeCompany } from "./contactformfields";
import { NewContactAction } from "./newcontact";

// The full contact form, opened the way a reader opens it. The stub answers
// every company read with one company, so any work email prefills it.

function form(company?: HomeCompany, holder = "Nordlicht Logistik AG") {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ contact: ["read", "create"] }),
      "GET /companies": () =>
        jsonResponse({
          ...emptyPage,
          data: [
            {
              id: "01930000-0000-7000-8000-0000000000o2",
              display_name: holder,
            },
          ],
        }),
    });
    return (
      <StoryProviders>
        <NewContactAction company={company} />
      </StoryProviders>
    );
  };
}

type Canvas = { canvasElement: HTMLElement };

async function openWith(
  user: ReturnType<typeof userEvent.setup>,
  { canvasElement }: Canvas,
) {
  await user.click(
    await within(canvasElement).findByRole("button", { name: "New contact" }),
  );
}

async function open(context: Canvas) {
  await openWith(userEvent.setup(), context);
}

async function typeWorkEmail(context: Canvas) {
  const user = userEvent.setup();
  await openWith(user, context);
  const body = within(document.body);
  await user.type(await body.findByLabelText("Full name *"), "Dana Buyer");
  await user.click(body.getByText("Add email"));
  await user.type(
    body.getByLabelText("Email *"),
    "dana.buyer@nordlicht-logistik.de",
  );
  await expect(
    await body.findByText(
      "Matched to an existing company by the work email domain.",
    ),
  ).toBeVisible();
}

const meta: Meta = {
  title: "Records/Contacts/New contact",
};
export default meta;

type Story = StoryObj;

export const CompanyField: Story = { render: form(), play: open };

export const PrefilledFromWorkEmail: Story = {
  render: form(),
  play: typeWorkEmail,
};

export const PrefilledFromWorkEmailDark: Story = {
  ...PrefilledFromWorkEmail,
  globals: { theme: "dark" },
};

export const PrefilledFromWorkEmailPhone: Story = {
  ...PrefilledFromWorkEmail,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const LongCompanyNamePhone: Story = {
  render: form(
    undefined,
    "Nordlicht Logistik und Lagerhaltung Gesellschaft mit beschränkter Haftung",
  ),
  play: typeWorkEmail,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// On a company's page the company is already chosen, so the box is left out.
export const OnCompanyPage: Story = {
  render: form({
    id: "01930000-0000-7000-8000-0000000000o2",
    listKey: "company-contacts:01930000-0000-7000-8000-0000000000o2",
  }),
  play: open,
};

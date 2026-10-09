// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { ContactsTagOffer, EmployerTagOffer } from "./tagfollow";

// The offers that follow an applied tag: to the contact's company, and to the
// company's contacts.

const meta: Meta = {
  title: "Records/Record 360/Tag follow-up offers",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const COMPANY = "01a06151-0000-7000-8000-0000000000a1";
const TAG = { id: "t-1", name: "Product X" };

function Served({ children }: Readonly<{ children: React.ReactNode }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["update"], contact: ["update"] } as never),
    [`GET /companies/${COMPANY}`]: () =>
      jsonResponse({ id: COMPANY, name: "Demo GmbH", writable: true }),
    [`GET /records/company/${COMPANY}/tags`]: () =>
      jsonResponse({ data: [], withheld: false }),
    "GET /contacts": () =>
      jsonResponse({
        data: [
          { id: "c-1", full_name: "Anna Weber", version: 3, tags: [] },
          {
            id: "c-2",
            full_name: "Clara Ruiz",
            version: 2,
            tags: [{ tag_id: "t-1", name: "Product X" }],
          },
        ],
        page: { has_more: false, next_cursor: null },
      }),
  });
  return (
    <StoryProviders>
      <ToastProvider>
        {children}
        <ToastRegion />
      </ToastProvider>
    </StoryProviders>
  );
}

/** After tagging a contact: one press tags their company too. */
export const ToCompany: Story = {
  render: () => (
    <Served>
      <EmployerTagOffer
        employer={{ company_id: COMPANY, company_name: "Demo GmbH" }}
        tag={TAG}
        onClose={() => {}}
      />
    </Served>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByRole("button", {
        name: "Tag Demo GmbH",
      }),
    ).toBeInTheDocument();
  },
};

/** After tagging a company: the reader ticks which contacts get it. */
export const ToContacts: Story = {
  render: () => (
    <Served>
      <ContactsTagOffer companyID={COMPANY} tag={TAG} onClose={() => {}} />
    </Served>
  ),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    await user.click(
      await within(canvasElement).findByRole("button", {
        name: "Choose contacts",
      }),
    );
    await expect(
      await within(document.body).findByRole("checkbox", {
        name: "Clara Ruiz (already tagged)",
      }),
    ).toBeDisabled();
  },
};

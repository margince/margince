// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { RecordDetails } from "../../design-system/recorddetails";
import { useT } from "../../i18n";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { EmailValue, reachRows } from "./leadreach";

// The address and number of a lead share one reach card; an unset one reads "Not set".

const meta: Meta = {
  title: "Records/Leads/Reach",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Lead = components["schemas"]["Lead"];

const lead: Lead = {
  id: "l-1",
  full_name: "Jonas Petersen",
  email: "jonas@nordwind.example",
  phone: "+49 30 1234567",
  status: "contacted",
  score: 72,
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

function ReachRows({ shown }: Readonly<{ shown: Lead }>) {
  const t = useT();
  const rows = reachRows(shown, t, {
    email: <EmailValue lead={shown} />,
    phone: <span>{shown.phone}</span>,
  });
  return <RecordDetails sections={[{ key: "reach", label: "Reach", rows }]} />;
}

function reach(shown: Lead) {
  installFetchStub({});
  return (
    <StoryProviders>
      <div style={{ maxWidth: 340 }}>
        <ReachRows shown={shown} />
      </div>
    </StoryProviders>
  );
}

// The address is the door to writing; the glyph beside it copies it.
export const EmailAndPhone: Story = {
  render: () => reach(lead),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("link", { name: "jonas@nordwind.example" }),
    ).toBeVisible();
    await expect(
      canvas.getByRole("button", { name: "Copy email address" }),
    ).toBeVisible();
    await expect(canvas.getByText("+49 30 1234567")).toBeVisible();
    await expect(canvas.queryByText("Not set")).toBeNull();
  },
};

// A number alone leaves the address as its own empty row.
export const EmailUnset: Story = {
  render: () => reach({ ...lead, email: undefined }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(await canvas.findByText("+49 30 1234567")).toBeVisible();
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Show \d+ empty fields?$/ }),
    );
    await expect(await canvas.findByText("Not set")).toBeVisible();
  },
};

// A lead with neither folds both rows under the empty-fields toggle.
export const NothingSet: Story = {
  render: () => reach({ ...lead, email: undefined, phone: undefined }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: /^Show \d+ empty fields?$/ }),
    );
    await expect(await canvas.findAllByText("Not set")).toHaveLength(2);
    await expect(canvas.getByText("Email")).toBeVisible();
    await expect(canvas.getByText("Phone")).toBeVisible();
  },
};

// A long address must stay inside the card on a phone.
export const EmailAndPhonePhone: Story = {
  name: "Email and phone (phone)",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () =>
    reach({
      ...lead,
      email: "jonas.petersen.logistik@nordwind-logistik.example",
    }),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("link", {
        name: "jonas.petersen.logistik@nordwind-logistik.example",
      }),
    ).toBeVisible();
    await expect(canvas.getByText("+49 30 1234567")).toBeVisible();
  },
};

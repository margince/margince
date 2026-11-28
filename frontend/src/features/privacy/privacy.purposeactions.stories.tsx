// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { StoryProviders, stubWithSession } from "../../testing/story-utils";
import { ConsentPurposesCard } from "./privacy.purposes";

const purpose = {
  id: "p1",
  key: "product_news",
  label: "Product news",
  requires_double_opt_in: false,
};
function purposeActions(refusal: "used" | "failure" | null = null) {
  return () => {
    stubWithSession(
      {
        "GET /consent-purposes": () =>
          Response.json({ data: [purpose], page: { has_more: false } }),
        "PATCH /consent-purposes/p1": () =>
          refusal === "used"
            ? Response.json(
                {
                  status: 409,
                  code: "consent_purpose_in_use",
                  title: "Conflict",
                  detail: "Consent is recorded.",
                },
                { status: 409 },
              )
            : refusal === "failure"
              ? Response.json(
                  {
                    status: 500,
                    code: "internal",
                    title: "Internal Server Error",
                    detail: "Save failed.",
                  },
                  { status: 500 },
                )
              : Response.json({ ...purpose, label: "Product updates" }),
        "DELETE /consent-purposes/p1": () =>
          new Response(null, { status: 204 }),
      },
      { contact: ["read"], consent_config: ["update"] },
    );
    return (
      <StoryProviders>
        <ConsentPurposesCard />
      </StoryProviders>
    );
  };
}
const meta: Meta<typeof ConsentPurposesCard> = {
  title: "Settings/Compliance/Privacy and retention/Purpose actions",
  component: ConsentPurposesCard,
};
export default meta;
type Story = StoryObj<typeof ConsentPurposesCard>;
async function open(action: "Rename" | "Archive", canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await canvas.findByText(purpose.label);
  await userEvent.click(canvas.getByRole("button", { name: "Actions" }));
  await userEvent.click(
    within(document.body).getByRole("button", { name: action }),
  );
  return await within(document.body).findByRole("dialog");
}
export const Rename: Story = {
  render: purposeActions(),
  play: async ({ canvasElement }) => {
    await open("Rename", canvasElement);
  },
};
export const Archive: Story = {
  render: purposeActions(),
  play: async ({ canvasElement }) => {
    await open("Archive", canvasElement);
  },
};
async function refusedRename(canvasElement: HTMLElement) {
  const dialog = within(await open("Rename", canvasElement));
  const label = dialog.getByRole("textbox", { name: "Label" });
  await userEvent.clear(label);
  await userEvent.type(label, "Product updates");
  await userEvent.click(dialog.getByRole("button", { name: "Save" }));
  return dialog;
}
export const InUse: Story = {
  render: purposeActions("used"),
  play: async ({ canvasElement }) => {
    const dialog = await refusedRename(canvasElement);
    await expect(
      dialog.findByText(/Consent is already recorded/),
    ).resolves.toBeTruthy();
  },
};
export const SaveError: Story = {
  render: purposeActions("failure"),
  play: async ({ canvasElement }) => {
    const dialog = await refusedRename(canvasElement);
    await expect(dialog.findByRole("alert")).resolves.toBeTruthy();
  },
};

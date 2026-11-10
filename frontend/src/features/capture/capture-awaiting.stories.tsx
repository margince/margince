// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { AwaitingContacts } from "./capture-awaiting";

// The waiting drawer with one contact per state, so whose move it is reads in
// one frame: three wait on Margince, two on the owner.

type AwaitingContact = components["schemas"]["CaptureAwaitingContact"];

function waiting(
  n: number,
  name: string,
  addresses: AwaitingContact["addresses"],
): AwaitingContact {
  return {
    contact_id: `0190b7a2-0000-7000-8000-00000000004${n}`,
    display_name: name,
    emails: addresses.map((a) => a.email),
    addresses,
    captured_at: "2026-09-10T14:40:00Z",
  };
}

const ROWS: AwaitingContact[] = [
  waiting(1, "Jana Keller", [
    { email: "jana@commercetools.com", state: "unsure" },
    { email: "j.keller@private.example", state: "unjudgeable" },
  ]),
  waiting(2, "Marta Ortiz", [
    { email: "m.ortiz@steireif.de", state: "classifying" },
  ]),
  waiting(3, "Test Sender", [
    { email: "test@demo.example", state: "unjudgeable" },
  ]),
  waiting(4, "Paul Brandt", [
    { email: "paul@brandt.example", state: "queued" },
  ]),
  waiting(5, "Ana Silva", [{ email: "ana@silva.example", state: "decided" }]),
];

const meta = {
  title: "Settings/You/Connections/Waiting contacts",
  render: () => {
    installFetchStub({ "GET /me": meRoute({}) });
    return (
      <StoryProviders>
        <AwaitingContacts rows={ROWS} />
      </StoryProviders>
    );
  },
} satisfies Meta;
export default meta;
type Story = StoryObj<typeof meta>;

export const EveryState: Story = {};

// A contact with two addresses offers each answer per address, plus removing
// the contact, which decides no sender.
export const AnswersPerAddress: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Actions for Jana Keller" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await expect(
      await page.findByRole("button", {
        name: "Exclude j.keller@private.example",
      }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Remove from contacts" }),
    ).toBeVisible();
  },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { HandoverOffer } from "./handoveroffer";

// The owner offering their company to a colleague, as it opens inside the
// access panel: the colleague picker, a note, and the submit row.
const meta: Meta = {
  title: "Records/Record 360/Hand-over offer",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const COMPANY = {
  id: "01a05500-0000-7000-8000-0000000000c1",
  owner_id: meFixture().user.id,
  writable: true,
  archived_at: null,
};

function Offer({ colleague }: Readonly<{ colleague: string }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
    [`GET /records/company/${COMPANY.id}/ownership-offer`]: () =>
      jsonResponse({ status: 404, title: "No open offer." }, 404),
    [`GET /records/company/${COMPANY.id}/ownership-offer/candidates`]: () =>
      jsonResponse({
        data: [
          {
            id: "01a05500-0000-7000-8000-0000000000e2",
            display_name: colleague,
          },
        ],
      }),
  });
  return (
    <StoryProviders>
      <div style={{ maxInlineSize: "22rem" }}>
        <HandoverOffer kind="company" record={COMPANY} />
      </div>
    </StoryProviders>
  );
}

async function openForm() {
  await userEvent.click(
    await screen.findByRole("button", { name: "Hand over to…" }),
  );
  const send = await screen.findByRole("button", { name: "Send offer" });
  const cancel = screen.getByRole("button", { name: "Cancel" });
  // The submit row keeps the standard gap: the two verbs never touch.
  await expect(
    send.getBoundingClientRect().left - cancel.getBoundingClientRect().right,
  ).toBeGreaterThan(0);
}

/** The form open, with Cancel and Send offer apart in the submit row. */
export const Open: Story = {
  render: () => <Offer colleague="Mira Voss" />,
  play: openForm,
};

/** A colleague whose name holds no space still leaves the row in place. */
export const LongColleagueName: Story = {
  render: () => (
    <Offer colleague="Maximiliane-Oberhauser-Wittgenstein-Papadopoulos-Brandt" />
  ),
  play: openForm,
};

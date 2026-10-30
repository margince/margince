// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { ConfirmDetailsAction } from "./confirmdetailsaction";
import { mailPreview } from "./controllermail.fixtures";

// The ask on a contact's consent panel: it opens the mail first, and sends it
// only from the preview's confirm.

const CONTACT = "contact-1";
const SEAT = meFixture({ allow: { contact: ["read", "update"] } });

const SENDS: RouteMap = {
  [`GET /contacts/${CONTACT}/consent/mail-preview`]: () =>
    jsonResponse(mailPreview("record_confirmation")),
  [`POST /contacts/${CONTACT}/consent/confirm-request`]: () =>
    jsonResponse(
      {
        delivered_to: "lena@kps.test",
        expires_at: "2026-11-27T09:00:00Z",
        queued: true,
        sendable: true,
      },
      201,
    ),
};

function action(
  me: object,
  props: Partial<Parameters<typeof ConfirmDetailsAction>[0]> = {},
) {
  return () => {
    installFetchStub({ "GET /me": () => jsonResponse(me), ...SENDS });
    return (
      <StoryProviders>
        <ConfirmDetailsAction
          contactId={CONTACT}
          mayWrite
          recipient="lena@kps.test"
          {...props}
        />
      </StoryProviders>
    );
  };
}

// The ask is refused until /me says the installation can send. Leaving that
// state remounts the button, so the enabled one is found afresh.
async function askWhenSendable(canvasElement: HTMLElement) {
  const canvas = within(canvasElement);
  await waitFor(
    () => expect(canvas.getByTestId("confirm-details-ask")).toBeEnabled(),
    { timeout: 10_000 },
  );
  return canvas.getByTestId("confirm-details-ask");
}

const meta: Meta<typeof ConfirmDetailsAction> = {
  title: "Records/Contact 360/Ask to confirm details",
  component: ConfirmDetailsAction,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ConfirmDetailsAction>;

export const Ready: Story = { render: action(SEAT) };

export const PreviewOpen: Story = {
  render: action(SEAT),
  play: async ({ canvasElement }) => {
    await userEvent.click(await askWhenSendable(canvasElement));
    await within(await screen.findByRole("dialog")).findByText(
      "Ihre Kontaktdaten prüfen",
    );
  },
};

export const PreviewOpenPhone: Story = {
  ...PreviewOpen,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** Sent from the preview: the queued sentence and the link's expiry. */
export const Sent: Story = {
  render: action(SEAT),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await askWhenSendable(canvasElement));
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByText("Ihre Kontaktdaten prüfen");
    await userEvent.click(dialog.getByRole("button", { name: "Send request" }));
    await canvas.findByTestId("confirm-details-sent");
  },
};

/** An installation with no mail lane: the ask is refused with its reason. */
export const NoSystemMail: Story = {
  render: action({ ...SEAT, confirm_requests_sendable: false }),
};

/** A contact with no address: the caller's reason refuses the ask. */
export const NoAddress: Story = {
  render: action(SEAT, {
    recipient: null,
    unavailableReason: en["contact.rail.noEmailAddress"],
  }),
};

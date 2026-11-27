// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import {
  jsonResponse,
  type RouteMap,
  StoryProviders,
  stubWithSession,
} from "../../testing/story-utils";
import { ConnectSharedMailboxDialog } from "./sharedmailboxes.forms";

// The connect dialog on its own, as it opens and as each refusal shows.

const TEAMS = [
  { id: "018f3a1b-0000-7000-8000-0000000000e1", name: "Sales desk" },
];

function dialog(routes: RouteMap = {}) {
  return () => {
    stubWithSession(routes, {}, { roles: ["admin"] });
    return (
      <StoryProviders>
        <ConnectSharedMailboxDialog teams={TEAMS} onClose={() => {}} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof ConnectSharedMailboxDialog> = {
  title: "Settings/Data/Integrations/Connect a shared mailbox",
  component: ConnectSharedMailboxDialog,
};
export default meta;
type Story = StoryObj<typeof ConnectSharedMailboxDialog>;

export const Opened: Story = { render: dialog() };

/** An address and a port it cannot send, each said on its own field. */
export const FieldsRefused: Story = {
  render: dialog(),
  play: async () => {
    const box = within(await screen.findByRole("dialog"));
    await userEvent.type(
      box.getByRole("textbox", {
        name: englishCopy["sharedMailboxes.address"],
      }),
      "not-an-email",
    );
    const port = box.getByRole("textbox", { name: "Port" });
    await userEvent.clear(port);
    await userEvent.type(port, "abc");
    await userEvent.click(
      box.getByRole("button", { name: englishCopy["sharedMailboxes.connect"] }),
    );
    const sentence = await box.findByText("Enter a whole number for Port.");
    await waitFor(() => expect(sentence).toBeVisible());
    await expect(port).toHaveAttribute("aria-invalid", "true");
  },
};

/** The mail server refused the login: said at the foot, the secret emptied. */
export const LoginRefused: Story = {
  render: dialog({
    "POST /shared-mailboxes": () =>
      jsonResponse(
        {
          title: "Unprocessable Entity",
          status: 422,
          code: "imap_login_rejected",
          detail: "The mailbox rejected these credentials.",
        },
        422,
      ),
  }),
  play: async () => {
    const box = within(await screen.findByRole("dialog"));
    await userEvent.type(
      box.getByRole("textbox", {
        name: englishCopy["sharedMailboxes.address"],
      }),
      "sales@acme.example",
    );
    await userEvent.click(
      box.getByRole("combobox", { name: /Team that reads it/ }),
    );
    await userEvent.click(
      await screen.findByRole("option", { name: "Sales desk" }),
    );
    await userEvent.type(
      box.getByRole("textbox", { name: "IMAP server" }),
      "imap.acme.example",
    );
    await userEvent.type(
      box.getByRole("textbox", {
        name: englishCopy["connectors.imapUsername"],
      }),
      "sales@acme.example",
    );
    await userEvent.type(
      box.getByLabelText(englishCopy["connectors.imapSecret"], {
        exact: false,
      }),
      "app-password",
    );
    await userEvent.click(
      box.getByRole("button", { name: englishCopy["sharedMailboxes.connect"] }),
    );
    const sentence = await box.findByText(/rejected these credentials/);
    await waitFor(() => expect(sentence).toBeVisible());
  },
};

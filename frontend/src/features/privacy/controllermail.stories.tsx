// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import {
  ControllerMailDialog,
  type ControllerMailKind,
} from "./controllermail";
import {
  mailPreview,
  NO_ADDRESS_PROBLEM,
  NO_MAIL_LANE_PROBLEM,
} from "./controllermail.fixtures";

// The mail the installation's relay would send a contact, before the rep sends
// it. The stub answers one preview per story, so each language is its own story.

type MailPreview = components["schemas"]["ControllerMailPreview"];

function dialog(
  kind: ControllerMailKind,
  preview: () => Response | Promise<Response>,
) {
  return () => {
    installFetchStub({
      "GET /contacts/contact-1/consent/mail-preview": preview,
    });
    return (
      <StoryProviders>
        <ControllerMailDialog
          contactId="contact-1"
          kind={kind}
          open
          onClose={() => {}}
          onSent={() => {}}
        />
      </StoryProviders>
    );
  };
}

const answer = (preview: MailPreview) => () => jsonResponse(preview);

const meta: Meta<typeof ControllerMailDialog> = {
  title: "Records/Contact 360/Controller mail preview",
  component: ControllerMailDialog,
};
export default meta;
type Story = StoryObj<typeof ControllerMailDialog>;

/** The privacy notice in the installation's language, which the picker marks. */
export const PrivacyNotice: Story = {
  render: dialog("privacy_notice", answer(mailPreview("privacy_notice"))),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await body.findByText(
      "Informationen zur Verarbeitung Ihrer personenbezogenen Daten",
    );
    await expect(
      body.getByText("The installation’s mail language is Deutsch."),
    ).toBeVisible();
  },
};

export const ConfirmationRequestEnglish: Story = {
  render: dialog(
    "record_confirmation",
    answer(mailPreview("record_confirmation", "en")),
  ),
};

export const ConfirmationRequestVietnamese: Story = {
  render: dialog(
    "record_confirmation",
    answer(mailPreview("record_confirmation", "vi")),
  ),
};

/** The picker open over the languages the server returned. */
export const LanguagePicker: Story = {
  render: dialog(
    "record_confirmation",
    answer(mailPreview("record_confirmation")),
  ),
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("combobox", { name: "Language" }),
    );
    await body.findByRole("option", { name: "Tiếng Việt" });
  },
};

const LONG = mailPreview("privacy_notice", "de", {
  to_address:
    "lena.hoffmann-vonderheydt.einkauf.niederlassung-sued@kps-industrieservice.test",
  subject:
    "Informationen zur Verarbeitung Ihrer personenbezogenen Daten durch die KPS Industrieservice GmbH und ihre verbundenen Unternehmen",
  text: `${mailPreview("privacy_notice", "de").text}\n\n${"Verantwortlich ist die KPS Industrieservice GmbH. ".repeat(12)}`,
});

export const LongSubjectAndText: Story = {
  render: dialog("privacy_notice", answer(LONG)),
};

export const LongSubjectAndTextPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: dialog("privacy_notice", answer(LONG)),
};

/** An installation whose relay names no sender. */
export const NoSender: Story = {
  render: dialog(
    "record_confirmation",
    answer(mailPreview("record_confirmation", "de", { from_address: null })),
  ),
};

/** A contact with no live address: the refusal, and a send that is refused. */
export const NoAddress: Story = {
  render: dialog("record_confirmation", () =>
    jsonResponse(NO_ADDRESS_PROBLEM, 422),
  ),
};

export const NoAddressPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: dialog("record_confirmation", () =>
    jsonResponse(NO_ADDRESS_PROBLEM, 422),
  ),
};

/** An installation with no mail lane: the same refusal the send would give. */
export const NoMailLane: Story = {
  render: dialog("privacy_notice", () =>
    jsonResponse(NO_MAIL_LANE_PROBLEM, 409),
  ),
};

export const Loading: Story = {
  render: dialog("privacy_notice", () => new Promise(() => {})),
};

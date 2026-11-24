// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent } from "storybook/test";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { SignatureTemplateCard } from "./signaturetemplatecard";

// One signature layout for the whole workspace, with the reader's own
// signature rendered from it underneath.

const TEMPLATE = "<p><b>{name}</b><br>{title}<br>{phone}</p>";

function story(allow: Parameters<typeof meRoute>[0]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /email-signature-template": () =>
        jsonResponse({ template: TEMPLATE }),
      "GET /me/email-signature": () =>
        jsonResponse({
          body: "",
          title: "Head of Sales",
          phone: "+49 711 555 0100",
          template_active: true,
        }),
      "POST /emails:sign-off": () =>
        jsonResponse({
          text: "Mira Voss",
          html: "<p><b>Mira Voss</b><br>Head of Sales<br>+49 711 555 0100</p>",
          kind: "template",
        }),
    });
    return (
      <StoryProviders>
        <SavePageProvider>
          <SignatureTemplateCard />
          <PageSaveBar />
        </SavePageProvider>
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Company/Company profile/Email signature template",
  component: SignatureTemplateCard,
  parameters: { layout: "padded" },
} satisfies Meta<typeof SignatureTemplateCard>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Editable: Story = {
  render: story({ installation_settings: ["read", "update"] }),
};

// A typed layout is an unsaved change: the page's save bar offers Save and Discard.
export const EditedOnSaveBar: Story = {
  render: story({ installation_settings: ["read", "update"] }),
  play: async () => {
    const box = await screen.findByRole("textbox", { name: "Template (HTML)" });
    await userEvent.type(box, " Gradion");
    await expect(await screen.findByText("Unsaved changes")).toBeTruthy();
  },
};

// The template stays readable, and the panel says once why it cannot change.
export const ReadOnly: Story = {
  render: story({ installation_settings: ["read"] }),
};

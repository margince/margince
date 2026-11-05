// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Panel } from "../../design-system/panel";
import { SettingList } from "../../design-system/settingrow";
import { useToast } from "../../design-system/toast";
import { type Locale, translate } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { SignatureSettingRow } from "./settingssignaturerow";

function SignatureRow() {
  const toast = useToast();
  return (
    <Panel>
      <SettingList bleed="settings">
        <SignatureSettingRow toast={toast} />
      </SettingList>
    </Panel>
  );
}
function signatureStory(html = "", locale: Locale = "en") {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /me/email-signature": () =>
        jsonResponse({
          body: "Mira Voss",
          html,
          title: "Head of Sales",
          phone: "+49 711 555 0100",
          template_active: true,
        }),
      "POST /emails:sign-off": () =>
        jsonResponse({
          text: "Mira Voss",
          html:
            html ||
            "<p><b>Mira Voss</b><br>Head of Sales<br>+49 711 555 0100</p>",
          kind: html ? "signature" : "template",
        }),
    });
    return (
      <StoryProviders locale={locale}>
        <SignatureRow />
      </StoryProviders>
    );
  };
}
const render = signatureStory();
const meta = {
  title: "Settings/You/Account/Signature row",
  component: SignatureRow,
  parameters: { layout: "padded" },
} satisfies Meta<typeof SignatureRow>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Saved: Story = { render };
export const TemplateEditor: Story = {
  render,
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Edit signature",
      }),
    );
  },
};

export const TemplateEditorDark: Story = {
  ...TemplateEditor,
  globals: { theme: "dark" },
};

function ownHtmlEditor(locale: Locale): Story {
  return {
    render: signatureStory("<p><b>Mira Voss</b><br>Head of Sales</p>", locale),
    play: async ({ canvasElement }) => {
      await userEvent.click(
        await within(canvasElement).findByRole("button", {
          name: translate(locale, "settings.signatureEdit"),
        }),
      );
      const dialog = await within(document.body).findByRole("dialog");
      const markup = await within(dialog).findByRole("textbox", {
        name: translate(locale, "settings.signatureHtmlLabel"),
      });
      await expect(markup).toHaveValue(
        "<p><b>Mira Voss</b><br>Head of Sales</p>",
      );
      const frame = await within(dialog).findByTitle(
        translate(locale, "settings.signaturePreview"),
      );
      await expect(frame.getAttribute("srcdoc")).toContain("<b>Mira Voss</b>");
    },
  };
}
export const PersonalHtml: Story = ownHtmlEditor("en");
export const PersonalHtmlDark: Story = {
  ...PersonalHtml,
  globals: { theme: "dark" },
};
export const PersonalHtmlGerman: Story = ownHtmlEditor("de");
export const PersonalHtmlGermanDark: Story = {
  ...PersonalHtmlGerman,
  globals: { theme: "dark" },
};
export const PersonalHtmlVietnamese: Story = ownHtmlEditor("vi");
export const PersonalHtmlVietnameseDark: Story = {
  ...PersonalHtmlVietnamese,
  globals: { theme: "dark" },
};

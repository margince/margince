// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { type Locale, translate } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { RetainedSignaturePanel } from "./retainedsignature";
import { SettingsSavePage } from "./settingssavepage";

const id = "00000000-0000-4000-8000-0000000000a1";
const retained = (): components["schemas"]["RetainedEmailSignature"] => ({
  id,
  owner_id: "00000000-0000-4000-8000-0000000000b1",
  owner_name: "Mira Voss",
  version: 7,
  signature: {
    body: "Mira Voss",
    html: "<p><b>Mira Voss</b><br>Head of Sales</p>",
    title: "Head of Sales",
    phone: "+49 711 555 0100",
    template_active: false,
  },
  preview_html: "<p><b>Mira Voss</b><br>Head of Sales</p>",
});

function review(locale: Locale = "en") {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      [`GET /privacy/retained-signatures/${id}`]: () =>
        jsonResponse(retained()),
    });
    return (
      <StoryProviders locale={locale}>
        <SettingsSavePage>
          <RetainedSignaturePanel id={id} />
        </SettingsSavePage>
      </StoryProviders>
    );
  };
}
const meta = {
  title: "Settings/Compliance/Privacy and retention/Retained signature",
  component: RetainedSignaturePanel,
  args: { id },
  parameters: { layout: "padded" },
} satisfies Meta<typeof RetainedSignaturePanel>;
export default meta;
type Story = StoryObj<typeof meta>;
export const Saved: Story = {
  render: review(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("textbox", {
        name: translate("en", "settings.signatureTitle"),
      }),
    ).toHaveValue("Head of Sales");
    await expect(
      canvas.getByRole("textbox", {
        name: translate("en", "settings.signatureHtmlLabel"),
      }),
    ).toHaveValue(retained().signature.html);
    await expect(canvas.getByTitle("Saved signature preview")).toBeVisible();
  },
};
export const Draft: Story = {
  render: review(),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const title = await canvas.findByRole("textbox", {
      name: translate("en", "settings.signatureTitle"),
    });
    await userEvent.clear(title);
    await userEvent.type(title, "Sales team");
    await expect(canvas.getByRole("button", { name: "Save" })).toBeEnabled();
  },
};
export const German: Story = { render: review("de") };
export const Vietnamese: Story = { render: review("vi") };

export const SavedPhone: Story = {
  ...Saved,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
export const DraftPhone: Story = {
  ...Draft,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const ConcurrentEdit: Story = {
  render: () => {
    let current = retained();
    installFetchStub({
      "GET /me": meRoute({}),
      [`GET /privacy/retained-signatures/${id}`]: () => jsonResponse(current),
      [`PUT /privacy/retained-signatures/${id}`]: () => {
        current = {
          ...current,
          version: 8,
          signature: {
            ...current.signature,
            body: "Reviewed by another controller",
          },
        };
        return jsonResponse(
          {
            status: 409,
            code: "version_skew",
            detail: "The retained signature changed.",
          },
          409,
        );
      },
    });
    return (
      <StoryProviders>
        <SettingsSavePage>
          <RetainedSignaturePanel id={id} />
        </SettingsSavePage>
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const body = await canvas.findByRole("textbox", {
      name: translate("en", "settings.signatureLabel"),
    });
    await userEvent.clear(body);
    await userEvent.type(body, "My retained draft");
    await userEvent.click(canvas.getByRole("button", { name: "Save" }));
    await expect(
      await canvas.findByRole("button", { name: "Keep my draft" }),
    ).toBeVisible();
    await expect(body).toHaveValue("My retained draft");
    await expect(canvas.getByRole("button", { name: "Save" })).toBeDisabled();
  },
};
export const ConcurrentEditPhone: Story = {
  ...ConcurrentEdit,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

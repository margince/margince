// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { PaymentAgeSettings } from "./paymentage";

const saved = fn<(body: unknown) => void>();
type Settings = Pick<
  components["schemas"]["InstallationSettings"],
  "payment_long_overdue_days" | "locks"
>;

function ageStory(writable: boolean, locked = false) {
  return () => {
    saved.mockClear();
    let settings: Settings = {
      payment_long_overdue_days: 365,
      locks: locked
        ? [
            {
              key: "installation.payment_long_overdue_days",
              origin: {
                source: "env",
                name: "MARGINCE_INSTALLATION_PAYMENT_LONG_OVERDUE_DAYS",
              },
            },
          ]
        : [],
    };
    installFetchStub({
      "GET /me": () =>
        jsonResponse(
          meFixture({
            allow: {
              installation_settings: writable ? ["read", "update"] : ["read"],
            },
          }),
        ),
      "GET /installation/settings": () => jsonResponse(settings),
      "PATCH /installation/settings": (body) => {
        saved(body);
        if (body !== null && typeof body === "object")
          settings = { ...settings, ...body };
        return jsonResponse(settings);
      },
    });
    return (
      <StoryProviders>
        <PaymentAgeSettings />
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/Data/Integrations/Payment age",
  component: PaymentAgeSettings,
} satisfies Meta<typeof PaymentAgeSettings>;
export default meta;
type Story = StoryObj<typeof meta>;

export const Editable: Story = {
  render: ageStory(true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const input = await canvas.findByLabelText(en["payments.ageLabel"]);
    const user = userEvent.setup();
    await user.clear(input);
    await user.type(input, "730");
    await user.keyboard("{Enter}");
    await expect(saved).toHaveBeenCalledWith({
      payment_long_overdue_days: 730,
    });
  },
};

export const ReadOnly: Story = {
  render: ageStory(false),
  play: async ({ canvasElement }) => {
    await expect(
      await within(canvasElement).findByLabelText(en["payments.ageLabel"]),
    ).toBeDisabled();
  },
};

export const SetByEnvironment: Story = {
  ...ReadOnly,
  render: ageStory(true, true),
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ForcedMfaScreen } from "./forcedmfa";

const SECRET = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP";
const RECOVERY_CODES = [
  "aB3-dEf_ghIJkl",
  "Zx9_yW8-vU7tsr",
  "Qp0-oN1_mL2kjH",
  "gF4_eD5-cB6aZy",
  "Xw7-vU8_tS9rQp",
  "oN0_mL1-kJ2iHg",
  "fE3-dC4_bA5zYx",
  "wV6_uT7-sR8qPo",
  "nM9-lK0_jI1hGf",
  "eD2_cB3-aZ4yXw",
];

const meta: Meta<typeof ForcedMfaScreen> = {
  title: "Signed out/Forced two-step setup",
  component: ForcedMfaScreen,
  parameters: { layout: "fullscreen" },
  decorators: [
    (Story) => {
      installFetchStub({
        "POST /me/mfa/totp": () =>
          jsonResponse({
            secret: SECRET,
            otpauth_uri: `otpauth://totp/Margince:dana%40example.test?secret=${SECRET}&issuer=Margince`,
          }),
        "POST /me/mfa/totp/confirm": () =>
          jsonResponse({ recovery_codes: RECOVERY_CODES }),
      });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
  args: { onEnrolled: () => {} },
};
export default meta;

type Story = StoryObj<typeof ForcedMfaScreen>;

const startSetup = async () => {
  await userEvent.click(await screen.findByRole("button", { name: "Set up" }));
  await screen.findByRole("img", { name: /qr code/i });
};

/** The boundary as the member meets it after the password: the reason and the way on. */
export const AtTheBoundary: Story = {};

/** The scan step, on the sign-in ground. */
export const ScanStep: Story = { play: startSetup };

/** The scan step on a phone. */
export const ScanStepPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: startSetup,
};

/** The recovery codes, dark, with Done as the only way on. */
export const RecoveryCodes: Story = {
  globals: { theme: "dark" },
  play: async () => {
    await startSetup();
    await userEvent.type(
      screen.getByRole("textbox", { name: /code from the app/i }),
      "123456{Enter}",
    );
    const codes = await screen.findByRole("textbox", {
      name: "Recovery codes",
    });
    await waitFor(() => expect(codes).toBeVisible());
  },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { Panel } from "../design-system/panel";
import { SettingList } from "../design-system/settingrow";
import type { MfaStatus } from "./mfa.queries";
import { MfaSettingRow } from "./mfacard";
import { MfaSetupModal } from "./mfasetup";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

const meta: Meta<typeof MfaSettingRow> = {
  title: "Settings/You/Account/Two-step sign-in",
  component: MfaSettingRow,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof MfaSettingRow>;

const SECRET = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP";
const ENROLMENT = {
  secret: SECRET,
  otpauth_uri: `otpauth://totp/Margince:dana%40example.test?secret=${SECRET}&issuer=Margince`,
};
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

function Row({ status }: Readonly<{ status: MfaStatus }>) {
  installFetchStub({
    "GET /me/mfa": () => jsonResponse(status),
    "POST /me/mfa/totp": () => jsonResponse(ENROLMENT),
    "DELETE /me/mfa": () =>
      jsonResponse({ title: "Refused", status: 401 }, 401),
  });
  return (
    <StoryProviders>
      <Panel>
        <SettingList bleed="settings">
          <MfaSettingRow />
        </SettingList>
      </Panel>
    </StoryProviders>
  );
}

function SetupStep() {
  installFetchStub({
    "POST /me/mfa/totp/confirm": () =>
      jsonResponse({ recovery_codes: RECOVERY_CODES }),
  });
  return (
    <StoryProviders>
      <MfaSetupModal
        open
        enrolment={ENROLMENT}
        starting={false}
        startError={null}
        onRetry={() => undefined}
        onClose={() => undefined}
      />
    </StoryProviders>
  );
}

const OFF: MfaStatus = {
  enrolled: false,
  confirmed: false,
  recovery_codes_left: 0,
};

/** Not set up: the row offers Set up. */
export const Off: Story = { render: () => <Row status={OFF} /> };

/** On, with the recovery codes that remain counted on the row. */
export const On: Story = {
  render: () => (
    <Row
      status={{ enrolled: true, confirmed: true, recovery_codes_left: 10 }}
    />
  ),
};

/** Two codes left: the row warns, and says new codes mean setting it up again. */
export const LowCodes: Story = {
  name: "Few recovery codes",
  render: () => (
    <Row status={{ enrolled: true, confirmed: true, recovery_codes_left: 2 }} />
  ),
};

/** A setup that was started and never confirmed is not a factor yet. */
export const Unfinished: Story = {
  render: () => (
    <Row
      status={{ enrolled: true, confirmed: false, recovery_codes_left: 0 }}
    />
  ),
};

/** The scan step: the QR code, the key for typing in by hand, and the code. */
export const SetUp: Story = {
  name: "Set-up step",
  render: () => <SetupStep />,
};

/** The scan step at phone width, where the dialog is a full-screen sheet. */
export const SetUpPhone: Story = {
  name: "Set-up step, phone",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <SetupStep />,
};

/** After a code is accepted: the recovery codes, shown this once. */
export const RecoveryCodes: Story = {
  name: "Recovery codes step",
  render: () => <SetupStep />,
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.type(
      await page.findByRole("textbox", { name: /code from the app/i }),
      "123456",
    );
    await userEvent.click(page.getByRole("button", { name: "Turn on" }));
    await expect(
      await page.findByRole("textbox", { name: "Recovery codes" }),
    ).toBeVisible();
  },
};

/** Removing asks for a code; a wrong one is refused under the field. */
export const Remove: Story = {
  render: () => (
    <Row status={{ enrolled: true, confirmed: true, recovery_codes_left: 8 }} />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Remove" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.type(
      await page.findByRole("textbox", { name: /^code/i }),
      "000000",
    );
    await userEvent.click(
      page.getByRole("button", { name: "Remove two-step sign-in" }),
    );
    await page.findByText(/code was not accepted/i);
  },
};

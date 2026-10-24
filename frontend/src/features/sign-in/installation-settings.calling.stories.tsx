// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useId, useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import { meFixture } from "../../testing/mefixture";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { CallingSettingsRows } from "./installation-settings.calling";

type InstallationSettings = components["schemas"]["InstallationSettings"];
type Patch = components["schemas"]["UpdateInstallationSettingsRequest"];

const ALL_ON: Pick<
  InstallationSettings,
  "call_whatsapp" | "call_telegram" | "call_zalo"
> = { call_whatsapp: true, call_telegram: true, call_zalo: true };

// A write that lands at once, or one the server refused.
function Calling({
  canManage = true,
  locks,
  refused = false,
}: Readonly<{
  canManage?: boolean;
  locks?: InstallationSettings["locks"];
  refused?: boolean;
}>) {
  const t = useT();
  const denialId = useId();
  const [settings, setSettings] = useState({ ...ALL_ON, locks });
  const update = {
    mutate: (patch: Patch) => {
      if (!refused) setSettings((was) => ({ ...was, ...patch }));
    },
    isPending: false,
    isError: refused,
    variables: undefined,
    error: refused ? new Error("refused") : null,
  };
  return (
    <Panel title={t("installationSettings.companyTitle")}>
      {!canManage && (
        <PanelBody>
          <p id={denialId}>{t("installationSettings.readOnly")}</p>
        </PanelBody>
      )}
      <CallingSettingsRows
        settings={{ ...FULL, ...settings }}
        canManage={canManage}
        denialId={denialId}
        update={update}
      />
    </Panel>
  );
}

const FULL: InstallationSettings = {
  name: "Example",
  base_language: "en",
  fiscal_year_start_month: 1,
  sign_in_providers: [],
  max_upload_bytes: 25_000_000,
  dead_work_banner_hours: 24,
  oauth_access_token_ttl_minutes: 43_200,
  operations: defaultOperations,
  agent_allowances: defaultAgentAllowances,
  forecast_forward_measure: "commit_evidence",
  timezone: meFixture().user.timezone,
  base_currency: "EUR",
  base_currency_locked: false,
  ...ALL_ON,
};

const meta: Meta<typeof CallingSettingsRows> = {
  title: "Settings/Company/Company profile/Calling",
  component: CallingSettingsRows,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  render: () => <Calling />,
};
export default meta;
type Story = StoryObj<typeof CallingSettingsRows>;

export const Editable: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const telegram = await canvas.findByRole("switch", {
      name: "Telegram chat",
    });
    await userEvent.click(telegram);
    await expect(telegram).toHaveAttribute("aria-checked", "false");
    await expect(
      canvas.getByRole("switch", { name: "WhatsApp chat" }),
    ).toHaveAttribute("aria-checked", "true");
  },
};

export const Dark: Story = { globals: { theme: "dark" } };

export const ReadOnly: Story = {
  render: () => <Calling canManage={false} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("switch", { name: "Zalo chat" }),
    ).toBeDisabled();
  },
};

// Set in the environment: that switch is refused and names the variable.
export const LockedByEnvironment: Story = {
  render: () => (
    <Calling
      locks={[
        {
          key: "installation.call_zalo",
          origin: { source: "env", name: "MARGINCE_INSTALLATION_CALL_ZALO" },
        },
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("switch", { name: "Zalo chat" }),
    ).toBeDisabled();
    await expect(
      canvas.getByRole("switch", { name: "WhatsApp chat" }),
    ).not.toBeDisabled();
  },
};

export const Refused: Story = {
  render: () => <Calling refused />,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("switch", { name: "WhatsApp chat" });
  },
};

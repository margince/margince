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
import { IndividualPerformanceRows } from "./installation-settings.individual";

type InstallationSettings = components["schemas"]["InstallationSettings"];
type Patch = components["schemas"]["UpdateInstallationSettingsRequest"];

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
  call_whatsapp: true,
  call_telegram: true,
  call_zalo: true,
  individual_performance: true,
  forecast_self_correcting: true,
};

// A write that lands at once, or one the server refused.
function Individual({
  canManage = true,
  on = true,
  locks,
  refused = false,
}: Readonly<{
  canManage?: boolean;
  on?: boolean;
  locks?: InstallationSettings["locks"];
  refused?: boolean;
}>) {
  const t = useT();
  const denialId = useId();
  const [settings, setSettings] = useState({
    ...FULL,
    individual_performance: on,
    locks,
  });
  const update = {
    mutate: (patch: Patch) => {
      if (!refused) setSettings((was) => ({ ...was, ...patch }));
    },
    isPending: false,
    isError: refused,
    error: refused ? new Error("refused") : null,
  };
  return (
    <Panel title={t("installationSettings.companyTitle")}>
      {!canManage && (
        <PanelBody>
          <p id={denialId}>{t("installationSettings.readOnly")}</p>
        </PanelBody>
      )}
      <IndividualPerformanceRows
        settings={settings}
        canManage={canManage}
        denialId={denialId}
        update={update}
      />
    </Panel>
  );
}

const meta: Meta<typeof IndividualPerformanceRows> = {
  title: "Settings/Company/Company profile/Individual performance",
  component: IndividualPerformanceRows,
  decorators: [
    (Story) => (
      <StoryProviders>
        <Story />
      </StoryProviders>
    ),
  ],
  render: () => <Individual />,
};
export default meta;
type Story = StoryObj<typeof IndividualPerformanceRows>;

const SWITCH = "Show individual performance";

export const SwitchOff: Story = {
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const control = await canvas.findByRole("switch", { name: SWITCH });
    await userEvent.click(control);
    await expect(control).toHaveAttribute("aria-checked", "false");
    await expect(
      canvas.getByText(/Nobody sees a colleague/),
    ).toBeInTheDocument();
  },
};

export const Off: Story = {
  render: () => <Individual on={false} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("switch", { name: SWITCH }),
    ).toHaveAttribute("aria-checked", "false");
  },
};

export const Dark: Story = {
  globals: { theme: "dark" },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("switch", { name: SWITCH });
    await expect(document.documentElement.dataset.theme).toBe("dark");
  },
};

export const ReadOnly: Story = {
  render: () => <Individual canManage={false} />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("switch", { name: SWITCH }),
    ).toBeDisabled();
  },
};

// Set in the environment: the switch is refused and names the variable.
export const LockedByEnvironment: Story = {
  render: () => (
    <Individual
      locks={[
        {
          key: "installation.individual_performance",
          origin: {
            source: "env",
            name: "MARGINCE_INSTALLATION_INDIVIDUAL_PERFORMANCE",
          },
        },
      ]}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      await canvas.findByRole("switch", { name: SWITCH }),
    ).toBeDisabled();
  },
};

export const Refused: Story = {
  render: () => <Individual refused />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const control = await canvas.findByRole("switch", { name: SWITCH });
    await userEvent.click(control);
    await expect(control).toHaveAttribute("aria-checked", "true");
    await expect(canvas.getByText("Settings not saved")).toBeInTheDocument();
  },
};

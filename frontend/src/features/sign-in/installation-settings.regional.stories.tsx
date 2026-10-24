// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { components } from "../../api/schema";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { RegionalSettingsFields } from "./installation-settings.regional";

function RegionalFields({
  locks,
}: Readonly<{ locks?: components["schemas"]["SettingLocks"] }>) {
  const [draft, setDraft] = useState<
    components["schemas"]["InstallationSettings"]
  >({
    name: "Example",
    base_language: "en",
    fiscal_year_start_month: 1,
    sign_in_providers: [],
    max_upload_bytes: 25000000,
    dead_work_banner_hours: 24,
    oauth_access_token_ttl_minutes: 43_200,
    operations: defaultOperations,
    agent_allowances: defaultAgentAllowances,
    forecast_forward_measure: "commit_evidence",
    timezone: "Europe/Berlin",
    base_currency: "EUR",
    base_currency_locked: false,
    call_whatsapp: true,
    call_telegram: true,
    call_zalo: true,
    date_format: "dmy",
    time_format: "24h",
  });
  return (
    <StoryProviders>
      <div className="form-stack">
        <RegionalSettingsFields
          locks={locks}
          draft={draft}
          canManage
          refused={new Map()}
          onChange={setDraft}
        />
      </div>
    </StoryProviders>
  );
}
const meta: Meta<typeof RegionalSettingsFields> = {
  title: "Settings/Company/Company profile/Regional formats",
  component: RegionalSettingsFields,
  render: () => <RegionalFields />,
};
export default meta;
type Story = StoryObj<typeof RegionalSettingsFields>;
export const Editable: Story = {};
export const Dark: Story = { globals: { theme: "dark" } };
// The date format set in the configuration file: the field is refused and
// names the file and key to change.
export const LockedByConfigFile: Story = {
  render: () => (
    <RegionalFields
      locks={[
        {
          key: "installation.date_format",
          origin: {
            source: "config_file",
            name: "installation.date_format",
            file: "/etc/margince/margince.yaml",
          },
        },
      ]}
    />
  ),
};

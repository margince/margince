// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import type { components } from "../api/schema";
import { RegionalSettingsFields } from "./installation-settings.regional";
import { StoryProviders } from "./story-utils";

function RegionalFields() {
  const [draft, setDraft] = useState<
    components["schemas"]["InstallationSettings"]
  >({
    name: "Example",
    base_language: "en",
    fiscal_year_start_month: 1,
    sign_in_providers: [],
    max_upload_bytes: 25000000,
    dead_work_banner_hours: 24,
    forecast_forward_measure: "commit_evidence",
    timezone: "Europe/Berlin",
    base_currency: "EUR",
    base_currency_locked: false,
    date_format: "dmy",
    time_format: "24h",
  });
  return (
    <StoryProviders>
      <div className="form-stack">
        <RegionalSettingsFields
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

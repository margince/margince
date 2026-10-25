// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { DateFormatsProvider } from "../../app/dateformats";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import type { InstallationSettings } from "./installation-settings";
import { InstallationProfileDialog } from "./installation-settings.profile";

const SETTINGS: InstallationSettings = {
  name: "Brandt Automotive GmbH",
  base_language: "en",
  fiscal_year_start_month: 1,
  sign_in_providers: [],
  max_upload_bytes: 25000000,
  dead_work_banner_hours: 24,
  oauth_access_token_ttl_minutes: 43_200,
  operations: defaultOperations,
  agent_allowances: defaultAgentAllowances,
  forecast_forward_measure: "commit_evidence",
  timezone: REPORTING_FIXTURE_ZONE,
  base_currency: "EUR",
  date_format: "dmy",
  time_format: "24h",
  base_currency_locked: true,
  call_whatsapp: true,
  call_telegram: true,
  call_zalo: true,
  base_currency_locked_reason:
    "3 rate(s) in the sheet convert into EUR and cannot be restated against another base; clear the rate sheet first",
  base_currency_lock: { reason: "rate_sheet", count: 3, base_currency: "EUR" },
};

function ProfileForm({ locale }: Readonly<{ locale?: "de" | "vi" }>) {
  const [draft, setDraft] = useState(SETTINGS);
  return (
    <StoryProviders locale={locale}>
      <DateFormatsProvider>
        <InstallationProfileDialog
          settings={SETTINGS}
          draft={draft}
          focus="base_currency"
          canManage
          dirty={false}
          pending={false}
          refused={new Map()}
          blanketError={null}
          describe={(control) => control["aria-describedby"]}
          onChange={setDraft}
          onClose={() => setDraft(SETTINGS)}
          onSubmit={() => setDraft(SETTINGS)}
        />
      </DateFormatsProvider>
    </StoryProviders>
  );
}

const meta: Meta<typeof InstallationProfileDialog> = {
  title: "Settings/Company/Company profile/Installation profile form",
  component: InstallationProfileDialog,
};
export default meta;
type Story = StoryObj<typeof InstallationProfileDialog>;

// The currency field's hint words the lock facts in the reader's language.
export const BaseCurrencyLocked: Story = { render: () => <ProfileForm /> };

export const BaseCurrencyLockedVietnamese: Story = {
  render: () => <ProfileForm locale="vi" />,
};

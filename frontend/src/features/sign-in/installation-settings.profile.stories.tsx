// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { DateFormatsProvider } from "../../app/dateformats";
import { faultSentence } from "../../lib/problem";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import { REPORTING_FIXTURE_ZONE } from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import type {
  InstallationPart,
  InstallationSettings,
} from "./installation-settings";
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
  forecast_forward_measure: "likely",
  timezone: REPORTING_FIXTURE_ZONE,
  base_currency: "EUR",
  date_format: "dmy",
  time_format: "24h",
  base_currency_locked: true,
  call_whatsapp: true,
  call_telegram: true,
  call_zalo: true,
  individual_performance: true,
  forecast_self_correcting: true,
  forecast_likely_threshold: 90,
  base_currency_locked_reason:
    "3 rate(s) in the sheet convert into EUR and cannot be restated against another base; remove every currency from the rate sheet first",
  base_currency_lock: { reason: "rate_sheet", count: 3, base_currency: "EUR" },
};

function ProfileForm({
  locale,
  part = "regional",
  start = SETTINGS,
  refused = new Map(),
  lockRefused = false,
}: Readonly<{
  locale?: "de" | "vi";
  part?: InstallationPart;
  start?: InstallationSettings;
  refused?: ReadonlyMap<string, string>;
  lockRefused?: boolean;
}>) {
  const [draft, setDraft] = useState(start);
  return (
    <StoryProviders locale={locale}>
      <DateFormatsProvider>
        <InstallationProfileDialog
          part={part}
          settings={SETTINGS}
          draft={draft}
          focus={part === "forecast" ? "forecast_likely_threshold" : "timezone"}
          canManage
          dirty={JSON.stringify(draft) !== JSON.stringify(SETTINGS)}
          pending={false}
          refused={refused}
          lockRefused={lockRefused}
          formRef={() => undefined}
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

// A save refused because a rate froze the currency after the dialog opened.
// The refusal words the lock, and the hint that would repeat it stays hidden.
export const BaseCurrencyRefusedGerman: Story = {
  render: () => (
    <ProfileForm
      locale="de"
      start={{ ...SETTINGS, base_currency: "CHF" }}
      refused={
        new Map([
          [
            "base_currency",
            faultSentence(
              {
                field: "installation.base_currency",
                code: "setting_frozen",
                message: "3 rate(s) in the sheet convert into EUR",
                params: {
                  reason: "rate_sheet",
                  count: "3",
                  base_currency: "EUR",
                },
              },
              "de",
            ),
          ],
        ])
      }
      lockRefused
    />
  ),
};

// A new fiscal start in the draft: the dialog counts the saved work it
// concerns and says what happens to it before anything is saved.
export const FiscalStartMoved: Story = {
  render: () => {
    installFetchStub({
      "GET /installation/settings/fiscal-year-impact": () =>
        jsonResponse({
          saved_questions: 14,
          reports_with_kept_periods: 2,
          reports_with_relative_periods: 1,
        }),
    });
    return <ProfileForm start={{ ...SETTINGS, fiscal_year_start_month: 4 }} />;
  },
};

// A cleared Likely threshold, refused on its field with the bounds in percent.
export const ThresholdRefused: Story = {
  render: () => (
    <ProfileForm
      part="forecast"
      start={{ ...SETTINGS, forecast_likely_threshold: 0 }}
      refused={
        new Map([
          [
            "forecast_likely_threshold",
            "Likely threshold must be from 1% to 100%.",
          ],
        ])
      }
    />
  ),
};

// A changed draft and Escape: the dialog asks before it throws the draft away.
// Cancel discards at once.
export const EscapeAsksBeforeDiscarding: Story = {
  render: () => <ProfileForm />,
  play: async () => {
    const user = userEvent.setup();
    const drawer = within(await screen.findByRole("dialog"));
    const zone = drawer.getByRole("textbox", { name: "Company time zone" });
    await user.clear(zone);
    await user.type(zone, "Europe/Vienna");
    // Dispatched by hand: Storybook wraps its key presses in act, and the
    // question opening under one suspends on the copy catalog and warns.
    zone.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    const question = await screen.findByRole("dialog", {
      name: "Discard unsaved changes?",
    });
    await waitFor(() => expect(question).toBeVisible());
  },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { DateFormatsProvider } from "../../app/dateformats";
import { defaultAgentAllowances } from "../../testing/agentallowances.fixtures";
import { defaultOperations } from "../../testing/operationsettings.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { InstallationSettingsCard } from "./installation-settings";

// Settings → Admin settings → General → Installation. Two surfaces in one card:
// three ROWS that read what the installation is set to, and the one dialog that
// edits all three together (the server takes one sparse PATCH, so there is one
// save). The stories worth looking at are therefore split between the two —
// what the rows say, and what the form under them looks like once it is open.

const SETTINGS = {
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
  timezone: "Europe/Berlin",
  base_currency: "EUR",
  date_format: "dmy",
  time_format: "24h",
  base_currency_locked: false,
  call_whatsapp: true,
  call_telegram: false,
  call_zalo: true,
  individual_performance: true,
};

const RATE_SHEET_LOCK = {
  ...SETTINGS,
  base_currency_locked: true,
  base_currency_locked_reason:
    "3 rate(s) in the sheet convert into EUR and cannot be restated against another base; clear the rate sheet first",
  base_currency_lock: { reason: "rate_sheet", count: 3, base_currency: "EUR" },
};
const RATES_FROZEN_LOCK = {
  ...SETTINGS,
  base_currency_locked: true,
  base_currency_locked_reason:
    "1 record(s) — closed deals, sent offers, mirrored invoices, contracts and commission entries — have already frozen a conversion rate against it, so changing the base would re-mean every roll-up built on them",
  base_currency_lock: { reason: "rates_frozen", count: 1 },
};

function story(
  settings: Record<string, unknown>,
  allow: Parameters<typeof meRoute>[0],
  locale?: "de" | "vi",
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      "GET /installation/settings": () => jsonResponse(settings),
    });
    return (
      <StoryProviders locale={locale}>
        <DateFormatsProvider>
          <InstallationSettingsCard />
        </DateFormatsProvider>
      </StoryProviders>
    );
  };
}

// Opens the dialog the way a reader does — from one row's own Edit verb, which
// is also what puts focus on that row's field.
async function openFrom(canvasElement: HTMLElement, fact: RegExp) {
  const canvas = within(canvasElement);
  await userEvent.click(await canvas.findByRole("button", { name: fact }));
}

const MANAGER = { installation_settings: ["read", "update"] } as const;
const READER = { installation_settings: ["read"] } as const;

const meta: Meta<typeof InstallationSettingsCard> = {
  title: "Settings/Company/Company profile/Installation",
  component: InstallationSettingsCard,
};
export default meta;
type Story = StoryObj<typeof InstallationSettingsCard>;

// The card at rest: three answers in one column, each with the verb that
// changes it. This is the reading the row language exists for — an operator
// auditing the installation travels one column instead of parsing a form.
export const Editable: Story = { render: story(SETTINGS, MANAGER) };

// The form the rows lead to, opened from the timezone row, so focus is in the
// timezone field. One heading for the currency rule, one Save.
export const ProfileDialog: Story = {
  render: story(SETTINGS, MANAGER),
  play: async ({ canvasElement }) => {
    await openFrom(canvasElement, /edit reporting timezone/i);
  },
};

export const BaseCurrencyLocked: Story = {
  render: story(RATE_SHEET_LOCK, MANAGER),
};

export const BaseCurrencyLockedByFrozenRates: Story = {
  render: story(RATES_FROZEN_LOCK, MANAGER),
};

export const BaseCurrencyLockedGerman: Story = {
  render: story(RATE_SHEET_LOCK, MANAGER, "de"),
};

export const BaseCurrencyLockedOlderServer: Story = {
  render: story({ ...RATE_SHEET_LOCK, base_currency_lock: undefined }, MANAGER),
};

// The lock reason is the longest description on the card. On a phone it has to
// wrap without pushing the value or the verb off the row.
export const BaseCurrencyLockedPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(RATE_SHEET_LOCK, MANAGER),
};

// A permission, not a lock. Every Edit verb is refused together and the reason
// is stated ONCE, with each refused button pointing at it — printing it beside
// three buttons would say one fact three times.
export const ReadOnly: Story = { render: story(SETTINGS, READER) };

// The locked dialog in dark, because that is the pairing dark gets wrong: a
// disabled TextInput says so with a fill and an ink a step off the enabled
// field's, and one step is exactly what a darker palette compresses. Three more
// things are on trial with it — the Field labels, the hints under them (which
// here carry a lock REASON, a sentence a reader has to be able to read), and the
// primary Save in the dialog's action row.
export const BaseCurrencyLockedDark: Story = {
  globals: { theme: "dark" },
  render: story(RATE_SHEET_LOCK, MANAGER),
  play: async ({ canvasElement }) => {
    await openFrom(canvasElement, /edit base currency/i);
  },
};

// The rows in dark. What is on trial is the hairline between them and the muted
// value column against the card ground: both are color-mix()es that follow the
// dark accent lift, and a value that goes as quiet as its own description stops
// reading as the answer.
export const EditableDark: Story = {
  globals: { theme: "dark" },
  render: story(SETTINGS, MANAGER),
};

// The card at 390px. The row gives up its two columns before it gives up the
// reading, so label, description, value and verb stack — and what narrow tests
// here is whether the VALUE still reads as this row's answer once it is no
// longer aligned in a column of its own, with a three-line hint above it.
//
// Storybook applies the viewport from the MANAGER, by resizing the preview
// iframe — so the fe-uat capture, which loads a bare iframe.html, renders this at
// the harness's own width and its PNG is NOT a picture of a phone. Review it in
// Storybook, or by narrowing the browser.
export const EditablePhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(SETTINGS, MANAGER),
};

export const RegionalFormats: Story = {
  render: story(SETTINGS, MANAGER),
  play: async ({ canvasElement }) => {
    await openFrom(canvasElement, /edit date format/i);
  },
};

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../story-utils";
import { BasisAct } from "./basis-act";
import { initialConversationState } from "./conversation-machine";
import type { ConversationState } from "./conversation-types";

// The reporting basis, asked right after the company is confirmed: base
// currency and reporting timezone, prefilled from the installation, with the
// currency shown locked once a deal has frozen it.

const asking: ConversationState = {
  ...initialConversationState,
  act: "basis",
  phase: "bs.ask",
};

const settings = {
  name: "Brandt Automotive",
  timezone: "Europe/Berlin",
  base_currency: "EUR",
  base_language: "en",
  fiscal_year_start_month: 1,
  sign_in_providers: [],
  base_currency_locked: false,
  max_upload_bytes: 26214400,
};

function act(locked: boolean, locale?: "de") {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ installation_settings: ["update"] }),
      "GET /installation/settings": () =>
        jsonResponse({
          ...settings,
          base_currency_locked: locked,
          ...(locked
            ? { base_currency_locked_reason: "3 deals have frozen EUR" }
            : {}),
        }),
    });
    return (
      <StoryProviders locale={locale}>
        <BasisAct state={asking} dispatch={() => {}} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof BasisAct> = {
  title: "Onboarding/Conversation/Basis act",
  component: BasisAct,
};
export default meta;
type Story = StoryObj<typeof BasisAct>;

/** Both fields open, prefilled. */
export const Open: Story = { render: act(false) };

/** The currency frozen by a deal: the field says it cannot change. */
export const CurrencyLocked: Story = { render: act(true) };

/** The German act. */
export const OpenGerman: Story = { render: act(false, "de") };

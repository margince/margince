// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { BasisAct } from "./basis-act";
import { initialConversationState } from "./conversation-machine";
import type { ConversationState } from "./conversation-types";

// The basis step: base currency and reporting timezone, prefilled from the
// installation. Stories cover a locked currency and a refused save.

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
            ? {
                base_currency_locked_reason: "3 record(s) have frozen EUR",
                base_currency_lock: { reason: "rates_frozen", count: 3 },
              }
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

const rateSheet = { reason: "rate_sheet", count: 2, base_currency: "EUR" };

// The step loaded unlocked; a rate was added before Continue, so the save is
// refused and the read after it reports the lock.
function refusedAct() {
  let refused = false;
  installFetchStub({
    "GET /me": meRoute({ installation_settings: ["update"] }),
    "GET /installation/settings": () =>
      jsonResponse(
        refused
          ? {
              ...settings,
              base_currency_locked: true,
              base_currency_lock: rateSheet,
            }
          : settings,
      ),
    "PATCH /installation/settings": () => {
      refused = true;
      return jsonResponse(
        {
          code: "validation_error",
          detail: "The installation could not be saved.",
          details: {
            errors: [
              {
                field: "installation.base_currency",
                code: "setting_frozen",
                message: "2 rate(s) in the sheet convert into EUR",
                params: {
                  reason: "rate_sheet",
                  count: "2",
                  base_currency: "EUR",
                },
              },
              {
                field: "installation.timezone",
                code: "setting_invalid",
                message: '"Mars/Olympus" is not an IANA zone name',
              },
            ],
          },
        },
        422,
      );
    },
  });
  return (
    <StoryProviders locale="de">
      <BasisAct state={asking} dispatch={() => {}} />
    </StoryProviders>
  );
}

const meta: Meta<typeof BasisAct> = {
  title: "Onboarding/Conversation/Basis act",
  component: BasisAct,
};
export default meta;
type Story = StoryObj<typeof BasisAct>;

/** Both reporting fields open and prefilled. */
export const Open: Story = { render: act(false) };

/** The currency frozen by a deal: the field says it cannot change. */
export const CurrencyLocked: Story = { render: act(true) };

/** The German act. */
export const OpenGerman: Story = { render: act(false, "de") };

/** A German save refused on both fields: the currency's lock and the time zone. */
export const RefusedGerman: Story = {
  render: refusedAct,
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    const currency = await canvas.findByLabelText("Basiswährung");
    await user.clear(currency);
    await user.type(currency, "CHF");
    const zone = canvas.getByLabelText("Zeitzone des Unternehmens");
    await user.clear(zone);
    await user.type(zone, "Mars/Olympus");
    await user.click(canvas.getByRole("button", { name: "Weiter" }));
    await waitFor(() => expect(currency).toBeDisabled());
    await expect(zone).toHaveAttribute("aria-invalid", "true");
  },
};

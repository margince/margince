// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import { useT } from "../i18n";
import { RefreshModelPrices } from "./rate-catalogue-refresh";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// RE-PRICING THE MODELS THIS INSTALLATION CALLS, on its own.
//
// The control answers in the same breath, so the frames are about the REPORT:
// one line per provider, each with the outcome the server reached. It sits in a
// Panel's action band on the model sheet, so that is the frame here; the report
// drops under the button rather than beside it, and the band is what shows
// whether it does.

const REFRESH = "POST /ai-model-rates/refresh";

const REPORT = {
  providers: [
    {
      provider: "openai_compatible",
      outcome: "updated",
      updated: 3,
      unchanged: 1,
      models: ["mistralai/mistral-small-2603", "google/gemma-4-31b-it"],
    },
    {
      provider: "jev_compatible",
      outcome: "unchanged",
      updated: 0,
      unchanged: 1,
      models: [],
    },
    {
      provider: "gemini",
      outcome: "not_available",
      updated: 0,
      unchanged: 0,
      models: [],
    },
    {
      provider: "anthropic",
      outcome: "not_available",
      updated: 0,
      unchanged: 0,
      models: [],
    },
    {
      provider: "ollama",
      outcome: "not_available",
      updated: 0,
      unchanged: 0,
      models: [],
    },
  ],
};

const UNREACHABLE = {
  providers: [
    {
      provider: "openai_compatible",
      outcome: "unreachable",
      updated: 0,
      unchanged: 0,
      models: [],
    },
    {
      provider: "gemini",
      outcome: "not_available",
      updated: 0,
      unchanged: 0,
      models: [],
    },
  ],
};

function band(routes: Parameters<typeof installFetchStub>[0]) {
  return () => {
    installFetchStub(routes);
    return (
      <StoryProviders>
        <Band />
      </StoryProviders>
    );
  };
}

function Band() {
  const t = useT();
  return (
    <Panel
      title={t("settings.rates.modelTitle")}
      actions={<RefreshModelPrices />}
    >
      <PanelBody>
        <PanelIntro>{t("settings.rates.modelIntro")}</PanelIntro>
      </PanelBody>
    </Panel>
  );
}

const meta: Meta<typeof RefreshModelPrices> = {
  title: "Settings/AI/Models and routing/Refresh model prices",
  component: RefreshModelPrices,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof RefreshModelPrices>;

const press: Story["play"] = async ({ canvasElement }) => {
  const canvas = within(canvasElement);
  await userEvent.click(
    await canvas.findByRole("button", { name: "Refresh model prices" }),
  );
};

/** At rest: one ghost verb in the band, and nothing claimed beside it. */
export const AtRest: Story = {
  render: band({ [REFRESH]: () => jsonResponse(REPORT) }),
};

/**
 * The report after a run: the broker's models written or found current, and
 * every vendor that publishes no price list saying so instead of staying silent.
 */
export const Report: Story = {
  render: band({ [REFRESH]: () => jsonResponse(REPORT) }),
  play: press,
};

/** The catalogue could not be read: the broker's line says so, nothing was written. */
export const CatalogueUnreachable: Story = {
  render: band({ [REFRESH]: () => jsonResponse(UNREACHABLE) }),
  play: press,
};

/** Refused, in the server's own words; the retry is the button beside it. */
export const Refused: Story = {
  render: band({
    [REFRESH]: () =>
      jsonResponse(
        {
          type: "https://errors.gradion.com/forbidden",
          title: "Forbidden",
          status: 403,
          code: "permission_denied",
          detail: "Refreshing prices needs the rates grant.",
        },
        403,
      ),
  }),
  play: press,
};

/** The write in flight: `pending`, never `disabled`. */
export const RefreshInFlight: Story = {
  render: band({ [REFRESH]: () => new Promise<Response>(() => {}) }),
  play: press,
};

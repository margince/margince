// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { meFixture } from "../app/mefixture";
import {
  ProviderRefreshLine,
  RefreshModelPricesButton,
  RefreshSummary,
  useRefreshModelPrices,
} from "./rate-catalogue-refresh";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// Re-pricing the models this installation calls from the broker's catalogue:
// the button, the one-line summary on the list, and each vendor's own answer
// inside its sheet.

const REPORT = {
  providers: [
    {
      provider: "openai_compatible",
      outcome: "updated",
      updated: 2,
      unchanged: 1,
      added: 1,
      kept: 1,
      models: ["a/b"],
      unlisted: ["e/typo"],
    },
    {
      provider: "gemini",
      outcome: "not_available",
      updated: 0,
      unchanged: 0,
      added: 0,
      kept: 0,
      models: [],
      unlisted: [],
    },
    {
      provider: "jev_compatible",
      outcome: "unreachable",
      updated: 0,
      unchanged: 0,
      added: 0,
      kept: 0,
      models: [],
      unlisted: [],
    },
  ],
};

function Panels() {
  const refresh = useRefreshModelPrices();
  return (
    <>
      <RefreshModelPricesButton refresh={refresh} />
      <RefreshSummary refresh={refresh} />
      <ProviderRefreshLine
        line={refresh.data?.providers.find(
          (p) => p.provider === "openai_compatible",
        )}
      />
      <ProviderRefreshLine
        line={refresh.data?.providers.find((p) => p.provider === "gemini")}
      />
    </>
  );
}

function Demo() {
  return (
    <StoryProviders>
      <Panels />
    </StoryProviders>
  );
}

const meta: Meta<typeof Demo> = {
  title: "Settings/AI/AI models/Refresh model prices",
  component: Demo,
};
export default meta;
type Story = StoryObj<typeof Demo>;

function stub(refuse = false) {
  installFetchStub({
    "GET /me": () =>
      jsonResponse(
        meFixture({ allow: { ai_model_rate: ["read", "create", "update"] } }),
      ),
    "POST /ai-model-rates/refresh": () =>
      refuse
        ? jsonResponse(
            {
              type: "https://errors.gradion.com/forbidden",
              title: "Forbidden",
              status: 403,
              code: "permission_denied",
              detail: "Refreshing prices needs the rates grant.",
            },
            403,
          )
        : jsonResponse(REPORT),
  });
}

const press: Story["play"] = async ({ canvasElement }) => {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: "Refresh model prices",
    }),
  );
};

export const AtRest: Story = {
  render: () => {
    stub();
    return <Demo />;
  },
};
export const Report: Story = {
  render: () => {
    stub();
    return <Demo />;
  },
  play: press,
};
export const Refused: Story = {
  render: () => {
    stub(true);
    return <Demo />;
  },
  play: press,
};

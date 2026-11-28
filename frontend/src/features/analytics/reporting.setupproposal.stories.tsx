// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent } from "storybook/test";
import { en as englishCopy } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import {
  type SetupProposal,
  SetupProposalStep,
} from "./reporting.setupproposal";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

// The ask under a report's description, and the proposal it returns.

const meta: Meta = { title: "Records/Reports/Analytics/Setup proposal" };
export default meta;
type Story = StoryObj;

const DESCRIPTION = "Sales in Germany this quarter, over time";

const proposal: SetupProposal = {
  site: "custom_report_setup",
  generated_by: "model",
  name: "Germany this quarter",
  selection: {
    country: "DE",
    period: "this_quarter",
    metrics: ["bookings_won"],
    blocks: ["bookings_trend"],
  },
  sources: [
    {
      field: "name",
      value: "Germany this quarter",
      phrase: "Germany this quarter",
    },
    { field: "country", value: "DE", phrase: "in Germany" },
    { field: "period", value: "this_quarter", phrase: "this quarter" },
    {
      field: "metric",
      value: "bookings_won",
      label: "Sales won",
      phrase: "sales",
    },
    { field: "chart", value: "bookings_trend", phrase: "over time" },
  ],
  clarifications: ["Which team is this for?"],
  unsupported: [
    {
      phrase: "discount per rep",
      code: "not_expressible",
      reason: "No metric holds a discount.",
    },
  ],
};

function Step() {
  return (
    <StoryProviders>
      <SetupProposalStep
        site="custom_report_setup"
        description={DESCRIPTION}
        onApply={() => {}}
      />
    </StoryProviders>
  );
}

async function ask() {
  await userEvent.click(
    await screen.findByRole("button", {
      name: englishCopy["reporting.setup.suggestReport"],
    }),
  );
}

/** Before the ask: one quiet button. */
export const Ready: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return <Step />;
  },
};

/** The proposal, naming the phrase behind each field it filled. */
export const Proposed: Story = {
  play: ask,
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "POST /analytics/setup-proposal": () => jsonResponse(proposal),
    });
    return <Step />;
  },
};

/** With no AI model set up, the step says so in place of the button. */
export const NoModel: Story = {
  play: ask,
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "POST /analytics/setup-proposal": () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Conflict",
            status: 409,
            code: "ai_not_configured",
            detail: "no model",
          },
          409,
        ),
    });
    return <Step />;
  },
};

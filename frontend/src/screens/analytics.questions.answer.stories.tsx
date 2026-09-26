// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { Panel, PanelBody } from "../design-system/panel";
import { AnswerTable, QuestionFailure } from "./analytics.questions.answer";
import {
  ANSWER,
  EMPTY_ANSWER,
  EXPLANATION,
  QUERY,
  refusal,
  STAGES,
  USERS,
  WITHHELD_ANSWER,
} from "./analytics.questions.testkit";
import type {
  AnalyticsAnswer,
  AnalyticsQuery,
} from "./analytics.questions.vocab";
import { ProblemError } from "./common";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// What a question comes back as: the table, the withheld and capped notes,
// the drawer each row opens, and a refusal read as guidance.
const meta: Meta = { title: "Records/Reports/Questions/Answer" };
export default meta;

type Story = StoryObj;

function answerStory(query: AnalyticsQuery, answer: AnalyticsAnswer) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /stages": () => jsonResponse(STAGES),
      "GET /users": () => jsonResponse(USERS),
      "POST /analytics/explain": () => jsonResponse(EXPLANATION),
    });
    return (
      <StoryProviders>
        <Panel title="Answer">
          <PanelBody>
            <AnswerTable
              query={query}
              answer={answer}
              baseCurrency="EUR"
              source={{ kind: "query", query }}
            />
          </PanelBody>
        </Panel>
      </StoryProviders>
    );
  };
}

export const Ready: Story = { render: answerStory(QUERY, ANSWER) };

export const ReadyDark: Story = { ...Ready, globals: { theme: "dark" } };

// One group fell under the floor: it stays a row that says so, and a note
// says some groups were kept back without saying how many.
export const WithheldRows: Story = {
  render: answerStory(QUERY, WITHHELD_ANSWER),
};

export const WithheldRowsDark: Story = {
  ...WithheldRows,
  globals: { theme: "dark" },
};

export const Empty: Story = { render: answerStory(QUERY, EMPTY_ANSWER) };

// Native amounts summed with no currency split: no figure, and a note that
// says what would give one.
export const MixedCurrencies: Story = {
  render: answerStory(
    { ...QUERY, group_by: ["stage_id"] },
    {
      ...ANSWER,
      columns: ["stage_id", "count", "sum_amount_minor"],
      rows: ANSWER.rows.map(({ currency: _dropped, ...row }) => row),
    },
  ),
};

// Many columns and long values: the table scrolls inside its own box.
const wideQuery: AnalyticsQuery = {
  entity: "deals-by-stage",
  group_by: ["stage_id", "currency", "status", "win_probability"],
  measures: [
    { fn: "count" },
    { fn: "sum", field: "amount_minor" },
    { fn: "avg", field: "amount_minor" },
    { fn: "median", field: "weighted_amount_minor" },
    { fn: "p75", field: "weighted_amount_minor" },
    { fn: "count_distinct", field: "partner_company_id" },
  ],
  limit: 100,
};
const wideRows = Array.from({ length: 100 }, (_, index) => ({
  stage_id: index % 2 ? "s-qual" : "s-prop",
  currency: index % 3 ? "EUR" : "VND",
  status: index % 2 ? "open" : "won",
  win_probability: 20 + (index % 5) * 10,
  count: 3 + index,
  sum_amount_minor: 1_250_000 * (index + 1),
  avg_amount_minor: 250_000 + index,
  median_weighted_amount_minor: index % 4 ? 90_000 * index : null,
  p75_weighted_amount_minor: null,
  count_distinct_partner_company_id: index % 7,
  _withheld: false,
}));
export const LongContent: Story = {
  render: answerStory(wideQuery, {
    ...ANSWER,
    columns: [
      "stage_id",
      "currency",
      "status",
      "win_probability",
      "count",
      "sum_amount_minor",
      "avg_amount_minor",
      "median_weighted_amount_minor",
      "p75_weighted_amount_minor",
      "count_distinct_partner_company_id",
    ],
    rows: wideRows,
  }),
};

export const LongContentPhone: Story = {
  ...LongContent,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// A row opened to its records, the drawer the report cards already use.
export const DrillDown: Story = {
  render: answerStory(QUERY, ANSWER),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Explain Qualified, EUR",
      }),
    );
    await screen.findByRole("dialog");
    await screen.findByText("€42,000.00");
  },
};

export const DrillDownPhone: Story = {
  ...DrillDown,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

function failureStory(problem: unknown) {
  return () => (
    <StoryProviders>
      <QuestionFailure error={new ProblemError(problem)} />
    </StoryProviders>
  );
}

export const RefusedInvalid: Story = {
  render: failureStory(
    refusal(
      "invalid",
      "sum needs a measure, and stage_id is a dimension",
      "sum one of: amount_minor, weighted_amount_minor",
    ),
  ),
};

export const RefusedUnsupported: Story = {
  render: failureStory(
    refusal(
      "unsupported",
      "grouping by more than four fields is not supported",
      "group by at most four fields",
    ),
  ),
};

export const RefusedPrivacy: Story = {
  render: failureStory(
    refusal(
      "privacy",
      "every group would describe fewer than five records",
      "group by stage_id alone, which clears the floor",
    ),
  ),
};

export const RefusedPrivacyDark: Story = {
  ...RefusedPrivacy,
  globals: { theme: "dark" },
};

export const PermissionDenied: Story = {
  render: failureStory({
    title: "Forbidden",
    status: 403,
    code: "permission_denied",
    detail: "scope outside your lens",
  }),
};

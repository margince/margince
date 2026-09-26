// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { QuestionBuilder } from "./analytics.questions.builder";
import { newDraft, type QuestionDraft } from "./analytics.questions.draft";
import { ENTITIES, STAGES, USERS } from "./analytics.questions.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The question as it is being composed. Every choice comes from the seat's
// schema, so each story is one schema and one draft.
const meta: Meta = { title: "Records/Reports/Questions/Builder" };
export default meta;

type Story = StoryObj;

function Editing({ start }: Readonly<{ start: QuestionDraft }>) {
  const [draft, setDraft] = useState(start);
  return (
    <QuestionBuilder
      entities={ENTITIES}
      draft={draft}
      baseCurrency="EUR"
      onChange={setDraft}
      onAsk={() => {}}
      onSave={() => {}}
      asking={false}
      saving={false}
    />
  );
}

function builderStory(start: QuestionDraft) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /stages": () => jsonResponse(STAGES),
      "GET /users": () => jsonResponse(USERS),
    });
    return (
      <StoryProviders>
        <Editing start={start} />
      </StoryProviders>
    );
  };
}

// Nothing chosen yet: both verbs wait, and one sentence says on what.
export const NothingChosen: Story = { render: builderStory(newDraft("")) };

// A composed question: a grouping, a count beside a sum, and one filter of
// each value shape the deal report offers.
const composed: QuestionDraft = {
  entity: "deals-by-stage",
  groupBy: ["stage_id", "currency"],
  measures: [
    { id: 1, fn: "count", field: "" },
    { id: 2, fn: "sum", field: "amount_minor" },
  ],
  filters: [
    { id: 3, field: "currency", op: "eq", value: "EUR" },
    { id: 4, field: "win_probability", op: "gte", value: 50 },
    { id: 5, field: "stage_id", op: "ne", value: "s-prop" },
    { id: 6, field: "partner_company_id", op: "is_not_null", value: "" },
  ],
};

export const Composed: Story = { render: builderStory(composed) };

export const ComposedDark: Story = {
  ...Composed,
  globals: { theme: "dark" },
};

export const ComposedPhone: Story = {
  ...Composed,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// A measure missing its field refuses the question until it has one.
export const MissingField: Story = {
  render: builderStory({
    ...composed,
    measures: [{ id: 1, fn: "avg", field: "" }],
    filters: [],
  }),
};

// A yes-or-no dimension takes a two-way choice, and an id a named pick.
export const YesOrNoAndOwners: Story = {
  render: builderStory({
    entity: "meeting-conversion",
    groupBy: ["host_user_id"],
    measures: [{ id: 1, fn: "count", field: "" }],
    filters: [
      { id: 2, field: "became_opportunity", op: "eq", value: true },
      { id: 3, field: "host_user_id", op: "eq", value: "u-2" },
    ],
  }),
};

// An amount is typed in the currency's own units; a deal's own amount can be
// compared only once one currency is pinned, and the verbs say so until it is.
export const AmountNeedsCurrency: Story = {
  render: builderStory({
    ...composed,
    filters: [{ id: 3, field: "amount_minor", op: "gte", value: 50000 }],
  }),
};

export const AmountInPinnedCurrency: Story = {
  render: builderStory({
    ...composed,
    filters: [
      { id: 3, field: "currency", op: "eq", value: "EUR" },
      { id: 4, field: "amount_minor", op: "gte", value: 50000 },
    ],
  }),
};

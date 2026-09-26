// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, userEvent, within } from "storybook/test";
import { pickOption } from "../design-system/select-testing";
import { QuestionsView } from "./analytics.questions";
import {
  ANSWER,
  CONTEXT,
  EXPLANATION,
  QUERY,
  refusal,
  SCHEMA,
  STAGES,
  USERS,
  WITHHELD_ANSWER,
} from "./analytics.questions.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The Questions section of Analytics: the schema read, the builder, the answer
// it asks for, and a saved question opened at its own address.
const meta: Meta = { title: "Records/Reports/Questions" };
export default meta;

type Story = StoryObj;

const base: RouteMap = {
  "GET /me": meRoute({}),
  "GET /analytics/schema": () => jsonResponse(SCHEMA),
  "GET /stages": () => jsonResponse(STAGES),
  "GET /users": () => jsonResponse(USERS),
  "POST /analytics/query": () => jsonResponse(ANSWER),
  "POST /analytics/explain": () => jsonResponse(EXPLANATION),
  "POST /analytics/runs/run-1/cells/explain": () => jsonResponse(EXPLANATION),
};

function viewStory(routes: RouteMap, hash = "#/analytics/questions") {
  return () => {
    globalThis.location.hash = hash;
    installFetchStub({ ...base, ...routes });
    return (
      <StoryProviders>
        <QuestionsView
          context={CONTEXT}
          selection={{ scope: CONTEXT.default_scope }}
          onSelectScope={() => {}}
        />
      </StoryProviders>
    );
  };
}

// Compose the fixture question and ask it, the way a reader does: the deal
// report grouped by stage and currency, counted and summed.
async function askFixture({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) {
  const user = userEvent.setup();
  const canvas = within(canvasElement);
  await pickOption(
    user,
    await canvas.findByRole("combobox", { name: "Report" }),
    "All deals by stage",
  );
  await user.click(canvas.getByRole("combobox", { name: "Group by" }));
  await user.click(await screen.findByRole("option", { name: "Stage" }));
  await user.click(screen.getByRole("option", { name: "Currency" }));
  await user.keyboard("{Escape}");
  await user.click(canvas.getByRole("button", { name: "Add measure" }));
  const second = canvas.getByRole("listitem", { name: "Measure 2" });
  await pickOption(
    user,
    within(second).getByRole("combobox", { name: "Calculation" }),
    "Sum",
  );
  await pickOption(
    user,
    within(second).getByRole("combobox", { name: "Field" }),
    "Amount",
  );
  await user.click(canvas.getByRole("button", { name: "Ask" }));
}

export const Loading: Story = {
  render: viewStory({
    "GET /analytics/schema": () => new Promise<Response>(() => {}),
  }),
};

export const SchemaFailed: Story = {
  render: viewStory({
    "GET /analytics/schema": () =>
      jsonResponse({ title: "Server error", status: 500 }, 500),
  }),
};

// A seat whose grants leave no population to ask about: withheld, not empty.
export const NoEntities: Story = {
  render: viewStory({
    "GET /analytics/schema": () =>
      jsonResponse({ version: "v0", entities: null }),
  }),
};

export const NoEntitiesDark: Story = {
  ...NoEntities,
  globals: { theme: "dark" },
};

export const Ready: Story = { render: viewStory({}) };

export const Answered: Story = {
  render: viewStory({}),
  play: async (context) => {
    await askFixture(context);
    await screen.findByText("€862,000.00");
  },
};

export const AnsweredDark: Story = { ...Answered, globals: { theme: "dark" } };

export const AnsweredPhone: Story = {
  ...Answered,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const AnsweredWithheld: Story = {
  render: viewStory({
    "POST /analytics/query": () => jsonResponse(WITHHELD_ANSWER),
  }),
  play: async (context) => {
    await askFixture(context);
    await screen.findByText("Some groups are withheld");
  },
};

export const Refused: Story = {
  render: viewStory({
    "POST /analytics/query": () =>
      jsonResponse(
        refusal(
          "privacy",
          "every group would describe fewer than five records",
          "group by stage_id alone, which clears the floor",
        ),
        400,
      ),
  }),
  play: async (context) => {
    await askFixture(context);
    await screen.findByText(/which clears the floor/);
  },
};

// A saved question, answered again for this reader, with the question read
// back as facts and the line that says whose access answered it.
const savedRun = {
  id: "run-1",
  query: QUERY,
  answer: ANSWER,
  asked_by: "u-1",
  stored_floor: 5,
};

export const SavedRun: Story = {
  render: viewStory(
    { "GET /analytics/runs/run-1": () => jsonResponse(savedRun) },
    "#/analytics/questions/run-1",
  ),
};

export const SavedRunDark: Story = {
  ...SavedRun,
  globals: { theme: "dark" },
};

export const SavedRunPhone: Story = {
  ...SavedRun,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const SavedRunDenied: Story = {
  render: viewStory(
    {
      "GET /analytics/runs/run-1": () =>
        jsonResponse(
          { title: "Forbidden", status: 403, code: "permission_denied" },
          403,
        ),
    },
    "#/analytics/questions/run-1",
  ),
};

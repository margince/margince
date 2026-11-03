// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import {
  CustomReportDrawer,
  heldCustomReport,
} from "./reporting.custombuilder";
import type { SetupProposal } from "./reporting.setupproposal";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
// The story's signed-in reader, as /me names them.
const VIEWER = "00000000-0000-4000-8000-000000000001";

afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  window.location.hash = "";
});

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

it("asks for a proposal from the description and applies it to the builder", async () => {
  const user = userEvent.setup({ delay: null });
  const asked: unknown[] = [];
  installFetchStub({
    ...reportingStoryRoutes(),
    "POST /analytics/setup-proposal": (body) => {
      asked.push(body);
      return jsonResponse(proposal);
    },
  });
  render(
    <StoryProviders>
      <CustomReportDrawer
        initial={{
          ...reportingStoryEvaluation.selection,
          period: "this_month",
          country: undefined,
        }}
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  const suggest = await screen.findByRole("button", {
    name: "Suggest filters and charts",
  });
  expect(suggest).toBeDisabled();
  await user.type(
    screen.getByRole("textbox", { name: "What would you like to understand?" }),
    "Sales in Germany this quarter over time",
  );
  await user.click(suggest);
  expect(
    await screen.findByText("Proposed from your words"),
  ).toBeInTheDocument();
  expect(asked).toEqual([
    {
      site: "custom_report_setup",
      description: "Sales in Germany this quarter over time",
    },
  ]);
  expect(
    screen.getByText("Country: Germany, from “in Germany”"),
  ).toBeInTheDocument();
  expect(screen.getByText("Which team is this for?")).toBeInTheDocument();
  expect(
    screen.getByText("“discount per rep”: No metric holds a discount."),
  ).toBeInTheDocument();

  await user.click(screen.getByRole("button", { name: "Use proposal" }));
  await waitFor(() =>
    expect(screen.getByRole("textbox", { name: "Report name" })).toHaveValue(
      "Germany this quarter",
    ),
  );
  expect(
    screen.queryByText("Proposed from your words"),
  ).not.toBeInTheDocument();
  // The preview carries what the proposal filled in.
  const preview = screen.getByRole("button", { name: "Preview report" });
  await waitFor(() => expect(preview).toBeEnabled());
  await user.click(preview);
  const held = heldCustomReport(VIEWER);
  expect(held?.selection.country).toBe("DE");
  expect(held?.selection.period).toBe("this_quarter");
  expect(held?.selection.metrics).toEqual(["bookings_won"]);
  expect(held?.selection.blocks).toEqual(["bookings_trend"]);
});

it("tells the reader when no model is bound and leaves the builder usable", async () => {
  const user = userEvent.setup({ delay: null });
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
  render(
    <StoryProviders>
      <CustomReportDrawer
        initial={reportingStoryEvaluation.selection}
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  await user.type(
    await screen.findByRole("textbox", {
      name: "What would you like to understand?",
    }),
    "Sales in Germany",
  );
  await user.click(
    screen.getByRole("button", { name: "Suggest filters and charts" }),
  );
  expect(
    await screen.findByText(/needs an AI model configured/),
  ).toBeInTheDocument();
  expect(screen.getByRole("textbox", { name: "Report name" })).toBeEnabled();
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import {
  CustomReportDrawer,
  heldCustomReport,
} from "./reporting.custombuilder";
import { hasAssessment } from "./reporting.layout";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;

afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
  window.location.hash = "";
});

it("previews only a named report, with the assessment and the question it was given", async () => {
  const user = userEvent.setup({ delay: null });
  installFetchStub(reportingStoryRoutes());
  render(
    <StoryProviders>
      <CustomReportDrawer
        initial={reportingStoryEvaluation.selection}
        onClose={() => {}}
      />
    </StoryProviders>,
  );
  const preview = await screen.findByRole("button", {
    name: "Preview report",
  });
  expect(preview).toBeDisabled();

  await user.click(screen.getByRole("button", { name: "Use example" }));
  expect(
    screen.getByRole("textbox", { name: "What would you like to understand?" }),
  ).toHaveValue(
    "Margince sales in Germany this quarter: bookings, lead conversion and open pipeline by country.",
  );
  await user.type(
    screen.getByRole("textbox", { name: "Report name" }),
    "Germany sales review",
  );
  await user.click(
    screen.getByRole("checkbox", { name: "Include the assessment" }),
  );
  await user.click(screen.getByRole("button", { name: "Add this question" }));
  expect(preview).toBeEnabled();

  await user.click(preview);
  const held = heldCustomReport();
  expect(held?.name).toBe("Germany sales review");
  expect(held?.selection.layout && hasAssessment(held.selection.layout)).toBe(
    true,
  );
  const cards =
    held?.selection.layout?.flatMap((section) => section.cards) ?? [];
  expect(cards.filter((card) => card.kind === "question")).toHaveLength(1);
  expect(cards.find((card) => card.kind === "question")?.question).toEqual({
    entity: "pipeline-current",
    group_by: ["owner_id"],
    measures: [{ fn: "count" }],
    limit: 50,
  });
  // The blocks the evaluation reads name every chart card and no other.
  expect(held?.selection.blocks).toEqual(
    reportingStoryEvaluation.selection.blocks,
  );
  expect(window.location.hash).toContain("preview");
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StoryProviders } from "../../testing/story-utils";
import { ChartBody } from "./reporting.chartbody";
import {
  reportingCatalogClient,
  reportingLeadsEvaluation,
  reportingPartnersEvaluation,
} from "./reporting.story-fixtures";

afterEach(cleanup);

it("aligns breakdown and movement labels with their header and numeric columns at the end", () => {
  const base = reportingLeadsEvaluation.charts[0];
  if (!base) throw new Error("fixture needs a chart");
  const movement: typeof reportingLeadsEvaluation = {
    ...reportingLeadsEvaluation,
    charts: [
      {
        ...base,
        kind: "pipeline_movement",
        opening: 100,
        closing: 130,
        points: [{ key: "new", label: "New", value: 30, status: "ok" }],
      },
    ],
  };
  for (const evaluation of [
    reportingLeadsEvaluation,
    reportingPartnersEvaluation,
    movement,
  ]) {
    for (const chart of evaluation.charts.filter((candidate) =>
      ["breakdown", "pipeline_movement"].includes(candidate.kind),
    )) {
      const view = render(
        <StoryProviders client={reportingCatalogClient()}>
          <ChartBody
            chart={chart}
            evaluation={evaluation}
            onEvidence={() => {}}
          />
        </StoryProviders>,
      );
      const table = view.container.querySelector<HTMLTableElement>(
        ".report-chart-data table",
      );
      if (!table) throw new Error("the breakdown has no data table");
      const headers = within(table).getAllByRole("columnheader", {
        hidden: true,
      });
      for (const row of within(table)
        .getAllByRole("row", { hidden: true })
        .slice(1)) {
        const cells = within(row).getAllByRole("cell", { hidden: true });
        expect(cells).toHaveLength(headers.length);
        expect(cells[0]).not.toHaveClass("datatable-end");
        for (let index = 1; index < cells.length; index++) {
          expect(cells[index]).toHaveClass("datatable-end");
          expect(headers[index]).toHaveClass("datatable-end");
        }
      }
      view.unmount();
    }
  }
});

/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { StoryProviders } from "../../testing/story-utils";
import { ProjectsByPhaseTable } from "./analytics.delivery";

afterEach(cleanup);

it("shows server-converted phase totals and their separate missing-price counts", () => {
  render(
    <StoryProviders>
      <ProjectsByPhaseTable
        locale="en"
        baseCurrency="EUR"
        rows={[
          {
            phase: "delivering",
            projects: 2,
            open_deal_value_minor: 125_000,
            won_deal_value_minor: null,
            open_deal_unpriced: 1,
            won_deal_unpriced: 2,
          },
        ]}
      />
    </StoryProviders>,
  );
  expect(screen.getByText("€1,250.00")).toBeTruthy();
  expect(screen.getByText("1 deal not priced · excluded")).toBeTruthy();
  expect(screen.getByText("2 deals not priced · excluded")).toBeTruthy();
  expect(screen.getByText("—")).toBeTruthy();
  expect(screen.queryByText("€0.00")).toBeNull();
});

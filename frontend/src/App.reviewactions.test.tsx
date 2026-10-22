/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { reviewActions } from "./App.reviewactions";
import { render } from "./testing/settings.testkit";

afterEach(cleanup);

it("shows changed evidence instead of waiting forever when the company is missing", () => {
  render(
    <reviewActions.CoveredDealSuggestion
      open
      companyId={undefined}
      suggestionId="suggestion-one"
    />,
  );
  expect(screen.queryByRole("status")).not.toBeInTheDocument();
  expect(screen.getByText(/no longer available/i)).toBeInTheDocument();
});

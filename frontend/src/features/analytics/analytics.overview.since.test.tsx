/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { overviewCatalog, TEAM } from "./analytics.overview.fixtures";
import { SinceLastReview } from "./analytics.overview.since";
import { forecastEvaluation } from "./reporting.scenarios";
import { reportingStoryRoutes } from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
});

function since(pipelineId: string) {
  return (
    <StoryProviders>
      <SinceLastReview
        catalog={overviewCatalog}
        scope={TEAM}
        pipelineId={pipelineId}
        after="ready"
      />
    </StoryProviders>
  );
}

const chartPoint = () =>
  document.querySelector<HTMLButtonElement>(".waterfall-mark-action");

describe("Since the last capture", () => {
  it("closes its evidence when the selection changes, rather than showing it under the new one", async () => {
    const user = userEvent.setup({ delay: null });
    installFetchStub(reportingStoryRoutes(forecastEvaluation));
    const view = render(since("sales"));
    await waitFor(() => expect(chartPoint()).not.toBeNull());
    chartPoint()?.focus();
    await user.keyboard("{Enter}");
    expect(await screen.findByRole("dialog")).toBeVisible();

    view.rerender(since("renewals"));

    await waitFor(() => expect(chartPoint()).not.toBeNull());
    expect(screen.queryByRole("dialog")).toBeNull();
  });
});

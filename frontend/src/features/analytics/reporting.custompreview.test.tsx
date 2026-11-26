/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import { reportingStoryReport } from "../../testing/reporting.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { heldCustomReport, holdCustomReport } from "./reporting.custombuilder";
import { CustomReportPreview } from "./reporting.custompreview";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
const VIEWER = meFixture().user.id;
const SAVED = { ...reportingStoryReport, id: "report-saved" };

beforeEach(() => {
  holdCustomReport(VIEWER, {
    name: "Sales review",
    description: "Quarterly report",
    audience: "private",
    selection: reportingStoryEvaluation.selection,
  });
  globalThis.history.replaceState(null, "", "#/analytics/reports/preview");
});

afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

function mountPreview(save: () => Promise<Response>) {
  installFetchStub({
    ...reportingStoryRoutes(),
    "POST /analytics/reports": save,
  });
  render(
    <StoryProviders>
      <CustomReportPreview />
    </StoryProviders>,
  );
}

it("opens the saved report in the preview's place and lets the draft go", async () => {
  const user = userEvent.setup({ delay: null });
  const entries = globalThis.history.length;
  mountPreview(async () => jsonResponse(SAVED, 201));
  await user.click(await screen.findByRole("button", { name: "Save report" }));

  await waitFor(() =>
    expect(globalThis.location.hash).toBe("#/analytics/reports/report-saved"),
  );
  expect(globalThis.history.length).toBe(entries);
  expect(heldCustomReport(VIEWER)).toBeUndefined();
});

it("leaves a reader who moved on before the save finished where they are", async () => {
  const user = userEvent.setup({ delay: null });
  let finish: (response: Response) => void = () => undefined;
  const posted = vi.fn(
    () =>
      new Promise<Response>((resolve) => {
        finish = resolve;
      }),
  );
  mountPreview(posted);
  await user.click(await screen.findByRole("button", { name: "Save report" }));
  await waitFor(() => expect(posted).toHaveBeenCalled());
  globalThis.history.replaceState(null, "", "#/deals");
  finish(jsonResponse(SAVED, 201));

  // The draft goes last, once the save has done all it will.
  await waitFor(() => expect(heldCustomReport(VIEWER)).toBeUndefined());
  expect(globalThis.location.hash).toBe("#/deals");
});

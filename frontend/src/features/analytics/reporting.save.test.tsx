/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { SaveReportingDialog } from "./reporting.save";
import {
  reportingStoryReport,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const originalFetch = globalThis.fetch;
afterEach(() => {
  cleanup();
  globalThis.fetch = originalFetch;
  vi.restoreAllMocks();
});

it("saves nothing while a date of its custom range holds text it cannot read", async () => {
  const user = userEvent.setup({ delay: null });
  const write = vi.fn(() => jsonResponse(reportingStoryReport));
  installFetchStub({
    ...reportingStoryRoutes(),
    "PATCH /analytics/reports/report": write,
  });
  render(
    <StoryProviders>
      <SaveReportingDialog
        selection={{ ...reportingStoryReport.selection, period: "custom" }}
        report={reportingStoryReport}
        onClose={() => {}}
        onSaved={() => {}}
      />
    </StoryProviders>,
  );
  const through = screen.getByRole<HTMLInputElement>("textbox", {
    name: "Through",
  });
  // The browser's Enter submits through the button outside the form; user-event leaves that out.
  const submitOnEnter = (event: KeyboardEvent) => {
    if (event.key === "Enter") through.form?.requestSubmit();
  };
  through.addEventListener("keydown", submitOnEnter);
  await user.clear(through);
  await user.type(through, "31/02/2026{Enter}");

  expect(await screen.findByRole("alert")).toHaveTextContent(
    "Enter a date as DD/MM/YYYY.",
  );
  expect(through).toHaveFocus();
  expect(write).not.toHaveBeenCalled();
  through.removeEventListener("keydown", submitOnEnter);
});

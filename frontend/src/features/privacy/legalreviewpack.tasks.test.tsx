// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import { review } from "../../testing/legalreviews.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { LegalReviewPack } from "./legalreviewpack";

afterEach(cleanup);

it("prints a frozen task that has only a date as the day it was set for", () => {
  const withTask = review();
  withTask.packet.tasks = [
    {
      activity_id: "019f7e65-0000-7000-8000-0000000000a1",
      subject: "Send the redlines",
      // Picked for 16 Nov in Los Angeles; 07:59 UTC on the 17th.
      due_at: "2026-11-17T07:59:59Z",
      due_has_time: false,
      due_intent: {
        wall_clock: "2026-11-16T23:59:59",
        timezone: "America/Los_Angeles",
      },
    },
  ];
  render(
    <StoryProviders>
      <LegalReviewPack review={withTask} />
    </StoryProviders>,
  );
  const line = screen.getByText(/^Due /);
  expect(line.textContent).toContain("16");
  expect(line.textContent).not.toContain("17");
  expect(line.textContent).not.toMatch(/\d:\d\d/);
});

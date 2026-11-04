/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { en } from "../i18n/en";
import { RecordZoneProvider } from "../lib/recordzone";
import { StoryProviders } from "../testing/story-utils";
import { TaskTimeField } from "./followuptaskfields";

afterEach(cleanup);

// The record's clock in this suite: Berlin, which skips 02:00-03:00 on
// 29 March 2026.
const ZONE = "Europe/Berlin";

function field(day: string, time: string) {
  render(
    <StoryProviders>
      <RecordZoneProvider zone={ZONE}>
        <TaskTimeField day={day} value={time} onChange={() => {}} />
      </RecordZoneProvider>
    </StoryProviders>,
  );
}

// A picked time the clock skips that day is due at the first minute after the
// gap. The field says so, so the rep sees 03:00 rather than finding it later.
it("says when a picked time falls in the hour the clocks skip", () => {
  field("2026-03-29", "02:30");
  expect(
    screen.getByText(
      en["log.dueTimeSkipped"]
        .replace("{time}", "02:30")
        .replace("{actual}", "03:00"),
    ),
  ).toBeTruthy();
});

it("says nothing about a time the clock reads that day", () => {
  field("2026-03-30", "02:30");
  expect(screen.queryByText(/clocks go forward/)).toBeNull();
});

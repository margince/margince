// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { browserZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { activityTimeline } from "./activitytimeline";
import { TimelineRow } from "./timeline";

type Activity = components["schemas"]["Activity"];

const REP = "6c1e0f52-8a0b-4f7e-9d3c-2b5a7e1f0c44";

function task(fields: Partial<Activity>): Activity {
  return {
    id: "a-task",
    kind: "task",
    subject: "Call Anna about pricing",
    is_done: false,
    offers_email: false,
    occurred_at: "2026-11-12T09:00:00Z",
    occurred_precision: "instant",
    source: "manual",
    captured_by: `human:${REP}`,
    created_at: "2026-11-12T09:00:00Z",
    updated_at: "2026-11-12T09:00:00Z",
    ...fields,
  };
}

function renderRow(row: Activity) {
  const [entry] = activityTimeline([row], REP);
  if (!entry) throw new Error("the adapter dropped the row");
  render(
    <LocaleProvider initial="en">
      <ul>
        <TimelineRow entry={entry} zone={browserZone()} />
      </ul>
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("a saved activity whose words a model polished", () => {
  it("is marked AI-assisted on the timeline", () => {
    renderRow(task({ ai_assisted: true }));
    expect(screen.getByText(en["co.assistant.aiTag"])).toBeVisible();
  });

  it("carries no mark when the rep's own words were saved", () => {
    renderRow(task({}));
    expect(screen.queryByText(en["co.assistant.aiTag"])).toBeNull();
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "../../api/schema";
import { company360 } from "../../features/companies/company.fixtures";
import { LocaleProvider } from "../../i18n";
import { RecordSpine } from "./spine";

type View = components["schemas"]["Company360"];
type Row = NonNullable<View["activities"]>["data"][number];

afterEach(cleanup);

function email(
  id: string,
  at: string,
  threadKey: string,
  direction: "inbound" | "outbound",
): Row {
  return {
    id,
    kind: "email",
    is_done: true,
    subject: `Thread ${threadKey}`,
    occurred_at: at,
    thread_key: threadKey,
    links: [],
    direction,
    meeting_status: null,
    email_summary: null,
    source: "manual",
    captured_by: "human:test",
    created_at: at,
    updated_at: at,
  };
}

// They wrote, we answered a week later, and nothing has come back in the
// twelve days since.
const view: View = {
  ...company360,
  as_of: "2026-08-13T09:00:00Z",
  last_inbound_at: "2026-07-25T09:00:00Z",
  last_outbound_at: "2026-08-01T09:00:00Z",
  activities: {
    data: [
      email("m-2", "2026-08-01T09:00:00Z", "t-2", "outbound"),
      email("m-1", "2026-07-25T09:00:00Z", "t-1", "inbound"),
    ],
    page: { has_more: false, next_cursor: null },
  },
  next_steps: { data: [], page: { has_more: false, next_cursor: null } },
};

function columns(): string[] {
  render(
    <LocaleProvider initial="en">
      <RecordSpine source={view} />
    </LocaleProvider>,
  );
  const track = document.querySelector<HTMLElement>(".co-spine-track");
  return (track?.style.gridTemplateColumns ?? "").split(/\s(?=minmax|max)/);
}

// The thread runs on one time scale. Each stop is as wide as the days until
// the next. A week's gap between two conversations and twelve days of silence
// keep their proportion.
describe("the thread's time scale", () => {
  it("sizes each stop by the days until the next one", () => {
    expect(columns()).toEqual([
      "minmax(6.5rem, 7fr)",
      "minmax(6.5rem, 1fr)",
      "minmax(6.5rem, 12fr)",
      "max-content",
    ]);
  });
});

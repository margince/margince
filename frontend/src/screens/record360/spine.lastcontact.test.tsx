/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";

import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { company360 } from "../company.fixtures";
import { RecordSpine } from "./spine";

// The newest stop is titled "Last contact". A note we wrote and a meeting
// nobody held sit on the timeline too, and neither is contact.

type View = components["schemas"]["Company360"];
type Row = NonNullable<View["activities"]>["data"][number];

const SPOKE = "2026-08-18T09:00:00Z";

function row(
  id: string,
  kind: Row["kind"],
  subject: string,
  at: string,
  meetingStatus: Row["meeting_status"] = null,
): Row {
  return {
    id,
    kind,
    is_done: false,
    subject,
    occurred_at: at,
    thread_key: null,
    links: [],
    direction: null,
    meeting_status: meetingStatus,
    email_summary: null,
    source: "manual",
    captured_by: "human:test",
    created_at: at,
    updated_at: at,
  };
}

afterEach(cleanup);

it("skips a note and a called-off meeting when picking the last contact", () => {
  const view: View = {
    ...company360,
    as_of: "2026-08-25T09:00:00Z",
    last_outbound_at: SPOKE,
    activities: {
      data: [
        row(
          "a-note",
          "note",
          "No reply after two chasers",
          "2026-08-20T09:00:00Z",
        ),
        row(
          "a-off",
          "meeting",
          "Review that never happened",
          "2026-08-19T09:00:00Z",
          "canceled",
        ),
        row("a-call", "call", "Pricing call", SPOKE),
      ],
      page: { has_more: false, next_cursor: null },
    },
  };
  render(
    <LocaleProvider initial="en">
      <RecordSpine source={view} commercial={view.state_strip?.commercial} />
    </LocaleProvider>,
  );

  expect(screen.getByText("Pricing call")).toBeTruthy();
  expect(screen.queryByText("No reply after two chasers")).toBeNull();
  expect(screen.queryByText("Review that never happened")).toBeNull();
});

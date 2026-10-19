/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { company360 } from "../../companies/company.fixtures";
import { RecordSpine, type SpineSource } from "./spine";
import { timelineSpineSource } from "./timelinespine";

// The newest stop is titled "Last contact". A note we wrote and a meeting
// nobody held sit on the timeline too, and neither is contact.

type View = components["schemas"]["Company360"];
type Row = NonNullable<View["activities"]>["data"][number];

const AS_OF = "2026-08-25T09:00:00Z";
const SPOKE = "2026-08-18T09:00:00Z";
const DAYS_AGO_14 = "2026-08-11T09:00:00Z";
const DAYS_AGO_5 = "2026-08-20T09:00:00Z";

function row(
  id: string,
  kind: Row["kind"],
  subject: string,
  at: string,
  extra: Partial<Row> = {},
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
    meeting_status: null,
    email_summary: null,
    source: "manual",
    captured_by: "human:test",
    created_at: at,
    updated_at: at,
    ...extra,
  };
}

function account(rows: readonly Row[], overrides: Partial<View> = {}): View {
  return {
    ...company360,
    as_of: AS_OF,
    last_inbound_at: null,
    activities: {
      data: [...rows],
      page: { has_more: false, next_cursor: null },
    },
    ...overrides,
  };
}

function draw(source: SpineSource) {
  render(
    <LocaleProvider initial="en">
      <RecordSpine source={source} />
    </LocaleProvider>,
  );
}

// The stop drawn just before the gap, and the gap itself, as date and title.
function aroundTheGap(): { before?: string[]; gap?: string[] } {
  const stops = [...document.querySelectorAll(".co-spine-stop")].map(
    (node) => ({
      tone: node.className,
      text: [
        node.querySelector(".co-spine-when")?.textContent ?? "",
        node.querySelector(".co-spine-title")?.textContent ?? "",
      ],
    }),
  );
  const at = stops.findIndex((stop) => stop.tone.includes("co-spine-gap"));
  return at < 1 ? {} : { before: stops[at - 1].text, gap: stops[at].text };
}

afterEach(cleanup);

it("skips a note and a called-off meeting when picking the last contact", () => {
  draw(
    account(
      [
        row("a-note", "note", "No reply after two chasers", DAYS_AGO_5),
        row(
          "a-off",
          "meeting",
          "Review that never happened",
          "2026-08-19T09:00:00Z",
          { meeting_status: "canceled" },
        ),
        row("a-call", "call", "Pricing call", SPOKE),
      ],
      { last_outbound_at: SPOKE },
    ),
  );

  expect(screen.getByText("Pricing call")).toBeTruthy();
  expect(screen.queryByText("No reply after two chasers")).toBeNull();
  expect(screen.queryByText("Review that never happened")).toBeNull();
});

const ourEmail = row("a-mail", "email", "Proposal", DAYS_AGO_14, {
  direction: "outbound",
});

describe("the gap counts from the last contact drawn before it", () => {
  it("counts from a meeting held after our last email", () => {
    draw(
      account(
        [
          row("a-met", "meeting", "Review", DAYS_AGO_5, {
            meeting_status: "held",
          }),
          ourEmail,
        ],
        { last_outbound_at: DAYS_AGO_14 },
      ),
    );

    expect(aroundTheGap()).toEqual({
      before: ["20 Aug 2026", "Last contact"],
      gap: ["5 days", "Never replied"],
    });
  });

  it("counts from a hand-logged call after our last email", () => {
    draw(
      account([row("a-call", "call", "Check-in", DAYS_AGO_5), ourEmail], {
        last_outbound_at: DAYS_AGO_14,
      }),
    );

    expect(aroundTheGap().gap).toEqual(["5 days", "Never replied"]);
  });

  it("counts from our email sent after the meeting", () => {
    draw(
      account(
        [
          row("a-mail", "email", "Proposal", DAYS_AGO_5, {
            direction: "outbound",
          }),
          row("a-met", "meeting", "Review", DAYS_AGO_14, {
            meeting_status: "held",
          }),
        ],
        { last_outbound_at: DAYS_AGO_5 },
      ),
    );

    expect(aroundTheGap()).toEqual({
      before: ["20 Aug 2026", "Last contact"],
      gap: ["5 days", "Never replied"],
    });
  });

  it("does not restart the count at a cancelled meeting", () => {
    draw(
      account(
        [
          row("a-off", "meeting", "Review", DAYS_AGO_5, {
            meeting_status: "canceled",
          }),
          ourEmail,
        ],
        { last_outbound_at: DAYS_AGO_14 },
      ),
    );

    expect(aroundTheGap()).toEqual({
      before: ["11 Aug 2026", "Last contact"],
      gap: ["14 days", "Never replied"],
    });
  });

  it("draws no gap once they wrote after both", () => {
    draw(
      account(
        [
          row("a-reply", "email", "Re: Proposal", "2026-08-23T09:00:00Z", {
            direction: "inbound",
          }),
          row("a-met", "meeting", "Review", DAYS_AGO_5),
          ourEmail,
        ],
        {
          last_outbound_at: DAYS_AGO_14,
          last_inbound_at: "2026-08-23T09:00:00Z",
        },
      ),
    );

    expect(aroundTheGap()).toEqual({});
    expect(screen.queryByText("No contact since")).toBeNull();
  });

  it("draws no gap after a call they placed to answer our email", () => {
    draw(
      account(
        [
          row("a-call", "call", "Callback", DAYS_AGO_5, {
            direction: "inbound",
          }),
          ourEmail,
        ],
        { last_outbound_at: DAYS_AGO_14, last_inbound_at: DAYS_AGO_5 },
      ),
    );

    expect(aroundTheGap()).toEqual({});
  });

  it("counts from the account's last meeting when the timeline holds none", () => {
    // A company or contact read whose timeline is withheld still carries
    // `health.last_meeting_at`, and the head is dated from it.
    draw(
      account([], {
        last_outbound_at: DAYS_AGO_14,
        health: { ...company360.health, last_meeting_at: DAYS_AGO_5 },
      }),
    );

    expect(aroundTheGap()).toEqual({
      before: ["20 Aug 2026", "Last contact"],
      gap: ["5 days", "Never replied"],
    });
  });

  it("counts from an email with no recorded direction after the meeting", () => {
    // Drawn as the last contact, so the gap counts from it too.
    draw(
      account(
        [
          row("a-note", "email", "Notes from the visit", DAYS_AGO_5),
          row("a-met", "meeting", "Visit", "2026-08-17T09:00:00Z", {
            meeting_status: "held",
          }),
          ourEmail,
        ],
        { last_outbound_at: DAYS_AGO_14 },
      ),
    );

    expect(aroundTheGap()).toEqual({
      before: ["20 Aug 2026", "Last contact"],
      gap: ["5 days", "Never replied"],
    });
  });

  it("counts from the server's last contact when it is newer than every loaded row", () => {
    // The server reads every page; the rows here are only the first one.
    draw(
      account([ourEmail], {
        last_outbound_at: DAYS_AGO_14,
        last_contact: {
          at: "2026-08-22T09:00:00Z",
          kind: "meeting",
          activity_id: "00000000-0000-0000-0000-0000000000aa",
        },
      }),
    );

    expect(aroundTheGap()).toEqual({
      before: ["22 Aug 2026", "Last contact"],
      gap: ["3 days", "Never replied"],
    });
  });

  it("counts from the meeting on a deal's timeline page", () => {
    draw(
      timelineSpineSource(
        [
          row("a-met", "meeting", "Review", DAYS_AGO_5, {
            meeting_status: "held",
          }),
          ourEmail,
        ],
        AS_OF,
        false,
      ),
    );

    expect(aroundTheGap().gap).toEqual(["5 days", "Never replied"]);
  });
});

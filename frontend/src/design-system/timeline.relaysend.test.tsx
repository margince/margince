// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { activityTimeline } from "./activitytimeline";
import { TimelineRow } from "./timeline";

type Activity = components["schemas"]["Activity"];
type Delivery = components["schemas"]["EmailDelivery"];

const REP = "6c1e0f52-8a0b-4f7e-9d3c-2b5a7e1f0c44";
const COLLEAGUE = "0b9d2a71-3c5e-4f18-8a6d-7e2c1b4f9a03";

// A privacy notice the relay staged at a rep's request, as the activity list
// answers it: captured by the rep, sent by the installation.
function relayedNotice(
  controllerSend: Activity["controller_send"],
  delivery: Delivery = { state: "pending" },
): Activity {
  return {
    id: "a-relay",
    kind: "email",
    subject: "Information about your personal data",
    direction: "outbound",
    is_done: false,
    offers_email: false,
    occurred_at: "2026-10-20T09:00:00Z",
    occurred_precision: "instant",
    source: "manual",
    captured_by: `human:${REP}`,
    created_at: "2026-10-20T09:00:00Z",
    updated_at: "2026-10-20T09:00:00Z",
    controller_send: controllerSend,
    email_summary: {
      activity_id: "a-relay",
      subject: "Information about your personal data",
      preview: "We hold personal data about you.",
      occurred_at: "2026-10-20T09:00:00Z",
      occurred_precision: "instant",
      direction: "outbound",
      counterparty: "Lena Hoffmann",
      attachment_count: 0,
      move: "none",
      display_status: "team",
      delivery,
      version: 1,
    },
  };
}

function renderRow(row: Activity, viewer = REP) {
  const [entry] = activityTimeline([row], viewer);
  if (!entry) throw new Error("the adapter dropped the row");
  render(
    <LocaleProvider initial="en">
      <ul>
        <TimelineRow entry={entry} zone="UTC" />
      </ul>
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("a mail the installation's relay sent for a rep", () => {
  it("says Margince sent it for the reader, from which address, and how far it got", () => {
    renderRow(
      relayedNotice({ from_address: "privacy@kps.test" }, { state: "bounced" }),
    );

    expect(screen.getByText(en["trust.relayForYou"])).toBeVisible();
    expect(screen.getByText("From privacy@kps.test")).toBeVisible();
    expect(screen.getByText("Did not reach Lena Hoffmann")).toBeVisible();
  });

  it("names no address the relay did not record", () => {
    renderRow(relayedNotice({ from_address: null }, { state: "sent" }));

    expect(screen.getByText(en["trust.relayForYou"])).toBeVisible();
    expect(screen.queryByText(/^From /)).toBeNull();
    expect(screen.getByText("Sent to Lena Hoffmann")).toBeVisible();
  });

  it("never tells a colleague it was sent for them", () => {
    renderRow(relayedNotice({ from_address: "privacy@kps.test" }), COLLEAGUE);

    expect(screen.getByText(en["trust.relayForColleague"])).toBeVisible();
    expect(screen.queryByText(en["trust.relayForYou"])).toBeNull();
  });

  it("leaves a mail the rep sent from their own mailbox as it was", () => {
    renderRow(relayedNotice(undefined, { state: "sent" }));

    expect(screen.getByText("Sent to Lena Hoffmann")).toBeVisible();
    expect(screen.queryByText(/Sent by Margince/)).toBeNull();
    expect(screen.queryByText(/^From /)).toBeNull();
  });
});

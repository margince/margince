// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { MoneyPane, ThreadFold } from "./glance";

type Organization360 = components["schemas"]["Organization360"];
type Activity = components["schemas"]["Activity"];

afterEach(cleanup);

function drawMoney(loading: boolean) {
  return render(
    <LocaleProvider initial="en">
      <MoneyPane
        organizationId="o-1"
        loading={loading}
        readOnly={false}
        onAllDeals={() => undefined}
      />
    </LocaleProvider>,
  );
}

// The projects group reads its own state the way the deals group does. An
// absent list handed straight to the links section drew "No projects yet"
// with an Attach verb while the 360 was still on its way — an invitation to
// act on a section that had not answered.
describe("the money pane's projects group", () => {
  it("holds the loading state, not an empty plate, while the 360 is in flight", () => {
    drawMoney(true);
    expect(screen.queryByRole("button", { name: "Attach project" })).toBeNull();
    expect(screen.queryByText("No projects yet")).toBeNull();
    // The group is still named, so a reader can tell WHICH reading is on its
    // way, one level under the pane's own title.
    expect(
      screen.getByRole("heading", { level: 3, name: "Projects" }),
    ).toBeTruthy();
  });

  it("says the section could not be read when the 360 failed", () => {
    drawMoney(false);
    expect(screen.queryByRole("button", { name: "Attach project" })).toBeNull();
    expect(screen.queryByText("No projects yet")).toBeNull();
  });
});

// The fold draws the account's messages in FULL — sender, subject, preview,
// access badge — so a reader who can see the message expects to open it. It
// shipped without an opener, which is the one failure a full-fidelity preview
// must not have: the row said "there is a message here" and answered nothing.
describe("the folded thread on the account's 360", () => {
  const EMAIL_ID = "01a05500-0000-7000-8000-00000000dd01";

  const emailRow: Activity = {
    id: EMAIL_ID,
    kind: "email",
    occurred_at: "2026-08-29T09:15:00Z",
    subject: "Re: the renewal quote",
    body: "Can you hold the price until Friday?",
    direction: "inbound",
    content_state: "available",
    source: "manual",
    captured_by: "human:u-1",
    created_at: "2026-08-29T09:15:00Z",
    updated_at: "2026-08-29T09:15:00Z",
    version: 4,
    is_done: false,
    email_summary: {
      activity_id: EMAIL_ID,
      occurred_at: "2026-08-29T09:15:00Z",
      version: 4,
      subject: "Re: the renewal quote",
      preview: "Can you hold the price until Friday?",
      counterparty: "Dana Buyer",
      direction: "inbound",
      display_status: "team",
      move: "needs_reply",
      attachment_count: 0,
    },
  };

  function threadView(): Organization360 {
    return {
      as_of: "2026-08-29T10:00:00Z",
      organization: {
        id: "o-1",
        name: "Nordwind Logistik",
        display_name: "Nordwind Logistik",
        source: "manual",
        captured_by: "human:u-1",
        version: 1,
        created_at: "2026-01-05T09:00:00Z",
        updated_at: "2026-08-29T09:15:00Z",
      },
      activities: { data: [emailRow], page: { has_more: false } },
      sections_omitted: [],
    };
  }

  function drawThread(onOpenRecord?: (kind: string, id: string) => void) {
    return render(
      <LocaleProvider initial="en">
        <ThreadFold
          view={threadView()}
          loading={false}
          onOpenRecord={onOpenRecord}
        />
      </LocaleProvider>,
    );
  }

  it("opens the message it shows, through the page's own router", async () => {
    const user = userEvent.setup();
    const onOpenRecord = vi.fn();
    drawThread(onOpenRecord);

    // The row is a control, not a paragraph — the distinction the defect erased.
    const row = screen.getByRole("button", { name: /Re: the renewal quote/ });
    expect(row.getAttribute("aria-haspopup")).toBe("dialog");
    await user.click(row);

    // The same door every cited chip on this account takes, so the fold cannot
    // drift from the spine beside it.
    expect(onOpenRecord).toHaveBeenCalledWith("activity", EMAIL_ID);
  });

  it("still draws the message when the host mounts no reader", () => {
    drawThread(undefined);
    expect(screen.getByText("Re: the renewal quote")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /renewal quote/ })).toBeNull();
  });
});

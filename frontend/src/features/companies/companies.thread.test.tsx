// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { formatDateTime } from "../../format/format";
import { LocaleProvider } from "../../i18n";
import { RecordZoneProvider } from "../../lib/recordzone";
import {
  company,
  company360,
  companyBackstop,
  jsonResponse,
} from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import { RecordShell } from "../../testing/recordshell.testkit";
import { CompanyScreen } from "./companies";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

// Fourteen hours off UTC, so a drawer dated in the viewer's zone reads otherwise.
const RECORD_ZONE = "Pacific/Kiritimati";

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordZoneProvider zone={RECORD_ZONE}>
          <RecordShell>{ui}</RecordShell>
        </RecordZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const MESSAGES: Record<string, { subject: string; at: string }> = {
  "a-new": { subject: "About capacity", at: "2026-05-30T09:00:00Z" },
  "a-old": { subject: "About scope", at: "2026-05-20T09:00:00Z" },
};

function mail(id: string, links: unknown[] = []) {
  const { subject, at } = MESSAGES[id];
  return {
    id,
    kind: "email",
    subject,
    occurred_at: at,
    direction: "inbound",
    is_done: false,
    links,
    email_summary: {
      activity_id: id,
      occurred_at: at,
      version: 1,
      subject,
      preview: "Can you hold the price until Friday?",
      counterparty: "Dana Buyer",
      direction: "inbound",
      display_status: "team",
      move: "none",
      attachment_count: 0,
    },
    source: "manual",
    captured_by: "human:u1",
    created_at: at,
    updated_at: at,
  };
}

function presentation(id: string) {
  return {
    id,
    lifecycle: "delivered",
    occurred_at: MESSAGES[id].at,
    summary: mail(id).email_summary,
    body: "Can you hold the price until Friday?",
    thread_key: null,
    from: [{ address: "dana@acme.test", display_name: "Dana Buyer" }],
    to: [{ address: "rep@demo.test", display_name: "Lena Fischer" }],
    cc: [],
    bcc: [],
    bcc_withheld: false,
    attachments: [],
    links: [],
    thread: { members: [], next_cursor: null },
    access: {
      content_state: "available",
      audience: "workspace",
      selected_members: [],
      display_status: "team",
      can_change: false,
      change_mode: "message",
      held_by_others: false,
    },
    can_reply: false,
    can_relink: false,
    version: 1,
  };
}

function drawAccount() {
  stubFetch(
    async (url) => {
      const asked = /\/activities\/([^/]+)\/email-presentation/.exec(url);
      return asked
        ? jsonResponse(presentation(asked[1]))
        : companyBackstop(url);
    },
    {
      company360: {
        ...company360,
        activities: {
          data: [
            mail("a-new", [{ entity_type: "deal", entity_id: "d-7" }]),
            mail("a-old"),
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
    },
  );
  return render(<CompanyScreen id="o-1" />);
}

// The Latest card under How it stands lists the newest exchanges, and each row
// opens where the rest of the page opens it.
describe("the account overview's Latest card opens the messages it names", () => {
  function latest(_container: HTMLElement): HTMLElement {
    const card = screen
      .queryByRole("heading", { name: "Latest" })
      ?.closest<HTMLElement>("section");
    if (!card) {
      throw new Error("the overview draws no Latest card");
    }
    return card;
  }

  it("opens a message in the page's email drawer, dated in the record's zone", async () => {
    const user = userEvent.setup();
    const { container } = drawAccount();
    await screen.findByRole("heading", { name: company.display_name });
    await screen.findAllByText("About scope");

    await user.click(
      await within(latest(container)).findByRole("button", {
        name: /About scope/,
      }),
    );

    const drawer = await screen.findByRole("dialog", { name: "About scope" });
    expect(
      await within(drawer).findByText(
        formatDateTime(MESSAGES["a-old"].at, "en", RECORD_ZONE),
      ),
    ).toBeTruthy();
  });
});

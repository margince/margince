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
import { RecordZoneProvider } from "../app/recordzone";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { formatDateTime } from "../format/format";
import { LocaleProvider } from "../i18n";
import { CompanyScreen } from "./companies";
import {
  company,
  company360,
  companyBackstop,
  jsonResponse,
  stubFetch,
} from "./company.fixtures";

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

const SUBJECTS: Record<string, string> = {
  "a-new": "About capacity",
  "a-old": "About scope",
};

function mail(id: string, at: string, links: unknown[] = []) {
  return {
    id,
    kind: "email",
    subject: SUBJECTS[id],
    occurred_at: at,
    direction: "inbound",
    is_done: false,
    links,
    email_summary: {
      activity_id: id,
      occurred_at: at,
      version: 1,
      subject: SUBJECTS[id],
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
    occurred_at: "2026-05-30T09:00:00Z",
    summary: mail(id, "2026-05-30T09:00:00Z").email_summary,
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
            mail("a-new", "2026-05-30T09:00:00Z", [
              { entity_type: "deal", entity_id: "d-7" },
            ]),
            mail("a-old", "2026-05-20T09:00:00Z"),
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
    },
  );
  return render(<CompanyScreen id="o-1" />);
}

describe("the account overview's thread opens the messages it names", () => {
  it("opens a conversation on the spine in the page's email drawer, dated in the record's zone", async () => {
    const user = userEvent.setup();
    const { container } = drawAccount();
    await screen.findByRole("heading", { name: company.display_name });

    const spine = container.querySelector<HTMLElement>(".co-spine");
    if (!spine) {
      throw new Error("the overview draws no thread spine");
    }
    await user.click(
      await within(spine).findByRole("button", { name: /About scope/ }),
    );

    const drawer = await screen.findByRole("dialog", { name: "About scope" });
    expect(
      await within(drawer).findByText(
        formatDateTime("2026-05-30T09:00:00Z", "en", RECORD_ZONE),
      ),
    ).toBeTruthy();
  });

  it("opens a message row in the folded thread in the page's email drawer", async () => {
    const user = userEvent.setup();
    const { container } = drawAccount();
    await screen.findByRole("heading", { name: company.display_name });

    const fold = container.querySelector<HTMLElement>(".co-thread-fold");
    const summary = fold?.querySelector("summary");
    if (!fold || !summary) {
      throw new Error("the overview draws no thread fold to open");
    }
    await user.click(summary);
    await user.click(
      await within(fold).findByRole("button", { name: /About capacity/ }),
    );

    expect(
      await screen.findByRole("dialog", { name: "About capacity" }),
    ).toBeTruthy();
  });

  it("sends the folded thread's deal chip to the deal", async () => {
    const user = userEvent.setup();
    const { container } = drawAccount();
    await screen.findByRole("heading", { name: company.display_name });

    const fold = container.querySelector<HTMLElement>(".co-thread-fold");
    const summary = fold?.querySelector("summary");
    if (!fold || !summary) {
      throw new Error("the overview draws no thread fold to open");
    }
    await user.click(summary);
    await user.click(
      await within(fold).findByRole("button", { name: "on a deal" }),
    );

    expect(window.location.hash).toContain("d-7");
  });
});

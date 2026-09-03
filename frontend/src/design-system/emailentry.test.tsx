// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { EmailDetail } from "./emaildetail";
import { EmailEntry } from "./emailentry";
import { EmailReference } from "./emailreference";

// The row, the citation and the drawer, held to the promises the arrangement
// makes: one layout wherever it is mounted, a withheld row that stays visible
// and says nothing, and a drawer that costs a request only when it is opened.

type EmailSummary = components["schemas"]["EmailSummary"];

const READABLE: EmailSummary = {
  activity_id: "11111111-1111-4111-8111-111111111111",
  occurred_at: "2026-09-01T09:12:00Z",
  display_status: "team",
  attachment_count: 2,
  move: "needs_reply",
  version: 3,
  subject: "Angebot Q4",
  preview: "Können wir Dienstag sprechen?",
  direction: "inbound",
  counterparty: "Ana Sommer +2",
};

// Carries a subject and a preview ON PURPOSE. The server strips them, so a
// fixture without them tests nothing: the row would draw the same whether it
// honoured the status or ignored it, and the assertions below would pass on a
// component that had forgotten how to withhold.
const WITHHELD: EmailSummary = {
  ...READABLE,
  activity_id: "22222222-2222-4222-8222-222222222222",
  display_status: "withheld",
};

const PRESENTATION = {
  id: READABLE.activity_id,
  lifecycle: "delivered",
  occurred_at: READABLE.occurred_at,
  summary: READABLE,
  body: "Können wir Dienstag sprechen?\n\nViele Grüße\nAna",
  from: [{ address: "ana@example.test", display_name: "Ana Sommer" }],
  to: [],
  cc: [],
  bcc: [],
  bcc_withheld: false,
  attachments: [],
  links: [],
  access: {
    content_state: "available",
    display_status: "team",
    can_change: false,
    change_mode: "none",
  },
  can_reply: true,
  can_relink: false,
  version: 3,
};

function jsonOnce(body: unknown) {
  return vi.fn(
    async () =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
  );
}

function wrap(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider>{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("EmailEntry", () => {
  it("says who wrote, what about, and whose move it is", () => {
    wrap(<EmailEntry summary={READABLE} timestamp="1 Sep 09:12" />);
    expect(screen.getByText(/Ana Sommer \+2/)).toBeInTheDocument();
    expect(screen.getByText("Angebot Q4")).toBeInTheDocument();
    expect(
      screen.getByText("Können wir Dienstag sprechen?"),
    ).toBeInTheDocument();
    expect(screen.getByText("Needs reply")).toBeInTheDocument();
    // The count is the file's own, not a guess from the body.
    expect(screen.getByText("2")).toBeInTheDocument();
  });

  it("keeps a withheld row visible and says nothing in it", () => {
    wrap(<EmailEntry summary={WITHHELD} timestamp="1 Sep 09:12" />);
    // Visible, so a reader can tell a limited conversation from one that never
    // happened.
    expect(screen.getByText("1 Sep 09:12")).toBeInTheDocument();
    expect(screen.getByText("Withheld")).toBeInTheDocument();
    // And nothing of what was said or who said it, though the fixture carries
    // all of it. The counterparty matters as much as the subject: a name
    // beside a message the reader may not open still says who this person is
    // talking to.
    expect(screen.queryByText("Angebot Q4")).not.toBeInTheDocument();
    expect(
      screen.queryByText("Können wir Dienstag sprechen?"),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/Ana Sommer/)).not.toBeInTheDocument();
    // No move claim and no file count either: both describe the message.
    expect(screen.queryByText("Needs reply")).not.toBeInTheDocument();
    expect(screen.queryByText("2")).not.toBeInTheDocument();
  });

  it("opens on Enter and on Space, and says it opens a dialog", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    wrap(
      <EmailEntry summary={READABLE} timestamp="1 Sep 09:12" onOpen={onOpen} />,
    );
    const row = screen.getByRole("button");
    expect(row).toHaveAttribute("aria-haspopup", "dialog");
    row.focus();
    await user.keyboard("{Enter}");
    await user.keyboard(" ");
    expect(onOpen).toHaveBeenCalledTimes(2);
  });

  it("is not a control when there is nothing to open", () => {
    wrap(<EmailEntry summary={READABLE} timestamp="1 Sep 09:12" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });
});

describe("EmailReference", () => {
  it("names a message without previewing it", () => {
    wrap(<EmailReference subject="Angebot Q4" occurredAt="1 Sep" />);
    expect(screen.getByText("Angebot Q4")).toBeInTheDocument();
    // No preview and no access badge: a citation is not a reading, and a badge
    // without the message it qualifies is a fact floating free.
    expect(screen.queryByText(/Können wir/)).not.toBeInTheDocument();
    expect(screen.queryByText("Team")).not.toBeInTheDocument();
  });

  it("prints nothing of a withheld message, and does not open it", () => {
    const onOpen = vi.fn();
    wrap(
      <EmailReference
        subject="Angebot Q4"
        occurredAt="1 Sep"
        withheld
        onOpen={onOpen}
      />,
    );
    // The subject the caller passed is not this reader's to see, and an opener
    // the caller passed leads to a message they may not read.
    expect(screen.queryByText("Angebot Q4")).not.toBeInTheDocument();
    expect(screen.getByText("Not shared with you")).toBeInTheDocument();
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("says No subject rather than drawing an empty line", () => {
    wrap(<EmailReference subject={null} />);
    expect(screen.getByText("No subject")).toBeInTheDocument();
  });
});

describe("EmailDetail", () => {
  it("asks for the message only when it is opened", async () => {
    const fetchSpy = jsonOnce(PRESENTATION);
    vi.stubGlobal("fetch", fetchSpy);

    // A row on its own costs nothing: the summary it draws came with the list.
    wrap(<EmailEntry summary={READABLE} timestamp="1 Sep 09:12" />);
    expect(fetchSpy).not.toHaveBeenCalled();
    cleanup();

    wrap(
      <EmailDetail
        activityId={READABLE.activity_id}
        onClose={() => {}}
        formatWhen={() => "1 Sep 09:12"}
      />,
    );
    await waitFor(() => expect(fetchSpy).toHaveBeenCalledTimes(1));
    // The sender's own words, with the sign-off folded away rather than lost.
    await screen.findByText("Können wir Dienstag sprechen?");
    expect(screen.getByText("Show quoted history")).toBeInTheDocument();
    expect(screen.getByText(/Ana Sommer/)).toBeInTheDocument();
  });

  it("asks again on every open, and keeps nothing to repaint", async () => {
    const fetchSpy = jsonOnce(PRESENTATION);
    vi.stubGlobal("fetch", fetchSpy);
    // One client across both opens, as a real session has.
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false, staleTime: 30_000 } },
    });
    const draw = () =>
      render(
        <QueryClientProvider client={client}>
          <LocaleProvider>
            <EmailDetail
              activityId={READABLE.activity_id}
              onClose={() => {}}
              formatWhen={() => "1 Sep 09:12"}
            />
          </LocaleProvider>
        </QueryClientProvider>,
      );

    draw();
    await screen.findByText("Können wir Dienstag sprechen?");
    cleanup();

    // The global staleTime would let this second open skip the request and
    // paint the first one's answer. A message's content is an authorization
    // result, so the reopen has to ask again.
    draw();
    await waitFor(() => expect(fetchSpy).toHaveBeenCalledTimes(2));
  });

  it("says blind recipients exist without naming them", async () => {
    vi.stubGlobal(
      "fetch",
      jsonOnce({
        ...PRESENTATION,
        body: "Kurz.",
        from: [],
        bcc_withheld: true,
      }),
    );
    wrap(
      <EmailDetail
        activityId={READABLE.activity_id}
        onClose={() => {}}
        formatWhen={() => "1 Sep 09:12"}
      />,
    );
    // An empty list would read as "nobody was blind-copied", which is a
    // different fact from "you may not see who was".
    await screen.findByText(/blind-copied and are not shown to you/);
  });

  it("withholds a message the reader is outside the audience of", async () => {
    vi.stubGlobal(
      "fetch",
      jsonOnce({
        ...PRESENTATION,
        // As the server sends it: the projection nulls the subject and the
        // preview on a withheld row, so a fixture that kept them would be
        // testing a response the backend cannot produce.
        summary: { ...WITHHELD, subject: undefined, preview: undefined },
        body: null,
        from: [],
        access: {
          content_state: "withheld",
          display_status: "withheld",
          can_change: false,
          change_mode: "none",
        },
      }),
    );
    wrap(
      <EmailDetail
        activityId={WITHHELD.activity_id}
        onClose={() => {}}
        formatWhen={() => "1 Sep 09:12"}
      />,
    );
    await screen.findByText("Not shared with you");
    // No sender, no words, and no reason: why a message is limited describes
    // what it is about.
    expect(screen.queryByText(/Ana Sommer/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Können wir/)).not.toBeInTheDocument();
  });
});

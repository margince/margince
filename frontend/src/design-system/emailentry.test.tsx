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
import { EmailEntry, EmailWords } from "./emailentry";
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

// An outbound message the provider confirmed, and the same message refused.
// Both carry a counterparty, so what separates them on screen is the delivery
// and nothing else — which is the whole claim.
const DELIVERED: EmailSummary = {
  ...READABLE,
  activity_id: "33333333-3333-4333-8333-333333333333",
  direction: "outbound",
  counterparty: "Ana Sommer",
  delivery: { state: "sent", delivered_at: "2026-09-01T09:13:00Z", files: [] },
};

const PARKED: EmailSummary = {
  ...DELIVERED,
  activity_id: "44444444-4444-4444-8444-444444444444",
  delivery: {
    state: "parked",
    reason: "the channel refused the attachment",
    files: [],
  },
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

describe("EmailWords", () => {
  it("draws the server's preview and nothing around it", () => {
    const { container } = wrap(<EmailWords summary={READABLE} />);
    expect(
      screen.getByText("Können wir Dienstag sprechen?"),
    ).toBeInTheDocument();
    // The words alone. A host mounting this has already drawn the sender and
    // the time, so a second copy of either here is the defect the component
    // exists to prevent.
    expect(container.textContent).toBe("Können wir Dienstag sprechen?");
  });

  // The reason this lives beside EmailEntry rather than in the thread card.
  // WITHHELD carries a preview on purpose, so a component that read the field
  // instead of the rule would print it and this would fail.
  it("prints nothing of a withheld message, whatever the summary carries", () => {
    expect(WITHHELD.preview).toBeTruthy();
    const { container } = wrap(<EmailWords summary={WITHHELD} />);
    expect(container).toBeEmptyDOMElement();
  });

  it("draws nothing when the sender wrote nothing", () => {
    const { container } = wrap(
      <EmailWords summary={{ ...READABLE, preview: null }} />,
    );
    expect(container).toBeEmptyDOMElement();
  });
});

describe("EmailEntry", () => {
  it("says who wrote, what about, and whose move it is", () => {
    wrap(
      <EmailEntry
        summary={READABLE}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
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
    wrap(
      <EmailEntry
        summary={WITHHELD}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    // Visible, so a reader can tell a limited conversation from one that never
    // happened.
    expect(screen.getByText("1 Sep 09:12")).toBeInTheDocument();
    expect(screen.getByText("Withheld")).toBeInTheDocument();
    // And nothing of what was said or who said it, though the fixture carries
    // all of it. The counterparty matters as much as the subject: a name
    // beside a message the reader may not open still says who this contact is
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

  // The defect this file exists to stop coming back: the direction line was
  // glued to the counterparty in the browser, so a row with no name to glue
  // ended on the preposition — "Received from", with nothing after it. That
  // was every withheld row and every row whose summary carries no
  // counterparty, on every 360 timeline.
  it("never ends the direction on a preposition", () => {
    wrap(
      <EmailEntry
        summary={WITHHELD}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    expect(screen.getByText("Received")).toBeInTheDocument();
    expect(screen.queryByText("Received from")).not.toBeInTheDocument();
  });

  // A counterparty the server could not resolve arrives as "", not as null.
  // A bare presence check took that for a name and rendered "Received from "
  // — the same dangling preposition, one space longer.
  it("treats a blank counterparty as no name at all", () => {
    wrap(
      <EmailEntry
        summary={{ ...READABLE, counterparty: "   " }}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    expect(screen.getByText("Received")).toBeInTheDocument();
    expect(screen.queryByText(/^Received from/)).not.toBeInTheDocument();
  });

  it("says the direction WITH the name when there is one", () => {
    wrap(
      <EmailEntry
        summary={READABLE}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    // One sentence from one string, so the two forms cannot drift and a
    // locale can put the name where its grammar wants it.
    expect(screen.getByText(/^Received from Ana Sommer/)).toBeInTheDocument();
  });

  // An outbound row takes the same pair, and the name-less form has to be the
  // OUTBOUND word: a row saying "Received" about a message this workspace sent
  // is worse than the dangling preposition it replaced.
  it("keeps the direction right on an outbound row with no name", () => {
    wrap(
      <EmailEntry
        // A READABLE row with nothing to glue a name from. It used to borrow
        // the withheld fixture for that, which now earns a verb of its own:
        // this case is about the missing NAME, so the row under it has to be
        // one the reader may read.
        summary={{
          ...READABLE,
          direction: "outbound",
          counterparty: undefined,
        }}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    expect(screen.getByText("Sent")).toBeInTheDocument();
    expect(screen.queryByText(/^Received/)).not.toBeInTheDocument();
    // The symmetric half: "Sent" alone is right, "Sent to" alone is the
    // defect wearing the other direction.
    expect(screen.queryByText(/^Sent to\b/)).not.toBeInTheDocument();
  });

  // Every other timeline kind announces itself through a Badge. This one said
  // "email" in an icon alone, so a screen reader was told what happened
  // without being told what kind of thing it was.
  it("announces that the row is an email", () => {
    wrap(
      <EmailEntry
        summary={READABLE}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    // Asked of the sr-only span itself: "Email" appearing SOMEWHERE on the row
    // would still pass after the announcement was removed.
    expect(
      screen.getByText("Email", { selector: ".sr-only" }),
    ).toBeInTheDocument();
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
    wrap(
      <EmailEntry
        summary={READABLE}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  // The defect this arc exists to remove: "Sent" was drawn from DIRECTION
  // alone. A message parked because the channel refused its files read exactly
  // like one the provider confirmed, and the rep was told their mail went.
  it("does not call a parked message sent", () => {
    wrap(
      <EmailEntry
        summary={PARKED}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    expect(screen.getByText("Not sent to Ana Sommer")).toBeInTheDocument();
    expect(screen.queryByText("Sent to Ana Sommer")).not.toBeInTheDocument();
    // And the part a rep can act on, in the words the park was recorded with.
    expect(
      screen.getByText("the channel refused the attachment"),
    ).toBeInTheDocument();
  });

  it("says a confirmed message went, and says nothing about trouble", () => {
    wrap(
      <EmailEntry
        summary={DELIVERED}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    expect(screen.getByText("Sent to Ana Sommer")).toBeInTheDocument();
    expect(
      screen.queryByText("the channel refused the attachment"),
    ).not.toBeInTheDocument();
  });

  // A bounce is a later fact about a send the provider DID accept, so the
  // delivery keeps `sent` in the database and says `bounced` on the wire. A
  // row that reads the status alone tells a rep the mail arrived.
  it("does not call a bounced message sent", () => {
    wrap(
      <EmailEntry
        summary={{
          ...DELIVERED,
          delivery: {
            state: "bounced",
            reason: "550 5.1.1 user unknown",
            files: [],
          },
        }}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    expect(screen.getByText("Did not reach Ana Sommer")).toBeInTheDocument();
    expect(screen.getByText("550 5.1.1 user unknown")).toBeInTheDocument();
  });

  // A message somebody LOGGED rather than sent through the product has no
  // delivery at all. That is the rep's own claim that it went, and the row
  // keeps saying so.
  it("still says sent for a message with no delivery of its own", () => {
    wrap(
      <EmailEntry
        summary={{ ...DELIVERED, delivery: undefined }}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    expect(screen.getByText("Sent to Ana Sommer")).toBeInTheDocument();
  });

  // The chip counts what the message CARRIED, which the snapshot records and
  // the live list forgets: archiving the document afterwards drops
  // attachment_count to zero and must not rewrite what already went out.
  it("counts the files a message was staged with, not the ones still filed", () => {
    wrap(
      <EmailEntry
        summary={{
          ...DELIVERED,
          attachment_count: 0,
          delivery: {
            state: "sent",
            files: [
              { filename: "contract.pdf" },
              { filename: "annex.pdf" },
              { filename: "terms.pdf" },
            ],
          },
        }}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(
      screen.getByTitle("contract.pdf, annex.pdf, terms.pdf"),
    ).toBeVisible();
  });

  // A withheld row says nothing about delivery either. "This message you may
  // not read was parked because the recipient blocked us" is the content the
  // row just refused, in smaller print.
  it("says nothing about delivery on a withheld row", () => {
    wrap(
      <EmailEntry
        summary={{ ...PARKED, display_status: "withheld" }}
        timestamp="1 Sep 09:12"
        whyNotOpenable="withheld"
      />,
    );
    expect(
      screen.queryByText("the channel refused the attachment"),
    ).not.toBeInTheDocument();
    expect(screen.queryByText(/Not sent/)).not.toBeInTheDocument();
    // And it does not claim the opposite either: the row keeps the direction
    // and loses the verb, because "Sent" beside a message whose delivery it
    // just refused to read would be a claim it cannot support.
    expect(screen.queryByText(/^Sent/)).not.toBeInTheDocument();
    expect(screen.getByText("Outgoing")).toBeInTheDocument();
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
    wrap(
      <EmailEntry
        summary={READABLE}
        timestamp="1 Sep 09:12"
        whyNotOpenable="noDetail"
      />,
    );
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
    // The sender's own words, and then their sign-off, SHOWN.
    //
    // This message ends "Viele Grüße / Ana" and has no quoted history under it
    // at all. The assertion here used to require the "Show quoted history"
    // control over exactly that body — the test encoded the defect, so the one
    // reader who could have caught it agreed with it instead. A sign-off is the
    // sender still speaking; only an older message underneath gets folded.
    await screen.findByText("Können wir Dienstag sprechen?");
    expect(screen.queryByText("Show quoted history")).not.toBeInTheDocument();
    expect(screen.getByText(/Viele Grüße/)).toBeInTheDocument();
    expect(screen.getByText(/Ana Sommer/)).toBeInTheDocument();
  });

  // WHEN it was sent, in the header with the other envelope facts. It used to
  // sit under the message, below the attachments, so on anything longer than a
  // screen the reader scrolled past the whole body to learn the date.
  it("says when the message was sent, beside who it was sent to", async () => {
    vi.stubGlobal("fetch", jsonOnce(PRESENTATION));
    wrap(
      <EmailDetail
        activityId={READABLE.activity_id}
        onClose={() => {}}
        formatWhen={() => "1 Sep 09:12"}
      />,
    );

    const when = await screen.findByText("1 Sep 09:12");
    // In the envelope block, not trailing the body: the assertion is WHERE, so
    // finding the text anywhere on the page would pass over the bug.
    expect(when.closest(".emaildetail__parties")).not.toBeNull();
  });

  // A party the response could not name is DROPPED, not joined in as an empty
  // string. It used to render as a bare comma in the middle of the To line —
  // `display_name ?? address` keeps an empty string, because ?? only catches
  // null — which reads as a recipient whose name we lost.
  it("leaves out a party it cannot name, rather than a gap in the line", async () => {
    vi.stubGlobal(
      "fetch",
      jsonOnce({
        ...PRESENTATION,
        to: [
          { address: "", display_name: "" },
          { address: "andreas@buyer.test", display_name: null },
        ],
      }),
    );
    wrap(
      <EmailDetail
        activityId={READABLE.activity_id}
        onClose={() => {}}
        formatWhen={() => "1 Sep 09:12"}
      />,
    );

    // The line is read WITHOUT its label, because the gap the bug leaves is
    // between the label and the first name — an assertion over the whole
    // element's text has the word "To" in front of the comma and never sees it.
    const line = await screen.findByText(/andreas@buyer.test/);
    const label = line.querySelector(".emaildetail__partyLabel");
    const names = (line.textContent ?? "").slice(
      (label?.textContent ?? "").length,
    );
    expect(names.trim()).toBe("andreas@buyer.test");
  });

  it("folds an older message under this one, and says that is what it is", async () => {
    // The other side of the pair above. Without this case, deleting the fold
    // entirely would pass — a drawer that never folds anything satisfies "no
    // control over a sign-off" perfectly, and the quoted reply this product
    // does have to hide would be printed in full.
    vi.stubGlobal(
      "fetch",
      jsonOnce({
        ...PRESENTATION,
        body: "Ja, gerne.\n\nAm 1. September schrieb Ana:\n> Passt Dienstag?",
      }),
    );
    wrap(
      <EmailDetail
        activityId={READABLE.activity_id}
        onClose={() => {}}
        formatWhen={() => "1 Sep 09:12"}
      />,
    );
    await screen.findByText("Ja, gerne.");
    expect(screen.getByText("Show quoted history")).toBeInTheDocument();
    expect(screen.getByText(/Passt Dienstag\?/)).toBeInTheDocument();
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

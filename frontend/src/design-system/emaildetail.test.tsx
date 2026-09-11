// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// What the email drawer shows about a message beyond its words: the files that
// came with it, who it was with, what it is filed against, and the verb that
// answers it.
//
// Each of these is something the server had been sending all along and the
// drawer drew none of: a rep could see that a contract was mentioned and had
// no way to open it, could read a name and had no way to reach the contact,
// and could read a message and had no way to reply to it without going back
// for the row they opened it from. These are the claims that replaced that.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { EmailDetail } from "./emaildetail";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ACTIVITY = "01a05500-0000-7000-8000-0000000000a1";

function presentation(over: Record<string, unknown> = {}) {
  return {
    id: ACTIVITY,
    lifecycle: "delivered",
    occurred_at: "2026-09-01T09:15:00Z",
    summary: {
      activity_id: ACTIVITY,
      occurred_at: "2026-09-01T09:15:00Z",
      version: 3,
      subject: "The signed contract",
      preview: "Attached, as agreed.",
      display_status: "team",
      move: "none",
      attachment_count: 2,
    },
    body: "Attached, as agreed.",
    thread_key: "t1",
    from: [{ address: "dana@acme.test", display_name: "Dana Buyer" }],
    to: [],
    cc: [],
    bcc: [],
    bcc_withheld: false,
    attachments: [
      {
        id: "01a05500-0000-7000-8000-0000000000f1",
        filename: "contract.pdf",
        byte_size: 248000,
        content_type: "application/pdf",
      },
      {
        id: "01a05500-0000-7000-8000-0000000000f2",
        filename: "annex.pdf",
        byte_size: null,
        content_type: "application/pdf",
      },
    ],
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
    can_reply: true,
    can_relink: false,
    version: 3,
    ...over,
  };
}

function stubRead(body: unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(body), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
}

function draw(node: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function open() {
  return (
    <EmailDetail
      activityId={ACTIVITY}
      onClose={() => {}}
      formatWhen={(iso) => iso}
    />
  );
}

const ANA = "01a05500-0000-7000-8000-0000000000c1";
const BRANDT = "01a05500-0000-7000-8000-0000000000o1";

describe("the email drawer's attachments", () => {
  it("names each file and downloads it from the attachment endpoint", async () => {
    stubRead(presentation());
    draw(open());

    await waitFor(() => expect(screen.getByText("contract.pdf")).toBeTruthy());
    expect(screen.getByText("2 attachments")).toBeTruthy();

    // The NAME is the download — the pattern the contact and account file
    // lists already use, rather than a second action word at the end of a row.
    const link = screen.getByText("contract.pdf").closest("a");
    expect(link).not.toBeNull();
    expect(link?.getAttribute("href")).toBe(
      "/v1/attachments/01a05500-0000-7000-8000-0000000000f1",
    );
    expect(link?.getAttribute("download")).toBe("contract.pdf");

    // formatBytes' own scale: decimal units, so 248000 bytes is 248 kB. The
    // second file's size was never recorded and is absent rather than drawn as
    // zero bytes, which would be a claim about the file.
    expect(screen.getByText("248 kB")).toBeTruthy();
    expect(screen.queryByText(/0 byte/)).toBeNull();
  });

  it("draws no region for a message that carried nothing", async () => {
    stubRead(
      presentation({
        attachments: [],
        summary: {
          ...presentation().summary,
          attachment_count: 0,
        },
      }),
    );
    draw(open());

    await waitFor(() =>
      expect(screen.getByText("Attached, as agreed.")).toBeTruthy(),
    );
    // A heading over nothing says the message had files and they are missing,
    // which is a different claim from having had none.
    expect(screen.queryByText(/attachment/i)).toBeNull();
  });

  // The count is content. A reader outside the audience is told neither what
  // the files are nor that any arrived — knowing a contract was exchanged is
  // knowing something about the message.
  it("tells a reader outside the audience nothing about the files", async () => {
    // A HOSTILE fixture: withheld access alongside a full file list, which the
    // server does not send. That is the point — the drawer's own check is what
    // must refuse it, and a fixture that arrived already stripped would pass
    // with no check at all.
    stubRead(
      presentation({
        access: {
          ...presentation().access,
          content_state: "withheld",
          display_status: "withheld",
        },
      }),
    );
    draw(open());

    // Waits for the READ to land, not for the Modal: the dialog renders before
    // the fetch resolves, so a wait on it asserts against a loading state and
    // would pass however badly the loaded one behaved.
    await waitFor(() =>
      expect(screen.getByText("Not shared with you")).toBeTruthy(),
    );
    // Asked of document.body, NOT of render()'s container: Modal renders into
    // a PORTAL, so the container is empty and every assertion made against it
    // passes whatever the drawer draws. That mistake hid this very mutation.
    expect(document.body.textContent).not.toContain("contract.pdf");
    expect(document.body.textContent).not.toContain("annex.pdf");
    expect(document.body.querySelector(".emaildetail__files")).toBeNull();
    // Fails loudly if the fixture ever stops carrying files: this test's whole
    // claim is that a PRESENT list is refused, and a fixture that quietly lost
    // its attachments would assert nothing.
    if (presentation().attachments.length === 0) {
      throw new Error(
        "the hostile fixture carries no files; the claim is vacuous",
      );
    }
  });
});

// Who a message was with, as people a reader can go and look at.
//
// The header printed names as text, so a rep who wanted the contact behind an
// address had to close the message, remember the name and search for it. The
// address is already resolved on the server — `person_id` is set only for a
// contact this caller may see — and the header simply threw that away.
describe("the drawer's participants", () => {
  it("links a party the server resolved to a contact", async () => {
    stubRead(
      presentation({
        from: [
          {
            address: "ana@brandt.example",
            display_name: "Ana Sommer",
            person_id: ANA,
          },
        ],
      }),
    );
    draw(open());

    const contact = await screen.findByText("Ana Sommer");
    expect(contact.getAttribute("href")).toBe(`#/contacts/${ANA}`);
    // BESIDE the message, not over it: the reader is part-way through a mail
    // in a drawer over the record they were working on, and following the
    // contact in this tab would close both to reach a page they could have
    // opened from behind it.
    expect(contact.getAttribute("target")).toBe("_blank");
    expect(contact.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("keeps an unresolved address as text rather than a dead link", async () => {
    // An address the server could not put a contact to. A link here would go
    // to a record that does not exist, or to one this reader may not see —
    // which is the same page either way, and neither is the contact.
    stubRead(
      presentation({
        from: [{ address: "stranger@elsewhere.example", display_name: null }],
      }),
    );
    draw(open());

    const party = await screen.findByText(/stranger@elsewhere.example/);
    expect(party.closest("a")).toBeNull();
  });
});

/**
 * A host of the shape the real one has: a render prop returns an ELEMENT, and
 * the component inside it decides whether to draw anything.
 *
 * This is the whole point of the case below. `OpenEmailDrawer` returns
 * `<EmailRecordLinks …/>`, which is a truthy object whatever that component
 * goes on to render — so a drawer that decided by testing the returned node
 * drew its label over every message filed against nothing, and a test that
 * handed back a literal `null` reported it green.
 */
function NamesNothing() {
  return null;
}

// What the message is filed against, named by the host and labelled here.
describe("the drawer's filing line", () => {
  it("labels the records the host could name", async () => {
    stubRead(
      presentation({
        links: [{ entity_type: "company", entity_id: BRANDT }],
      }),
    );
    draw(
      <EmailDetail
        activityId={ACTIVITY}
        onClose={() => {}}
        formatWhen={(iso) => iso}
        renderRecords={() => <span>Brandt Automotive</span>}
      />,
    );

    await waitFor(() => expect(screen.getByText("Filed under")).toBeTruthy());
    expect(screen.getByText("Brandt Automotive")).toBeTruthy();
  });

  it("draws no label for a message filed against nothing", async () => {
    // A label over an empty value says the message is filed somewhere and the
    // drawer has lost track of where, which is a different claim from a
    // message filed against nothing.
    //
    // The host here returns an element, as the real one does. Swap the
    // drawer's check back to testing that node and this case fails while
    // everything else stays green — which is what makes it worth having.
    stubRead(presentation({ links: [] }));
    draw(
      <EmailDetail
        activityId={ACTIVITY}
        onClose={() => {}}
        formatWhen={(iso) => iso}
        renderRecords={() => <NamesNothing />}
      />,
    );

    await waitFor(() =>
      expect(screen.getByText("Attached, as agreed.")).toBeTruthy(),
    );
    expect(screen.queryByText("Filed under")).toBeNull();
  });
});

// The verb that answers the message, in the header where a reader meets it
// before the body rather than after one.
describe("the drawer's reply", () => {
  it("draws the host's verb beside the way out", async () => {
    stubRead(presentation());
    draw(
      <EmailDetail
        activityId={ACTIVITY}
        onClose={() => {}}
        formatWhen={(iso) => iso}
        // The catalog's own control, which is what the real host renders: a
        // native button here would place a shape in the header that no
        // production caller can produce, and the claim is about where the
        // header puts the verb it is given.
        renderReply={() => <Button small>Reply</Button>}
      />,
    );

    const verb = await screen.findByRole("button", { name: "Reply" });
    // In the header's action cluster, which is what puts it within reach of a
    // message that runs past a screen.
    expect(verb.closest(".emaildetail__actions")).not.toBeNull();
  });

  it("asks for no verb before the message has arrived", () => {
    // The host decides from the presentation — `can_reply` is the server's
    // answer — so there is nothing to ask until the read lands.
    stubRead(presentation());
    const renderReply = vi.fn(() => null);
    draw(
      <EmailDetail
        activityId={ACTIVITY}
        onClose={() => {}}
        formatWhen={(iso) => iso}
        renderReply={renderReply}
      />,
    );

    expect(renderReply).not.toHaveBeenCalled();
  });
});

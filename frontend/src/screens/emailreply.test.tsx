// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// Whether the drawer offers to answer the message it is showing, and what such
// a reply would be filed under.
//
// The verb is offered from the server's own `can_reply` and from nothing else,
// which is what keeps a drawer that is refusing to show a message from
// offering to answer it in the same header.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { EmailReplyAction, replyAnchor } from "./emailreply";

type EmailPresentation = components["schemas"]["EmailPresentation"];
type ActivityLink = components["schemas"]["ActivityLink"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ACTIVITY = "01a05500-0000-7000-8000-0000000000a1";
const ANA = "01a05500-0000-7000-8000-0000000000c1";
const BRANDT = "01a05500-0000-7000-8000-0000000000o1";
const DEAL = "01a05500-0000-7000-8000-0000000000d1";

/**
 * One presentation, typed as the generated contract shape rather than a
 * literal: a fixture that drifts from what the server sends fails the build
 * instead of proving the component handles a message nobody will ever receive.
 */
function presentation(over: Partial<EmailPresentation>): EmailPresentation {
  return {
    id: ACTIVITY,
    lifecycle: "delivered",
    occurred_at: "2026-09-01T09:15:00Z",
    summary: {
      activity_id: ACTIVITY,
      occurred_at: "2026-09-01T09:15:00Z",
      version: 3,
      subject: "Angebot Q4",
      preview: "Können wir Dienstag kurz sprechen?",
      display_status: "team",
      move: "needs_reply",
      attachment_count: 0,
    },
    body: "Können wir Dienstag kurz sprechen?",
    thread_key: "t1",
    from: [],
    to: [],
    cc: [],
    bcc: [],
    bcc_withheld: false,
    attachments: [],
    links: [{ entity_type: "person", entity_id: ANA }],
    thread: { members: [], next_cursor: null },
    access: {
      content_state: "available",
      display_status: "team",
      audience: "workspace",
      can_change: false,
      change_mode: "none",
    },
    can_reply: true,
    can_relink: false,
    version: 3,
    ...over,
  };
}

/**
 * Draw the action with `fetch` stubbed to reject, so a request this component
 * should not make fails loudly rather than hanging: the verb is decided from
 * the presentation the drawer already read, and a read of its own here would
 * be a second answer to a question the server has already given.
 */
function draw(node: ReactNode) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      throw new Error(`unexpected request: ${String(input)}`);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

/**
 * The verb, or null when the drawer offers none.
 *
 * By ROLE and label rather than by test id: what a reader can press is what
 * these cases are about, and a query that found a hidden node or a plain span
 * would pass on a verb nobody can reach.
 */
function reply() {
  return screen.queryByRole("button", { name: "Reply" });
}

describe("answering the message the drawer is showing", () => {
  it("offers the verb for a message this reader may answer", () => {
    draw(<EmailReplyAction presentation={presentation({})} />);

    expect(reply()).not.toBeNull();
  });

  it("offers nothing for a message the server says cannot be answered", () => {
    // What a withheld presentation carries. Offering to answer a message in
    // the same header that refuses to show it claims access to words this
    // reader was just told are not theirs.
    draw(
      <EmailReplyAction
        presentation={presentation({
          can_reply: false,
          access: {
            content_state: "withheld",
            display_status: "withheld",
            can_change: false,
            change_mode: "none",
          },
        })}
      />,
    );

    expect(reply()).toBeNull();
  });

  it("offers nothing for a message filed against no record", () => {
    // The composer files a send under the record it was anchored to. A reply
    // anchored to nothing is a message the product sends and can then show on
    // no timeline, so the verb is withheld rather than the send being made.
    draw(<EmailReplyAction presentation={presentation({ links: [] })} />);

    expect(reply()).toBeNull();
  });
});

describe("what a reply from the drawer is filed under", () => {
  // The drawer is opened from surfaces that are not a record page — a search
  // hit, the worklist, a brief's citation — so the anchor comes from the
  // message's own filing rather than from where it was opened.
  const person: ActivityLink = { entity_type: "person", entity_id: ANA };
  const organization: ActivityLink = {
    entity_type: "organization",
    entity_id: BRANDT,
  };
  const deal: ActivityLink = { entity_type: "deal", entity_id: DEAL };

  it("prefers the contact, whose timeline reads as the conversation", () => {
    // Order in the list must not decide it: the server sends links in its own
    // order, and a precedence that came out of that would move with it.
    expect(replyAnchor([organization, deal, person])).toEqual(person);
  });

  it("takes the work it is about when no contact is named", () => {
    expect(replyAnchor([organization, deal])).toEqual(deal);
  });

  it("takes the account last, being the broadest thing it could be", () => {
    expect(replyAnchor([organization])).toEqual(organization);
  });

  it("answers nothing for a message filed against nothing", () => {
    expect(replyAnchor([])).toBeNull();
  });
});

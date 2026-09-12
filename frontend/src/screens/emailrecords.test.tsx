// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// The records a message is filed against, named on the drawer's envelope block.
//
// Two claims: the name is a way to the record, and the way opens BESIDE the
// message rather than over it. A reader who follows a contact from a mail they
// are part-way through reading and loses the mail has been punished for asking
// who they were reading about.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { EmailRecordLinks } from "./emailrecords";

type EmailPresentation = components["schemas"]["EmailPresentation"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ACTIVITY = "01a05500-0000-7000-8000-0000000000a1";
const ANA = "01a05500-0000-7000-8000-0000000000c1";
const BRANDT = "01a05500-0000-7000-8000-0000000000o1";

/**
 * One presentation carrying the filing the case is about.
 *
 * Typed as the generated contract shape rather than a literal, so a fixture
 * that drifts from what the server sends fails the build instead of proving
 * the component handles a message nobody will ever receive.
 */
function presentation(links: EmailPresentation["links"]): EmailPresentation {
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
      move: "none",
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
    links,
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
  };
}

/**
 * Draw the list with the record reads `EntityRef` makes answered by path.
 *
 * Anything else throws, so a request this component should not make fails
 * loudly rather than resolving into a name the case then asserts on.
 */
function draw(node: ReactNode) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      // The client hands `fetch` a Request, whose stringification is
      // "[object Request]" — a stub reading that matches no route and answers
      // every read with a failure, which draws exactly like a refused one.
      const url = input instanceof Request ? input.url : String(input);
      const path = new URL(url, "https://test.local").pathname;
      const named: Record<string, unknown> = {
        [`/v1/contacts/${ANA}`]: { id: ANA, full_name: "Ana Sommer" },
        [`/v1/companies/${BRANDT}`]: {
          id: BRANDT,
          display_name: "Brandt Automotive",
        },
      };
      const body = named[path];
      if (!body) {
        throw new Error(`unexpected request: ${path}`);
      }
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
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

describe("the records a message is filed against", () => {
  it("names a contact and an account, each a way to its record", async () => {
    draw(
      <EmailRecordLinks
        presentation={presentation([
          { entity_type: "contact", entity_id: ANA },
          { entity_type: "company", entity_id: BRANDT },
        ])}
      />,
    );

    const contact = await screen.findByText("Ana Sommer");
    expect(contact.getAttribute("href")).toBe(`#/contacts/${ANA}`);
    const account = await screen.findByText("Brandt Automotive");
    expect(account.getAttribute("href")).toBe(`#/companies/${BRANDT}`);
  });

  it("opens beside the message rather than over it", async () => {
    draw(
      <EmailRecordLinks
        presentation={presentation([
          { entity_type: "contact", entity_id: ANA },
        ])}
      />,
    );

    const contact = await screen.findByText("Ana Sommer");
    expect(contact.getAttribute("target")).toBe("_blank");
    // `noopener` alongside it, and not left to an engine's default: a blank
    // target without it hands the opened page a live handle back into this one.
    expect(contact.getAttribute("rel")).toBe("noopener noreferrer");
  });

  it("draws nothing for a message filed against nothing", async () => {
    const { container } = draw(
      <EmailRecordLinks presentation={presentation([])} />,
    );

    // Nullish rather than empty: the drawer reads the answer to decide whether
    // to draw the label at all, and an empty element is truthy.
    await waitFor(() => expect(container.innerHTML).toBe(""));
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// What the email drawer shows about the files that came with a message.
//
// The server had been sending them all along and the drawer drew none of them:
// a rep reading a message in the product could see that a contract was
// mentioned and had no way to open it. These are the claims that replaced that.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
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

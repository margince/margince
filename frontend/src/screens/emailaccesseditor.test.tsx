// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */

// What the drawer says about who reads a message, and what a reader may change.
//
// The claim that matters most here is the one the timeline row cannot make: the
// editor opens with the set that is ALREADY on the message. The row's dialog
// starts blank, because a list row carries no access block — so a reader
// removing one person from a set of five had to re-tick the other four from
// memory. These tests hold the difference.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { EmailAccessEditor } from "./emailaccesseditor";

type EmailPresentation = components["schemas"]["EmailPresentation"];
type EmailAccess = components["schemas"]["EmailAccess"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const ACTIVITY = "01a05500-0000-7000-8000-0000000000a1";
const ANA = "01a05500-0000-7000-8000-0000000000c1";
const BAO = "01a05500-0000-7000-8000-0000000000c2";

const ROSTER = [
  { id: ANA, display_name: "Ana Fischer", email: "ana@demo.test" },
  { id: BAO, display_name: "Bao Nguyen", email: "bao@demo.test" },
];

/**
 * One presentation, with the access block the test is about.
 *
 * Typed as the generated `EmailPresentation` rather than an object literal, so
 * a fixture that drifts from the contract fails the build instead of proving
 * the component handles a shape the server never sends.
 */
function presentation(access: Partial<EmailAccess>): EmailPresentation {
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
      attachment_count: 0,
    },
    body: "Attached, as agreed.",
    thread_key: "t1",
    from: [],
    to: [],
    cc: [],
    bcc: [],
    bcc_withheld: false,
    attachments: [],
    links: [],
    thread: { members: [], next_cursor: null },
    can_reply: true,
    can_relink: false,
    version: 3,
    access: {
      content_state: "available",
      display_status: "team",
      audience: "workspace",
      can_change: false,
      change_mode: "none",
      ...access,
    },
  };
}

/**
 * Draw the editor with the roster already read.
 *
 * The seats are seeded into the cache entry `useRoster` reads rather than
 * stubbed at the wire: the roster is a PAGINATED WALK, so a fetch stub would
 * have to reproduce its cursor protocol, and a test that supplies its own
 * version of production proves nothing about production. This seeds the walk's
 * result and lets the real hook read it.
 *
 * `fetch` is stubbed to reject so a request this component should not make
 * fails loudly instead of hanging.
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
  client.setQueryData(["users"], { entries: ROSTER, partial: false });
  client.setQueryData(["teams"], { entries: [], partial: false });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{node}</LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("what the drawer says about who reads a message", () => {
  it("states the verdict even when the reader may change nothing", () => {
    draw(
      <EmailAccessEditor
        presentation={presentation({
          display_status: "participants",
          audience: "participants",
          can_change: false,
          change_mode: "none",
        })}
      />,
    );

    // Who reads a message is a fact about it, like its date. A reader without
    // standing to widen it is still told what it is.
    expect(screen.getByText("Participants")).toBeTruthy();
    expect(
      screen.getByText("Only the people on this message can read it."),
    ).toBeTruthy();
    // And offered nothing to press, because the server said so.
    expect(screen.queryByRole("button", { name: "Visibility" })).toBeNull();
  });

  it("offers no control to a reader the server refused, whatever mode it named", () => {
    draw(
      <EmailAccessEditor
        presentation={presentation({
          display_status: "selected",
          audience: "selected",
          // `can_change` alone decides. A response that refuses this caller
          // while still naming the write it would otherwise take must draw
          // nothing — the two are separate fields and only one is the
          // permission. Gating on `change_mode` alone passes every other case
          // in this file, which is how a missing check stays invisible.
          can_change: false,
          change_mode: "message_audience",
        })}
      />,
    );

    expect(screen.queryByRole("button", { name: "Visibility" })).toBeNull();
    expect(screen.queryByRole("button", { name: /Share/ })).toBeNull();
  });

  it("names the people a limited message is limited to", async () => {
    draw(
      <EmailAccessEditor
        presentation={presentation({
          display_status: "selected",
          audience: "selected",
          can_change: true,
          change_mode: "message_audience",
          selected_members: [
            { subject_type: "user", subject_id: ANA },
            { subject_type: "user", subject_id: BAO },
          ],
        })}
      />,
    );

    // The write's vocabulary is uuids; a reader checking who can see their mail
    // needs names.
    await waitFor(() => expect(screen.getByText("Ana Fischer")).toBeTruthy());
    expect(screen.getByText("Bao Nguyen")).toBeTruthy();
    expect(screen.queryByText(ANA)).toBeNull();
  });

  it("draws no member list when the reader may not enumerate the set", () => {
    draw(
      <EmailAccessEditor
        presentation={presentation({
          display_status: "selected",
          audience: "selected",
          can_change: false,
          change_mode: "none",
          // Absent, which is what the server sends a reader who may read the
          // content but not change who else may.
        })}
      />,
    );

    // An absent list is NOT an empty audience. Printing "nobody" would be a
    // false statement about a message limited to people this reader cannot
    // name, so nothing is drawn for it.
    expect(
      screen.getByText("Only the people named below can read this."),
    ).toBeTruthy();
    expect(screen.queryByRole("list")).toBeNull();
  });

  it("opens the editor with the standing set already ticked", async () => {
    const user = userEvent.setup();
    draw(
      <EmailAccessEditor
        presentation={presentation({
          display_status: "selected",
          audience: "selected",
          can_change: true,
          change_mode: "message_audience",
          selected_members: [{ subject_type: "user", subject_id: ANA }],
        })}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Visibility" }));

    // THE point of this editor. The timeline row's dialog opens blank, so a
    // reader could only replace a set, never edit one. Here Ana is already
    // ticked because the drawer had the access block in hand.
    const ana = await screen.findByRole("checkbox", { name: /Ana Fischer/ });
    expect((ana as HTMLInputElement).checked).toBe(true);
    const bao = screen.getByRole("checkbox", { name: /Bao Nguyen/ });
    expect((bao as HTMLInputElement).checked).toBe(false);
  });

  it("enables the confirm when only the member set moved", async () => {
    const user = userEvent.setup();
    draw(
      <EmailAccessEditor
        presentation={presentation({
          display_status: "selected",
          audience: "selected",
          can_change: true,
          change_mode: "message_audience",
          selected_members: [{ subject_type: "user", subject_id: ANA }],
        })}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Visibility" }));
    const confirm = await screen.findByRole("button", {
      name: "Save visibility",
    });

    // Nothing moved yet: the audience is the same and so is the set.
    expect((confirm as HTMLButtonElement).disabled).toBe(true);

    // Adding Bao changes the set but NOT the audience, which stays `selected`.
    // A confirm gated on the audience alone would sit disabled through exactly
    // the edit this surface exists for.
    await user.click(screen.getByRole("checkbox", { name: /Bao Nguyen/ }));
    expect((confirm as HTMLButtonElement).disabled).toBe(false);
  });

  it("refuses a selected audience with nobody in it", async () => {
    const user = userEvent.setup();
    draw(
      <EmailAccessEditor
        presentation={presentation({
          display_status: "selected",
          audience: "selected",
          can_change: true,
          change_mode: "message_audience",
          selected_members: [{ subject_type: "user", subject_id: ANA }],
        })}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Visibility" }));
    const confirm = await screen.findByRole("button", {
      name: "Save visibility",
    });

    // Unticking the only member leaves a limit nobody meant. Narrowing a
    // message to nobody is refused rather than written.
    await user.click(await screen.findByRole("checkbox", { name: /Ana/ }));
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
  });

  it("offers the thread control for a captured message, not the dialog", () => {
    draw(
      <EmailAccessEditor
        presentation={presentation({
          display_status: "participants",
          audience: "participants",
          can_change: true,
          change_mode: "thread_contribution",
        })}
      />,
    );

    // The server names which write it would accept. Nothing here re-derives it
    // from `captured_by`, which is the inference this editor replaces: a
    // captured message's audience is derived from every importing mailbox, so
    // what this reader changes is their own contribution to the thread.
    expect(screen.getByRole("button", { name: /Share/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Visibility" })).toBeNull();
  });
});

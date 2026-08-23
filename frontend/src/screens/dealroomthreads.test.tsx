/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ProblemError } from "./common";
import {
  type BoardDocument,
  type DealRoomThread,
  DocumentBoard,
  type ThreadVerbs,
} from "./dealroomthreads";

// The board is drawn once for BOTH sides of a Deal Room, and the buyer's half
// is served to somebody outside the organization entirely. What a refused
// write says here is therefore read by a party with no seat, no role and no
// business knowing the shape of the authority model that refused them.

afterEach(cleanup);

const DOCUMENT: BoardDocument = {
  id: "doc-1",
  groupKey: "contract",
  title: "Rahmenvertrag",
  meta: "rahmenvertrag.pdf",
};

function thread(over: Partial<DealRoomThread> = {}): DealRoomThread {
  return {
    id: "th-1",
    document_id: "doc-1",
    state: "open",
    required_change: false,
    comments: [
      {
        id: "c-1",
        body: "Which clause covers the notice period?",
        author: { name: "Buyer", side: "buyer" },
        created_at: "2026-08-01T09:00:00Z",
      },
    ],
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
    ...over,
  } as DealRoomThread;
}

function draw(verbs: Partial<ThreadVerbs> = {}, threads = [thread()]) {
  render(
    <LocaleProvider initial="en">
      <DocumentBoard
        title="Documents"
        sub="Everything shared with the buyer"
        groups={[{ key: "contract", label: "Contract" }]}
        documents={[DOCUMENT]}
        threads={threads}
        empty="No documents yet"
        verbs={{ mayRequireChange: false, ...verbs }}
      />
    </LocaleProvider>,
  );
}

describe("a refused reply", () => {
  // The server's own sentence, composed for a reader, reaches them untouched —
  // the opposite path from the permission refusal below, where the catalog
  // REPLACES a detail that was never composed for a reader.
  it("keeps the reader's draft and passes the server's own sentence through", async () => {
    const user = userEvent.setup();
    const reply = vi.fn(async () => {
      throw new ProblemError({
        code: "version_conflict",
        detail: "The thread moved on. Read it again before replying.",
      });
    });
    draw({ reply });

    await user.type(screen.getByLabelText(/reply/i), "Clause 7.");
    await user.click(screen.getByRole("button", { name: /^Reply$/ }));

    // The server's OWN sentence, composed for a reader, still reaches them.
    await waitFor(() => expect(screen.getByText(/moved on/)).toBeTruthy());
    // And the draft survives — a refused write must not also lose the words.
    expect(screen.getByLabelText(/reply/i)).toHaveValue("Clause 7.");
  });

  // The disclosure. `auth.Require` builds a refusal's detail from the RBAC
  // object and the verb, and this board renders to a buyer who is not in the
  // organization at all.
  it("never hands a refused party the RBAC object and verb", async () => {
    const user = userEvent.setup();
    const reply = vi.fn(async () => {
      throw new ProblemError({
        code: "permission_denied",
        detail: "dealroom.reply: permission denied",
      });
    });
    draw({ reply });

    await user.type(screen.getByLabelText(/reply/i), "Clause 7.");
    await user.click(screen.getByRole("button", { name: /^Reply$/ }));

    await waitFor(() =>
      expect(screen.getByText(/do not have permission/)).toBeTruthy(),
    );
    expect(screen.queryByText(/dealroom\.reply/)).toBeNull();
  });

  it("reports a failure nobody phrased for a reader as the shared line", async () => {
    const user = userEvent.setup();
    // A rejected fetch: wording nobody wrote for a user, and often our own
    // internals. It reads as the shared failure line rather than reaching the
    // screen as-is.
    const reply = vi.fn(async () => {
      throw new TypeError("Failed to fetch");
    });
    draw({ reply });

    await user.type(screen.getByLabelText(/reply/i), "Clause 7.");
    await user.click(screen.getByRole("button", { name: /^Reply$/ }));

    await waitFor(() =>
      expect(
        screen.getByText("The request failed. No cause reported."),
      ).toBeTruthy(),
    );
    expect(screen.queryByText(/Failed to fetch/)).toBeNull();
  });
});

describe("a refused new thread", () => {
  it("answers a permission refusal in the reader's words", async () => {
    const user = userEvent.setup();
    const open = vi.fn(async () => {
      throw new ProblemError({
        code: "permission_denied",
        detail: "dealroom.open_thread: permission denied",
      });
    });
    draw({ open });

    await user.click(
      screen.getByRole("button", { name: "Ask about this document" }),
    );
    // Scoped to THIS composer: the room-wide one below carries the same verb,
    // and a bare getByRole would be ambiguous rather than wrong.
    const field = screen.getByLabelText("Ask about this document");
    await user.type(field, "Is clause 7 negotiable?");
    const composer = field.closest(".thread-composer");
    if (composer === null) {
      throw new Error("the document composer did not render");
    }
    await user.click(
      within(composer as HTMLElement).getByRole("button", { name: "Post" }),
    );

    await waitFor(() =>
      expect(screen.getByText(/do not have permission/)).toBeTruthy(),
    );
    expect(screen.queryByText(/dealroom\.open_thread/)).toBeNull();
  });
});

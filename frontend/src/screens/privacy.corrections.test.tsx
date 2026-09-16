/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import type { RbacAction } from "../app/capability";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ConfirmSubmissionsPanel } from "./privacy.corrections";

// Deciding what a contact typed into the link we mailed them.
//
// The queue is the whole point: the rows have existed since the confirm page
// shipped and nothing listed them, so every correction anybody ever sent has
// been sitting unanswered. What this file holds is that the surface shows the
// comparison a decision needs, and that a decision travels as a variable rather
// than as whatever the render happened to be holding.

type ConfirmSubmission = components["schemas"]["ConfirmSubmission"];

const PAGE = { next_cursor: null, has_more: false };

// Typed, not asserted: a fixture cast into the contract type can drop a
// required field and still compile, and the test would pass after the wire
// shape moved under it.
function correction(
  id: string,
  field: string,
  value: string,
): ConfirmSubmission {
  return {
    id,
    contact_id: "c-1",
    kind: "correction",
    field,
    proposed_value: value,
    submitted_at: "2026-08-01T09:00:00Z",
    contact_name: "Anna Schmidt",
    current_value: "Schmitt",
  };
}

/** What a contact asking to be removed sends: no field, no value. */
function removal(id: string): ConfirmSubmission {
  return {
    id,
    contact_id: "c-2",
    kind: "removal",
    submitted_at: "2026-08-02T09:00:00Z",
    contact_name: "Bea Vogel",
  };
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

/** Records every resolve the screen posts, so a test can read the body. */
type Posted = { url: string; body: unknown };

function server(
  rows: readonly ConfirmSubmission[],
  posted: Posted[],
  // Typed against the contract's action vocabulary rather than inferred. An
  // inferred `string[]` compiles here and fails where meFixture reads it, so
  // the default has to name the type it is a default FOR.
  grant: readonly RbacAction[] = ["update"],
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      if (url.pathname.endsWith("/me")) {
        // The decision is a write to the CONTACT: a correction accepted changes
        // a field on it, and a rejection is the workspace's recorded answer
        // about somebody. So the grant the screen asks for is contact:update.
        return json(meFixture({ roles: ["admin"], allow: { contact: grant } }));
      }
      if (url.pathname.endsWith("/resolve")) {
        // The body rides the Request when the client builds one, and `init`
        // only when it does not — a stub reading `init` alone sees {} and the
        // assertion passes on nothing.
        const body = request
          ? await request.json()
          : JSON.parse(String(init?.body ?? "{}"));
        posted.push({ url: url.pathname, body });
        return json(rows[0]);
      }
      if (url.pathname.endsWith("/confirm-submissions")) {
        return json({ data: rows, page: PAGE });
      }
      return json({});
    }),
  );
}

/**
 * Two pages of the queue, served the way the route does: the second only to a
 * request carrying the cursor the first handed back.
 *
 * A stub that answered both pages to the same request would pass a screen that
 * never sent the cursor at all — which is exactly the shape of the defect, one
 * layer up.
 */
function pagedServer(first: ConfirmSubmission, second: ConfirmSubmission) {
  const asked: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      if (url.pathname.endsWith("/me")) {
        return json(
          meFixture({ roles: ["admin"], allow: { contact: ["update"] } }),
        );
      }
      if (url.pathname.endsWith("/confirm-submissions")) {
        const cursor = url.searchParams.get("cursor");
        asked.push(cursor ?? "");
        return cursor === "page-two"
          ? json({
              data: [second],
              page: { next_cursor: null, has_more: false },
            })
          : json({
              data: [first],
              page: { next_cursor: "page-two", has_more: true },
            });
      }
      return json({});
    }),
  );
  return asked;
}

describe("the queue of what contacts told us to change", () => {
  beforeEach(() => vi.unstubAllGlobals());
  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows both halves of the comparison, and whose record it is", async () => {
    server([correction("s-1", "full_name", "Schmidt")], []);
    render(<ConfirmSubmissionsPanel />);

    // The proposal alone is not a decision: "she says Schmidt, we hold
    // Schmitt" is, and a queue spanning every contact needs the name too —
    // two contacts proposing the same value are otherwise indistinguishable
    // while accepting either changes a different record.
    expect(await screen.findByText("Anna Schmidt")).toBeInTheDocument();
    expect(screen.getByText(/Schmitt/)).toBeInTheDocument();
    expect(screen.getByText(/Schmidt →|→ Schmidt/)).toBeInTheDocument();
    expect(screen.getByText("full_name")).toBeInTheDocument();
  });

  it("says a read failed rather than reporting an empty queue", async () => {
    // Coercing an undefined answer to [] told the reviewer nothing was waiting
    // when the read had in fact failed, which is the one wrong thing a work
    // queue can say.
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const request = input instanceof Request ? input : null;
        const url = new URL(
          request ? request.url : String(input),
          "https://test.local",
        );
        if (url.pathname.endsWith("/me")) {
          return json(
            meFixture({ roles: ["admin"], allow: { contact: ["update"] } }),
          );
        }
        return json({ title: "the queue is unreachable" }, 500);
      }),
    );
    render(<ConfirmSubmissionsPanel />);

    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(
      screen.queryByText(en["privacy.correctionsEmpty"]),
    ).not.toBeInTheDocument();
  });

  it("offers to acknowledge a removal rather than to update anything", async () => {
    // Accepting a removal records that somebody read it. What the contact
    // asked for is a rights case, and a button reading "accept and update"
    // would claim this removed them.
    server([removal("s-9")], []);
    const user = userEvent.setup();
    render(<ConfirmSubmissionsPanel />);

    await user.click(
      await screen.findByRole("button", {
        name: en["privacy.correctionDecide"],
      }),
    );
    expect(
      screen.getByRole("button", { name: en["privacy.correctionAcknowledge"] }),
    ).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["privacy.correctionAccept"] }),
    ).not.toBeInTheDocument();
  });

  it("says so plainly when nothing is waiting", async () => {
    server([], []);
    render(<ConfirmSubmissionsPanel />);

    expect(
      await screen.findByText(en["privacy.correctionsEmpty"]),
    ).toBeInTheDocument();
  });

  it("sends the decision and the note it was typed with", async () => {
    const posted: Posted[] = [];
    server([correction("s-1", "full_name", "Schmidt")], posted);
    const user = userEvent.setup();
    render(<ConfirmSubmissionsPanel />);

    await user.click(
      await screen.findByRole("button", {
        name: en["privacy.correctionDecide"],
      }),
    );
    await user.type(screen.getByRole("textbox"), "Passport says otherwise.");
    await user.click(
      screen.getByRole("button", { name: en["privacy.correctionReject"] }),
    );

    expect(posted).toHaveLength(1);
    expect(posted[0].url).toContain("/confirm-submissions/s-1/resolve");
    expect(posted[0].body).toEqual({
      resolution: "rejected",
      note: "Passport says otherwise.",
    });
  });

  it("offers no decision to a seat that cannot edit the contact", async () => {
    // The control is withheld rather than offered and refused: a governance
    // surface must not promise an authority it does not carry.
    server([correction("s-1", "full_name", "Schmidt")], [], ["read"]);
    render(<ConfirmSubmissionsPanel />);

    // The row still RENDERS: a reader who may open the contact may see what
    // that contact asked for. What they are not offered is the decision.
    expect(await screen.findByText("Anna Schmidt")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: en["privacy.correctionDecide"] }),
    ).not.toBeInTheDocument();
  });

  // THE TAIL IS REACHABLE. The queue was bounded and silent about it: a
  // reviewer who worked to the bottom of the list had seen the oldest page and
  // none of what arrived after, with nothing on screen saying so. What a
  // reviewer never hears about is a correction a data subject asked for.
  it("fetches the next page when the reviewer asks for more", async () => {
    const asked = pagedServer(
      correction("s-1", "full_name", "Schmidt"),
      correction("s-2", "title", "Head of Ops"),
    );
    render(<ConfirmSubmissionsPanel />);

    expect(await screen.findByText("full_name")).toBeInTheDocument();
    expect(screen.queryByText("title")).toBeNull();

    await userEvent.click(screen.getByRole("button", { name: /more/i }));

    expect(await screen.findByText("title")).toBeInTheDocument();
    // Still on screen: a page that replaced the last one would lose the rows
    // the reviewer is working through.
    expect(screen.getByText("full_name")).toBeInTheDocument();
    expect(asked).toEqual(["", "page-two"]);
  });

  // And a queue that fits on one page offers nothing to press, or the control
  // tells every reviewer there is more when there never is.
  it("offers no more to fetch when the queue ends", async () => {
    server([correction("s-1", "full_name", "Schmidt")], []);
    render(<ConfirmSubmissionsPanel />);

    expect(await screen.findByText("full_name")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /more/i })).toBeNull();
  });
});

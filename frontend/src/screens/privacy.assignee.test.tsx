/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { PrivacyInboxCard } from "./privacy";

// Who is answering an erasure request, on the one field that says so.
//
// A `Select` whose value matches no option paints its placeholder, and with no
// placeholder a non-breaking space in placeholder styling — which is exactly
// what the disabled unassigned em dash looks like. So a request that IS assigned
// read as unassigned whenever the assignee could not be named, and the next
// officer to open it reassigned statutory work off the colleague doing it.
//
// The roster walk still builds the picker's OFFERED list, but the assignee's
// own name is a separate by-id read (`GET /users/names`) with its own
// pending/settled/failed states — it owes nothing to the walk's budget, which
// is what lets it name a holder even while the offered list is still partial.
// Naming can still come up short in its own right: the read has not answered
// yet, it settles on nothing for this id (archived, or never held), or it
// names a colleague the picker still withholds — an agent seat cannot hold
// the row scope a subject request needs.

type DataSubjectRequest = components["schemas"]["DataSubjectRequest"];
type User = components["schemas"]["User"];

const PAGE = { next_cursor: null, has_more: false };

// Typed, not asserted: a fixture cast into the contract type can drop a required
// field and still compile, and the test would go on passing after the wire shape
// moved under it.
function dsrAssignedTo(assigneeId: string): DataSubjectRequest {
  return {
    id: "d1",
    kind: "erasure",
    subject_ref: "8f3a-contact-uuid",
    status: "open",
    assignee_id: assigneeId,
    due_at: "2026-08-01T00:00:00Z",
    created_at: "2026-07-01T00:00:00Z",
  };
}

function member(id: string, displayName: string, isAgent = false): User {
  return {
    id,
    email: `${id}@acme.test`,
    display_name: displayName,
    timezone: "Europe/Berlin",
    status: "active",
    is_agent: isAgent,
  };
}

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// What `/users` answers, cursor by cursor. The roster is a keyset walk, so a
// test that wants a truncated or a still-running one has to answer the way the
// server does rather than hand the hook a finished list.
type RosterServer = (cursor: string | null) => Promise<Response>;

/** One page, and no cursor to continue with: the whole roster. */
function oneRosterPage(members: readonly User[]): RosterServer {
  return async () => json({ data: members, page: PAGE });
}

/** A server that never stops offering another page, so the walk's bound bites. */
function endlessRoster(): RosterServer {
  return async (cursor) => {
    const index = cursor ? Number(cursor) : 0;
    return json({
      data: [member(`u-${index}`, `Member ${index}`)],
      page: { next_cursor: String(index + 1), has_more: true },
    });
  };
}

// What GET /users/names answers for the assignee's own id, the by-id read
// unofferedAssignee resolves through, apart from the roster walk above. A
// read that never answers proves the field's own loading state; an id the map
// omits is a settled absence, same as an archived seat.
type NameServer = () => Promise<Response>;

function namesOf(byId: Readonly<Record<string, string>>): NameServer {
  return async () =>
    json({
      data: Object.entries(byId).map(
        ([id, display_name]): components["schemas"]["SeatName"] => ({
          id,
          display_name,
        }),
      ),
    });
}

function unansweredNames(): NameServer {
  return () => new Promise<Response>(() => {});
}

function refusedNames(): NameServer {
  return async () => json({ title: "Forbidden", status: 403 }, 403);
}

function stub(
  dsr: DataSubjectRequest,
  roster: RosterServer,
  names: NameServer = namesOf({}),
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      if (url.pathname.endsWith("/me")) {
        // The queue reads on `privacy_request:read`: its rows name the contacts
        // who exercised an Art. 15/17 right, so a reader needs that grant to
        // reach any of the rows this file is about.
        return json(
          meFixture({
            roles: ["admin"],
            allow: { privacy_request: ["read"] },
          }),
        );
      }
      if (url.pathname.endsWith("/data-subject-requests")) {
        return json({ data: [dsr], page: PAGE });
      }
      if (url.pathname.endsWith("/users/names")) {
        return names();
      }
      if (url.pathname.endsWith("/users")) {
        return roster(url.searchParams.get("cursor"));
      }
      return json({});
    }),
  );
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

/**
 * Opens the one request in the queue and hands back its assignee field, plus the
 * session that opened it — one `userEvent` instance per test, because a second
 * one forgets which keys and buttons the first left held.
 */
async function openAssignee(): Promise<{
  user: UserEvent;
  picker: HTMLElement;
}> {
  const user = userEvent.setup();
  render(<PrivacyInboxCard />);
  await user.click(
    await screen.findByRole("button", { name: /8f3a-contact-uuid/i }),
  );
  return { user, picker: await screen.findByLabelText(en["privacy.assignee"]) };
}

beforeEach(() => localStorage.setItem("margince.workspaceSlug", "acme"));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("an assignee the picker's own list does not offer", () => {
  it("names the holder a finished roster does not carry, rather than reading as unassigned", async () => {
    // "u-gone" settles on nothing at `/users/names`: an archived seat, or an
    // id this installation never held. A deactivated colleague is still named.
    stub(dsrAssignedTo("u-gone"), oneRosterPage([member("u1", "Dana DPO")]));

    const { user, picker } = await openAssignee();

    expect(picker).toHaveTextContent(en["ref.notInRoster"]);
    // The em dash is the face of a request assigned to NOBODY. This one is
    // assigned, and a DPO who reads it as unassigned reassigns the work off the
    // colleague doing it with the statutory clock running.
    expect(picker).not.toHaveTextContent("—");

    // Legible without being offered, exactly as the unassigned entry is:
    // re-choosing the holder this request already has changes nothing.
    await user.click(picker);
    expect(
      screen.getByRole("option", { name: en["ref.notInRoster"] }),
    ).toHaveAttribute("aria-disabled", "true");
  });

  it("says the name is still coming while its own read is still running", async () => {
    // The picker's OFFERED list settles fine; it is the assignee's own by-id
    // name read (GET /users/names) that has not answered, and that read is
    // what this field waits on — the roster walk beside it is a separate
    // question.
    stub(
      dsrAssignedTo("u-gone"),
      oneRosterPage([member("u1", "Dana DPO")]),
      unansweredNames(),
    );

    const { picker } = await openAssignee();

    // Reporting the holder as departed here would be a claim about who is
    // answering a statutory request, made on the evidence of nothing having
    // arrived yet.
    expect(picker).toHaveTextContent(en["common.loading"]);
    expect(picker).not.toHaveTextContent("—");
  });

  it("says the name failed to load, never that the holder departed, when the read is refused", async () => {
    // A 403 excludes nobody. Reading it as a settled absence reports the
    // holder as gone on the evidence of a refusal rather than a name that
    // never arrived — the same confusion the unanswered-read case above
    // guards against, on the failed-read side of it.
    stub(
      dsrAssignedTo("u-gone"),
      oneRosterPage([member("u1", "Dana DPO")]),
      refusedNames(),
    );

    const { picker } = await openAssignee();

    await screen.findByText(en["ref.nameLoadFailed"]);
    expect(picker).toHaveTextContent(en["ref.nameLoadFailed"]);
    expect(picker).not.toHaveTextContent(en["ref.notInRoster"]);
  });

  it("names an assignee even while the offered list is still a partial one", async () => {
    // The by-id read that names the assignee is independent of the roster walk
    // that builds the offered list: a walk that stopped short of the workspace
    // still leaves this field able to say who holds the request, which
    // `state.partial` reports as a fact about the OPTIONS, not about the name
    // above them.
    stub(
      dsrAssignedTo("u-far"),
      endlessRoster(),
      namesOf({ "u-far": "Fara Nolan" }),
    );

    const { picker } = await openAssignee();

    expect(await screen.findByText("Fara Nolan")).toBeInTheDocument();
    expect(picker).toHaveTextContent("Fara Nolan");
    expect(await screen.findByText(en["state.partial"])).toBeInTheDocument();
  });

  it("names an agent holder by their own name, while still refusing to offer one", async () => {
    // An agent seat cannot hold the row scope a subject request needs, so the
    // picker never offers one — but a request already assigned to one is a fact
    // the field has to be able to state, and the by-id read can name it.
    stub(
      dsrAssignedTo("u-bot"),
      oneRosterPage([
        member("u1", "Dana DPO"),
        member("u-bot", "Erasure Runner", true),
      ]),
      namesOf({ "u-bot": "Erasure Runner" }),
    );

    const { user, picker } = await openAssignee();
    expect(picker).toHaveTextContent("Erasure Runner");

    await user.click(picker);
    const offered = screen.getByRole("option", { name: "Erasure Runner" });
    expect(offered).toHaveAttribute("aria-disabled", "true");
    expect(
      screen.getByRole("option", { name: "Dana DPO" }),
    ).not.toHaveAttribute("aria-disabled", "true");
  });
});

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { CompanyFacts } from "./companyfacts";
import {
  CompanyActionBadges,
  CompanyIdentityLine,
  CompanyPrimaryActions,
  CompanyRelationshipBadges,
} from "./companyheader";

// Who wrote the record, beside when it was written. The tag has always been able
// to name the author — `ProvenanceTag` takes a `renderUser` — and the header has
// always had the roster in hand, because the owner control on the line above
// reads it. Nobody connected the two, so a record every colleague could see
// reported its author as "a person".
//
// The fallback is the half worth pinning: the roster walk is bounded and the
// list it walks excludes archived members, so a name that cannot be resolved
// must go back to "typed by a person" rather than forward to the raw uuid.
// "typed by 3f2b8c…" is not more information than "typed by a person", it is the
// same non-answer with a reader-hostile spelling.

type Organization = components["schemas"]["Organization"];

// Typed, not asserted. A fixture cast into the contract type can drop a required
// field and still compile, so the test would go on passing after the wire shape
// moved under it — which is the one thing a fixture must not do.
const ORG: Organization = {
  // Absent reads as NOT writable, which is the fail-closed default a real
  // response never relies on: the server answers this per row.
  writable: true,
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  lifecycle: "customer",
  owner_id: "u-owner",
  captured_by: "human:u-author",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// `roster` is what /users answers with, as one complete page — the walk stops on
// a null cursor. An empty one is the honest shape of an author the roster does
// not carry, not a broken stub.
function stub(roster: ReadonlyArray<{ id: string; display_name: string }>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      const body = pathname.endsWith("/me")
        ? { user: { id: "u-reader", display_name: "The Reader" }, allow: {} }
        : { data: roster, page: { has_more: false, next_cursor: null } };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

// /me answering exactly `allow`, and nothing else — for CompanyPrimaryActions'
// own tests, which read a grant `stub` above never carried (its `/me` has no
// `authorization` at all, so `useCanWrite` would deny regardless of `allow`).
function stubGrants(allow: GrantSpec) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      const body = pathname.endsWith("/me")
        ? meFixture({ allow })
        : { data: [], page: { has_more: false, next_cursor: null } };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
}

// /me hangs, so `useCanWrite` reads `undefined` on the frames before it
// answers — for CompanyPrimaryActions' own pending-grant test, which asserts
// nothing claims a refusal `/me` has not decided yet.
function stubMeInFlight(): Array<(response: Response) => void> {
  const answer: Array<(response: Response) => void> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((request: Request) => {
      if (new URL(request.url).pathname.endsWith("/me")) {
        return new Promise<Response>((resolve) => {
          answer.push(resolve);
        });
      }
      return Promise.resolve(
        new Response(
          JSON.stringify({
            data: [],
            page: { has_more: false, next_cursor: null },
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
      );
    }),
  );
  return answer;
}

// /me answers, /users does not — the header on the first frames of a page load,
// held still. Nothing waits on a clock: the resolvers are collected so a test
// can let the roster answer when it wants to assert the settled reading.
function stubRosterInFlight(): Array<(response: Response) => void> {
  const answer: Array<(response: Response) => void> = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((request: Request) => {
      if (new URL(request.url).pathname.endsWith("/me")) {
        return Promise.resolve(
          new Response(
            JSON.stringify({
              user: { id: "u-reader", display_name: "The Reader" },
              allow: {},
            }),
            { status: 200, headers: { "content-type": "application/json" } },
          ),
        );
      }
      return new Promise<Response>((resolve) => {
        answer.push(resolve);
      });
    }),
  );
  return answer;
}

// /me answers, /users is refused — the reading a reader gets when the roster
// read comes back with nothing to say about anyone.
function stubRosterRefused() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      if (new URL(request.url).pathname.endsWith("/me")) {
        return new Response(
          JSON.stringify({
            user: { id: "u-reader", display_name: "The Reader" },
            allow: {},
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        );
      }
      return new Response(JSON.stringify({ title: "Forbidden", status: 403 }), {
        status: 403,
        headers: { "content-type": "application/problem+json" },
      });
    }),
  );
}

// The tag is one span carrying "typed by" and the name as sibling text nodes, so
// the reading a human gets is the span's whole text — asserting on the name alone
// would pass on markup that never says what the name is doing there.
function provenanceText(): string {
  const tag = document.querySelector(".provenance-human");
  if (!tag) {
    throw new Error("the identity line rendered no human provenance tag");
  }
  return tag.textContent?.replace(/\s+/g, " ").trim() ?? "";
}

function renderInApp(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

function renderLine() {
  renderInApp(<CompanyIdentityLine org={ORG} />);
}

// The owner control's mount. It sits in the record's facts box beside the
// name rather than mid-sentence in the identity meta line — one component,
// one mount, so the three roster states below are asserted where a reader
// actually meets them.
function renderFacts() {
  renderInApp(<CompanyFacts org={ORG} />);
}

describe("who wrote this record", () => {
  it("names the author the roster can resolve", async () => {
    stub([
      { id: "u-author", display_name: "Sofia Meier" },
      { id: "u-owner", display_name: "Mira Voss" },
    ]);
    renderLine();

    await waitFor(() => expect(provenanceText()).toBe("typed by Sofia Meier"));
    expect(screen.queryByText("typed by a person")).toBeNull();
  });

  it("says a person wrote it, not a uuid, when the roster cannot resolve them", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderLine();

    expect(await screen.findByText("typed by a person")).toBeTruthy();
    // The id must not reach the page in any form — neither whole nor truncated,
    // which is what the generic record reference would have rendered.
    expect(provenanceText()).toBe("typed by a person");
    expect(document.body.textContent).not.toContain("u-author");
  });
});

// Who OWNS the record, in its facts box and off the same roster read. The owner
// has three states and the header used to have two: it read the owner through
// the generic record reference, which paints the id whenever it has no name in
// hand — so every company page opened with a uuid in its header and swapped it
// for a name a moment later.
describe("who owns this record", () => {
  it("names the owner the roster can resolve", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderFacts();

    expect(await screen.findByText("Mira Voss")).toBeTruthy();
    expect(document.body.textContent).not.toContain("u-owner");
  });

  it("does not call the owner gone while the roster read is still in flight", async () => {
    const answer = stubRosterInFlight();
    renderFacts();

    // "no longer in the user list" is a claim about a read that came back. Said
    // over one still running, it reports an owner as departed on the evidence
    // of nothing having arrived yet.
    expect(await screen.findByText("Loading…")).toBeTruthy();
    expect(
      screen.queryByText("Current owner (no longer in the user list)"),
    ).toBeNull();
    expect(document.body.textContent).not.toContain("u-owner");

    for (const resolve of answer) {
      resolve(
        new Response(
          JSON.stringify({
            data: [{ id: "u-owner", display_name: "Mira Voss" }],
            page: { has_more: false, next_cursor: null },
          }),
          { status: 200, headers: { "content-type": "application/json" } },
        ),
      );
    }
    expect(await screen.findByText("Mira Voss")).toBeTruthy();
  });

  it("does not call the owner gone when the roster read failed", async () => {
    stubRosterRefused();
    renderFacts();

    // A refused read excludes nobody. Reading it as "no longer in the user
    // list" turns a 403 into a fact about who owns this account, which is the
    // one thing this control is here to get right.
    expect(await screen.findByText("Name didn't load")).toBeTruthy();
    expect(
      screen.queryByText("Current owner (no longer in the user list)"),
    ).toBeNull();
    expect(document.body.textContent).not.toContain("u-owner");
  });

  it("says the owner is outside the user list once the roster has answered without them", async () => {
    stub([]);
    renderFacts();

    expect(await screen.findByText(en["ref.notInRoster"])).toBeTruthy();
    // An owner the roster cannot name is still not shown as a uuid: waiting
    // will not resolve them, and their id answers no question a reader has.
    expect(document.body.textContent).not.toContain("u-owner");
  });
});

// The edit form prefills the same owner off the same roster read, so it made
// the same claim one control over: a reader who opened the form to check what
// the header said was told "no longer in the user list" a second time, by a
// read that had excluded nobody.
describe("the owner the edit form prefills", () => {
  it("does not call the owner gone when the roster read failed", async () => {
    stubRosterRefused();
    const user = userEvent.setup();
    renderInApp(
      <CompanyActionBadges
        org={ORG}
        onOpenHistory={() => undefined}
        onSetUpPartner={() => undefined}
      />,
    );

    await user.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    await user.click(await screen.findByTestId("edit-record"));

    expect(await screen.findByText("Name didn't load")).toBeTruthy();
    expect(
      screen.queryByText("Current owner (no longer in the user list)"),
    ).toBeNull();
  });
});

// The account page asked nothing before pressing Log activity/Add task — the
// contact page's identical verb already asks `useCanWrite("activity",
// "create")` and stays visible, refused, before a read seat types a word.
// Without this, a read-only seat opened the form, typed a note, and was
// refused only on submit.
describe("Log activity and Add task, gated on the create grant", () => {
  it("refuses both without the grant, over one shared sentence", async () => {
    stubGrants({});
    const user = userEvent.setup();
    renderInApp(
      <CompanyPrimaryActions
        org={ORG}
        composerOpen={false}
        onComposerOpen={() => undefined}
      />,
    );

    const log = await screen.findByRole("button", { name: "Log activity" });
    const task = await screen.findByRole("button", { name: "Add task" });
    expect(log.hasAttribute("disabled")).toBe(true);
    expect(task.hasAttribute("disabled")).toBe(true);
    const describedBy = log.getAttribute("aria-describedby");
    expect(describedBy).toBeTruthy();
    expect(task.getAttribute("aria-describedby")).toBe(describedBy);
    expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
      "You do not have permission to log activities on this record.",
    );

    // Both refused for the SAME reason, so the reader reads it once: pressing
    // one does not open a form the store would refuse anyway.
    await user.click(log);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("presses both, with the grant", async () => {
    stubGrants({ activity: ["create"] });
    renderInApp(
      <CompanyPrimaryActions
        org={ORG}
        composerOpen={false}
        onComposerOpen={() => undefined}
      />,
    );

    const log = await screen.findByRole("button", { name: "Log activity" });
    const task = await screen.findByRole("button", { name: "Add task" });
    expect(log.hasAttribute("disabled")).toBe(false);
    expect(task.hasAttribute("disabled")).toBe(false);
  });

  // A guard that has not answered yet refuses nothing: while /me is still in
  // flight, useCanWrite reads false the same way it would for a caller
  // without the grant — so without this, both buttons would flash the
  // "You do not have permission" sentence on every page load, before the
  // verdict is even in.
  it("stays quiet — disabled, no claimed refusal — while the grant is still in flight", async () => {
    const answer = stubMeInFlight();
    renderInApp(
      <CompanyPrimaryActions
        org={ORG}
        composerOpen={false}
        onComposerOpen={() => undefined}
      />,
    );

    const log = await screen.findByRole("button", { name: "Log activity" });
    const task = await screen.findByRole("button", { name: "Add task" });
    expect(log.hasAttribute("disabled")).toBe(true);
    expect(task.hasAttribute("disabled")).toBe(true);
    expect(
      screen.queryByText(
        "You do not have permission to log activities on this record.",
      ),
    ).toBeNull();

    // Settled before the test ends, or the mocked request outlives it — the
    // same reason stubRosterInFlight's own test resolves its capture above.
    for (const resolve of answer) {
      resolve(
        new Response(JSON.stringify(meFixture({ allow: {} })), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
      );
    }
  });
});

// STATE-4a decides absent-vs-disabled by CAUSE, and an archive is state: the
// menu used to drop every verb on an archived account, leaving a reader unable
// to tell a record that is closed from a build without an edit button.
describe("an archived account's verbs", () => {
  it("stay in the menu, refused, each reachable from the one sentence naming the archive", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    const user = userEvent.setup();
    renderInApp(
      <CompanyActionBadges
        org={{ ...ORG, archived_at: "2026-07-13T00:00:00Z" }}
        onOpenHistory={() => undefined}
        onSetUpPartner={() => undefined}
      />,
    );

    await user.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    // Each WAITED for. Only the first was, so the three below were read in
    // the tick it arrived in — and a menu whose items mount a beat apart threw
    // here, which is the shape that fails under a loaded run and never alone.
    const refused = [
      await screen.findByTestId("edit-record"),
      await screen.findByTestId("merge-record"),
      await screen.findByTestId("archive-record"),
      await screen.findByTestId("share-record"),
    ];
    for (const control of refused) {
      expect(control.hasAttribute("disabled")).toBe(true);
      // The reason has to be reachable FROM the control: a disabled button
      // cannot be focused and a `title` on it is announced by nobody, so a
      // sentence the control does not point at reaches no reader who needed it.
      const describedBy = control.getAttribute("aria-describedby");
      expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
        "This company is archived. Restore it to change anything on it.",
      );
    }
    // The reads next to them are untouched: what happened to a record is
    // exactly what a reader wants after it has been put away.
    expect(
      screen.getByTestId("company-full-history").hasAttribute("disabled"),
    ).toBe(false);
  });
});

// The header draws two vocabularies that overlap on `customer`: where the
// account STANDS (lifecycle, the editable badge beside the name) and what it IS
// to us (relationship types). A customer account carries the value in both, and
// the strip printed "Customer" twice from two fields that happened to agree —
// one fact rendered as a second reading confirming the first.
//
// The readings row that used to guard this compared the two itself and has since
// been removed; the guard was local to it and never covered the header. So it is
// pinned here, on the badges themselves — the ones beside the record's NAME,
// which is where a tag on the record belongs and the only place they are drawn.
describe("an account whose lifecycle and relationship agree", () => {
  it("says the word once, and keeps every relationship the lifecycle is not already saying", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderInApp(
      <CompanyRelationshipBadges
        org={{ ...ORG, relationship_types: ["customer", "partner"] }}
      />,
    );

    // These badges are what this component draws; the lifecycle badge is the
    // other mount, so a duplicate here is one "Customer" too many on its own.
    expect(await screen.findByText(en["org.relType.partner"])).toBeTruthy();
    expect(screen.queryByText(en["org.relType.customer"])).toBeNull();
  });

  it("still draws a relationship the lifecycle disagrees with", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderInApp(
      <CompanyRelationshipBadges
        org={{
          ...ORG,
          lifecycle: "prospect",
          relationship_types: ["customer"],
        }}
      />,
    );

    // An account can be worked as a prospect and be a customer of something
    // else already — dropping the badge because the two words differ would hide
    // a true reading rather than a repeated one.
    expect(await screen.findByText(en["org.relType.customer"])).toBeTruthy();
  });
});

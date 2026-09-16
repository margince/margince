/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { CompanyDetails } from "./companydetails";
import { CompanyFacts } from "./companyfacts";
import {
  CompanyActionBadges,
  CompanyRelationshipBadges,
} from "./companyheader";

// Who wrote the record and the record's own verbs are pinned in
// companyheaderfacts.test.tsx and companyheaderactions.test.tsx: the two
// pieces that moved out of this file when the header was split for the
// contact record page's own shapes. What stays here is the header's
// EDITABLE pieces (lifecycle, owner) and its menu (CompanyActionBadges).

type Company = components["schemas"]["Company"];

// Typed, not asserted. A fixture cast into the contract type can drop a required
// field and still compile, so the test would go on passing after the wire shape
// moved under it — which is the one thing a fixture must not do.
const COMPANY: Company = {
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

// The grants the reader holds wherever a spec is about something other than
// the grant: the record verbs ask `company.update` before they draw, so a
// /me with no authorization at all would refuse every Edit these specs open.
const READER = {
  authorization: meFixture({
    allow: { company: ["read", "update", "delete"] },
  }).authorization,
};

// `roster` is what /users answers with, as one complete page — the walk stops on
// a null cursor. An empty one is the honest shape of an author the roster does
// not carry, not a broken stub.
function stub(roster: ReadonlyArray<{ id: string; display_name: string }>) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      // The details card also reads the record's tags; a roster handed back
      // there would draw colleagues as tags.
      const body = pathname.endsWith("/me")
        ? { user: { id: "u-reader", display_name: "The Reader" }, ...READER }
        : pathname.includes("/tags")
          ? { data: [], page: { has_more: false, next_cursor: null } }
          : { data: roster, page: { has_more: false, next_cursor: null } };
      return new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
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
              ...READER,
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
            ...READER,
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

// The owner control's mount. It sits in the record's facts box beside the
// name rather than mid-sentence in the identity meta line — one component,
// one mount, so the three roster states below are asserted where a reader
// actually meets them.
function renderFacts() {
  renderInApp(<CompanyFacts company={COMPANY} />);
}

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
describe("the owner in Details", () => {
  it("keeps the current owner when the roster failed", async () => {
    stubRosterRefused();
    const user = userEvent.setup();
    renderInApp(<CompanyDetails company={COMPANY} />);
    await user.click(
      await screen.findByRole("button", { name: "Change Owner" }),
    );
    expect(screen.getByRole("combobox").textContent).toContain(
      en["ref.nameLoadFailed"],
    );
    expect(
      screen.queryByText("Current owner (no longer in the user list)"),
    ).toBeNull();
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
        company={{ ...COMPANY, archived_at: "2026-07-13T00:00:00Z" }}
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
        company={{ ...COMPANY, relationship_types: ["customer", "partner"] }}
      />,
    );

    // These badges are what this component draws; the lifecycle badge is the
    // other mount, so a duplicate here is one "Customer" too many on its own.
    expect(await screen.findByText(en["company.relType.partner"])).toBeTruthy();
    expect(screen.queryByText(en["company.relType.customer"])).toBeNull();
  });

  it("still draws a relationship the lifecycle disagrees with", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderInApp(
      <CompanyRelationshipBadges
        company={{
          ...COMPANY,
          lifecycle: "prospect",
          relationship_types: ["customer"],
        }}
      />,
    );

    // An account can be worked as a prospect and be a customer of something
    // else already — dropping the badge because the two words differ would hide
    // a true reading rather than a repeated one.
    expect(
      await screen.findByText(en["company.relType.customer"]),
    ).toBeTruthy();
  });
});

// The header's two groups answer one question between them — which verbs are
// worth a place on the page and which are worth a line in a list — and the
// answer is legible only if each group keeps its own shape. A menu row that
// grows a glyph puts its words on a second left edge; a header button that
// loses its glyph becomes a link among three buttons. Both had happened here.
describe("the shape of the header's verbs", () => {
  // The order every record type carries: the four verbs common to all of them
  // first, then what is particular to an account, then the one verb a reader
  // cannot walk back. A menu whose rows move between record types is a menu
  // read from the top every time instead of aimed at.
  it("lists the menu's verbs in the order every record carries them", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    const user = userEvent.setup();
    renderInApp(
      <CompanyActionBadges
        company={COMPANY}
        onOpenHistory={() => undefined}
        onSetUpPartner={() => undefined}
      />,
    );

    await user.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    // Waited for by its own handle first: the panel mounts its children on the
    // open, so reading the whole list in the tick the click returned in would
    // catch whichever rows had committed by then.
    await screen.findByTestId("archive-record");
    const panel = document.querySelector(".overflow-menu-items");
    if (!panel) {
      throw new Error("the menu drew no panel");
    }
    expect(
      [...panel.querySelectorAll("button")].map((row) => row.textContent),
    ).toEqual([
      en["merge.company"],
      en["record.share"],
      en["record.fullHistory"],
      en["company.partnerSetUp"],
      en["record.archive"],
    ]);
  });

  // Words, and only words. A row's glyph buys nothing a whole line of text
  // does not already say, and one glyph among eight rows indents that row's
  // words past every other row's — which is the ragged column `atoms.css`
  // reserves an empty icon slot to repair when a caller does it anyway.
  it("draws no glyph on any row of the menu", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    const user = userEvent.setup();
    renderInApp(
      <CompanyActionBadges
        company={COMPANY}
        onOpenHistory={() => undefined}
        onSetUpPartner={() => undefined}
      />,
    );

    await user.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    await screen.findByTestId("archive-record");
    const panel = document.querySelector(".overflow-menu-items");
    expect(panel?.querySelector("svg")).toBeNull();
  });
});

it("names an owner for readers who cannot edit the company", async () => {
  stub([{ id: "u-owner", display_name: "Mira Voss" }]);
  renderInApp(<CompanyDetails company={{ ...COMPANY, writable: false }} />);
  expect(await screen.findByText("Mira Voss")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Change Owner" })).toBeNull();
});
it("does not offer to clear a company's lifecycle", async () => {
  stub([]);
  const user = userEvent.setup();
  renderInApp(<CompanyDetails company={COMPANY} />);
  await user.click(
    await screen.findByRole("button", { name: "Change Account lifecycle" }),
  );
  await user.click(screen.getByRole("combobox"));
  expect(screen.queryByRole("option", { name: "Not set" })).toBeNull();
});

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { meFixture } from "../../testing/mefixture";
import { CompanyActionBadges, CompanyOwnerControl } from "./companyheader";
import { CompanyHeadLines } from "./companyheaderfacts";
import { CompanyLifecycleControl } from "./companylifecycle";
import { CompanyMarks } from "./companymarks";

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
function stub(
  roster: ReadonlyArray<{ id: string; display_name: string; status?: string }>,
  reader = READER,
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      // The details card also reads the record's tags; a roster handed back
      // there would draw colleagues as tags.
      const body = pathname.endsWith("/me")
        ? { user: { id: "u-reader", display_name: "The Reader" }, ...reader }
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

// The owner control, as the details' Ownership row mounts it.
function renderFacts() {
  renderInApp(<CompanyOwnerControl company={COMPANY} />);
}

// Who owns the record, off the roster read. The owner has three states.
// Reading the owner through the generic record reference would paint the id
// whenever it has no name in hand. Every company page would then open with a
// uuid in its header and swap it for a name a moment later.
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
    expect(
      (await screen.findByRole("button", { name: "Change Owner" })).textContent,
    ).toContain("Loading…");
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
    expect(await screen.findByText("Name did not load")).toBeTruthy();
    expect(
      screen.queryByText("Current owner (no longer in the user list)"),
    ).toBeNull();
    expect(document.body.textContent).not.toContain("u-owner");
  });

  it("names an owner who is invited but has not signed in yet", async () => {
    stub([
      { id: "u-owner", display_name: "Rainer Schuller", status: "invited" },
    ]);
    renderFacts();

    // An import hands accounts to colleagues before they first sign in. They
    // are not offered as a new owner, and they are still named as this one.
    expect(await screen.findByText("Rainer Schuller")).toBeTruthy();
    expect(screen.queryByText(en["ref.notInRoster"])).toBeNull();
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

// An UNOWNED account is writable only by an unbounded seat, so for everyone
// else the header's owner cell is the one door out of that state: picking
// yourself there is the claim, not a patch the server would refuse.
describe("claiming an unowned account", () => {
  const UNOWNED: Company = { ...COMPANY, owner_id: undefined, writable: false };

  function stubClaim(
    reader: typeof READER = READER,
    roster: ReadonlyArray<{ id: string; display_name: string }> = [
      { id: "u-reader", display_name: "The Reader" },
    ],
  ) {
    const calls: Array<{
      method: string;
      path: string;
      ifMatch: string | null;
    }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const { pathname } = new URL(request.url);
        calls.push({
          method: request.method,
          path: pathname,
          ifMatch: request.headers.get("If-Match"),
        });
        const body = pathname.endsWith("/me")
          ? { user: { id: "u-reader", display_name: "The Reader" }, ...reader }
          : pathname.endsWith("/claim")
            ? { ...UNOWNED, owner_id: "u-reader", writable: true, version: 2 }
            : { data: roster, page: { has_more: false, next_cursor: null } };
        return new Response(JSON.stringify(body), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }),
    );
    return calls;
  }

  it("claims the account when the reader picks themselves", async () => {
    const calls = stubClaim();
    const user = userEvent.setup();
    renderInApp(<CompanyOwnerControl company={UNOWNED} />);

    await user.click(
      await screen.findByRole("button", { name: "Change Owner" }),
    );
    await user.click(await screen.findByRole("option", { name: "The Reader" }));

    await waitFor(() =>
      expect(calls.find((call) => call.path.endsWith("/claim"))).toMatchObject({
        method: "POST",
        path: "/v1/records/company/o-1/claim",
        ifMatch: "1",
      }),
    );
    // The claim door, never a PATCH: an ownerless row is nobody's to patch.
    expect(calls.some((call) => call.method === "PATCH")).toBe(false);
  });

  const TEAM = [
    { id: "u-reader", display_name: "The Reader" },
    { id: "u-colleague", display_name: "A Colleague" },
  ];

  it("offers a reader who cannot write the row only themselves", async () => {
    stubClaim(READER, TEAM);
    const user = userEvent.setup();
    renderInApp(<CompanyOwnerControl company={UNOWNED} />);

    await user.click(
      await screen.findByRole("button", { name: "Change Owner" }),
    );
    await screen.findByRole("option", { name: "The Reader" });
    expect(
      screen.getAllByRole("option").map((option) => option.textContent),
    ).toEqual([en["co.pulse.unowned"], "The Reader"]);
  });

  it("offers every colleague to a reader who can write the unowned row", async () => {
    stubClaim(READER, TEAM);
    const user = userEvent.setup();
    renderInApp(
      <CompanyOwnerControl company={{ ...UNOWNED, writable: true }} />,
    );

    await user.click(
      await screen.findByRole("button", { name: "Change Owner" }),
    );
    expect(
      await screen.findByRole("option", { name: "A Colleague" }),
    ).toBeTruthy();
  });

  it("offers no claim to a reader without the grant to take accounts", async () => {
    stubClaim({
      authorization: meFixture({ allow: { company: ["read"] } }).authorization,
    });
    renderInApp(<CompanyOwnerControl company={UNOWNED} />);

    expect(await screen.findByText(en["field.unset"])).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Change Owner" })).toBeNull();
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
        "This company is archived and takes no changes.",
      );
    }
    // The reads next to them are untouched: what happened to a record is
    // exactly what a reader wants after it has been put away.
    expect(
      screen.getByTestId("company-full-history").hasAttribute("disabled"),
    ).toBe(false);
  });
});

// The design draws no relationship badge beside the name. The lifecycle chip is
// the one pill that says where the account stands, and the relationship types
// are a Profile tab field.
describe("the marks under an account's name", () => {
  it("draws the lifecycle and no relationship badge, whatever the types", async () => {
    stub([{ id: "u-owner", display_name: "Mira Voss" }]);
    renderInApp(
      <CompanyMarks
        company={{ ...COMPANY, relationship_types: ["customer", "partner"] }}
      />,
    );

    expect(
      await screen.findByText(en["company.lifecycle.customer"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["company.relType.partner"])).toBeNull();
  });
});

// The header's two groups answer one question between them — which verbs are
// worth a place on the page and which are worth a line in a list — and the
// answer is legible only if each group keeps its own shape. A menu row that
// grows a glyph puts its words on a second left edge; a header button that
// loses its glyph becomes a link among three buttons. Both had happened here.
// The program opens on the Partner tab, which reads the partner register. A
// seat that may edit the company but not read partners is told so up front.
describe("setting up a partner program", () => {
  async function setUpItem(partner: Array<"read" | "update">) {
    stub([{ id: "u-owner", display_name: "Mira Voss" }], {
      authorization: meFixture({
        allow: { company: ["read", "update"], partner },
      }).authorization,
    });
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
    return screen.findByRole("button", { name: en["company.partnerSetUp"] });
  }

  it("is open to a company editor who reads partners, without the partner grant", async () => {
    const item = await setUpItem(["read"]);
    await waitFor(() => expect(item.hasAttribute("disabled")).toBe(false));
  });

  it("is refused, with the reason, to a seat that cannot read partners", async () => {
    const item = await setUpItem([]);
    await waitFor(() => expect(item.hasAttribute("disabled")).toBe(true));
    const describedBy = item.getAttribute("aria-describedby");
    expect(document.getElementById(describedBy ?? "")?.textContent).toBe(
      en["company.partnerSetUpUnreadable"],
    );
  });
});

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
  renderInApp(
    <CompanyOwnerControl company={{ ...COMPANY, writable: false }} />,
  );
  expect(await screen.findByText("Mira Voss")).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Change Owner" })).toBeNull();
});
// The stage beside the account's name. A real control, not the inline value
// the Details grid carries: the header's is the design system's dropdown worn
// as a filled button, and these pin that it is one, that it writes through
// the ordinary PATCH, and that a refused write keeps the reader's answer.
describe("the lifecycle control beside the name", () => {
  // /me grants the update; PATCH answers with `patchStatus`, echoing what was
  // sent on success. Every body sent is collected so a case can assert the
  // exact write rather than that some request happened.
  function stubPatch(patchStatus: number) {
    const sent: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const { pathname } = new URL(request.url);
        const json = (body: unknown, status = 200) =>
          new Response(JSON.stringify(body), {
            status,
            headers: {
              "content-type":
                status >= 400 ? "application/problem+json" : "application/json",
            },
          });
        if (pathname.endsWith("/me")) {
          return json({
            user: { id: "u-reader", display_name: "The Reader" },
            ...READER,
          });
        }
        if (request.method === "PATCH") {
          const body: unknown = await request.json();
          sent.push(body);
          if (patchStatus >= 400) {
            return json(
              {
                title: "Conflict",
                status: patchStatus,
                detail: "Someone else changed this account.",
              },
              patchStatus,
            );
          }
          return json({ ...COMPANY, lifecycle: "prospect", version: 2 });
        }
        return json({ data: [], page: { has_more: false, next_cursor: null } });
      }),
    );
    return sent;
  }

  it("is a button-shaped control, not an inline value", async () => {
    stubPatch(200);
    renderInApp(<CompanyLifecycleControl company={COMPANY} />);
    const control = await screen.findByRole("combobox", { name: "Lifecycle" });
    // The button's box (`--controlHeight`), not the inline value's 24px one:
    // what the record layout suite measures is this element.
    expect(control.classList.contains("btn")).toBe(true);
    expect(control.classList.contains("inline-editable")).toBe(false);
    expect(control.getAttribute("data-testid")).toBe("company-lifecycle");
    expect(control.textContent).toContain("Customer");
  });

  it("writes the stage the reader picks, once", async () => {
    const sent = stubPatch(200);
    const user = userEvent.setup();
    renderInApp(<CompanyLifecycleControl company={COMPANY} />);
    await user.click(
      await screen.findByRole("combobox", { name: "Lifecycle" }),
    );
    await user.click(screen.getByRole("option", { name: "Prospect" }));
    await waitFor(() => expect(sent).toEqual([{ lifecycle: "prospect" }]));
  });

  it("does not write the stage the account already has", async () => {
    const sent = stubPatch(200);
    const user = userEvent.setup();
    renderInApp(<CompanyLifecycleControl company={COMPANY} />);
    await user.click(
      await screen.findByRole("combobox", { name: "Lifecycle" }),
    );
    await user.click(screen.getByRole("option", { name: "Customer" }));
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(sent).toEqual([]);
  });

  it("keeps the reader's pick and says why when the write is refused", async () => {
    stubPatch(409);
    const user = userEvent.setup();
    renderInApp(<CompanyLifecycleControl company={COMPANY} />);
    const control = await screen.findByRole("combobox", { name: "Lifecycle" });
    await user.click(control);
    await user.click(screen.getByRole("option", { name: "Prospect" }));
    expect(
      await screen.findByText("Someone else changed this account."),
    ).toBeTruthy();
    expect(control.textContent).toContain("Prospect");
    expect(control.getAttribute("aria-invalid")).toBe("true");
  });

  it("drops a refused pick once the stage is saved from somewhere else", async () => {
    stubPatch(409);
    const user = userEvent.setup();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const at = (company: Company) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <CompanyLifecycleControl company={company} />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const { rerender } = render(at(COMPANY));
    const control = await screen.findByRole("combobox", { name: "Lifecycle" });
    await user.click(control);
    await user.click(screen.getByRole("option", { name: "Prospect" }));
    await screen.findByText("Someone else changed this account.");
    // The Details grid saved a different stage; the record refetched with it.
    rerender(at({ ...COMPANY, lifecycle: "former_customer", version: 2 }));
    expect(control.textContent).toContain("Former customer");
    expect(screen.queryByText("Someone else changed this account.")).toBeNull();
    expect(control.getAttribute("aria-invalid")).not.toBe("true");
  });

  it("does not draw a late refusal over a stage saved meanwhile", async () => {
    let refuse: (response: Response) => void = () => {};
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
        if (request.method === "PATCH") {
          return new Promise<Response>((resolve) => {
            refuse = resolve;
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
    const user = userEvent.setup();
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const at = (company: Company) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <CompanyLifecycleControl company={company} />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const { rerender } = render(at(COMPANY));
    const control = await screen.findByRole("combobox", { name: "Lifecycle" });
    await user.click(control);
    await user.click(screen.getByRole("option", { name: "Prospect" }));
    // While the header's write is out, the Details grid saves another stage.
    rerender(at({ ...COMPANY, lifecycle: "former_customer", version: 2 }));
    refuse(
      new Response(
        JSON.stringify({
          title: "Conflict",
          status: 409,
          detail: "Someone else changed this account.",
        }),
        {
          status: 409,
          headers: { "content-type": "application/problem+json" },
        },
      ),
    );
    await waitFor(() =>
      expect(control.getAttribute("aria-busy")).not.toBe("true"),
    );
    expect(screen.queryByText("Someone else changed this account.")).toBeNull();
    expect(control.textContent).toContain("Former customer");
  });

  it("shows the stage, with no control, to a reader who may not change it", async () => {
    stubPatch(200);
    renderInApp(
      <CompanyLifecycleControl company={{ ...COMPANY, writable: false }} />,
    );
    expect(await screen.findByText("Customer")).toBeTruthy();
    expect(screen.queryByRole("combobox", { name: "Lifecycle" })).toBeNull();
  });
});

// Under the name: one line per fact, then the pills, led by where the
// account stands.
describe("the head's lines and pills", () => {
  const withSite: Company = {
    ...COMPANY,
    industry: "automotive",
    website_url: "https://brandt.example",
  };

  it("puts what the account is and its site on lines of their own", () => {
    stub([]);
    renderInApp(<CompanyHeadLines company={withSite} />);
    expect(screen.getByText("Automotive").tagName).toBe("SPAN");
    expect(screen.getByText("brandt.example").closest("a")).toBeTruthy();
  });

  it("leads the pills with the lifecycle control", async () => {
    stub([]);
    renderInApp(<CompanyMarks company={withSite} />);
    expect(
      await screen.findByRole("combobox", { name: "Lifecycle" }),
    ).toBeTruthy();
    expect(screen.queryByText("Automotive")).toBeNull();
  });
});

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider, type Translator, translate } from "../i18n";
import { en } from "../i18n/en";
import { ContactRail, contactStanding } from "./contactrail";

// The rail's words are short verdicts — "One-sided", "Never", "No inbound",
// "Thin", "Nothing stands out on this relationship." — which is exactly why the
// withheld case has to be pinned here rather than left to review. A verdict
// reads as a measured fact whatever produced it, and the 360 hands an
// unreadable section to the client by leaving it out: absent-because-empty and
// absent-because-forbidden arrive as the same undefined field, distinguished
// only by `sections_omitted`.
//
// So every test below comes in a PAIR with the granted one beside it. A test
// that only checks the withheld side would pass just as happily against a rail
// that hedged every reading, and a rail that says "Not shown" about a contact
// who genuinely never wrote is the same defect pointed the other way.

type Contact360 = components["schemas"]["Contact360"];
type Activity = components["schemas"]["Activity"];

const page = { has_more: false, next_cursor: null };

// Timestamps are stated as an age rather than as a date: the readings are
// derived from the distance to now, so a fixed date would assert a different
// number of days on every day the suite runs.
//
// "Now" is PINNED rather than read from the host clock. Ages derived from a
// moving now make the expected reading depend on the moment the suite runs —
// including on the midnight boundary, where the same fixture is 3 days old
// before and 4 days old after.
const NOW = new Date("2026-08-18T09:00:00Z");

function daysAgo(days: number): string {
  return new Date(NOW.getTime() - days * 86_400_000).toISOString();
}

const contact: components["schemas"]["Contact"] = {
  id: "p-1",
  full_name: "Dana Buyer",
  first_name: "Dana",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  // The server's own per-row answer, which the rail's edit affordances ask
  // for. Absent it, this fixture describes a contact this reader may not
  // write, and every editor correctly disappears.
  writable: true,
};

function inboundEmail(): Activity {
  return {
    id: "a-1",
    kind: "email",
    direction: "inbound",
    subject: "Re: retrofit timeline",
    occurred_at: daysAgo(3),
    source: "gmail",
    captured_by: "connector:gmail",
    created_at: daysAgo(3),
    updated_at: daysAgo(3),
    is_done: false,
  };
}

// A reader holding every grant, looking at a relationship with something on it:
// they wrote three days ago, we wrote five days before that, one colleague
// knows them, and there is an open deal with a meeting booked against it.
const granted: Contact360 = {
  as_of: "2026-08-18T09:00:00Z",
  contact,
  sections_omitted: [],
  last_inbound_at: daysAgo(3),
  last_outbound_at: daysAgo(8),
  network: {
    colleagues: [
      {
        user_id: "u-2",
        display_name: "Sam Rivera",
        strength: 0.7,
        strength_bucket: "strong",
        interactions_90d: 6,
        last_at: daysAgo(3),
        inbound_90d: 3,
        outbound_90d: 3,
      },
    ],
  },
  employments: {
    data: [
      {
        relationship_id: "rel-1",
        company_id: "o-1",
        company_name: "Brandt Automotive GmbH",
        role: "Head of Fleet",
        is_current_primary: true,
        started_at: "2022-03-01T00:00:00Z",
        ended_at: null,
      },
    ],
    page,
  },
  activities: { data: [inboundEmail()], page },
  commercial: {
    role: "champion",
    committee: [
      { contact_id: "p-2", full_name: "Ines Klaas", role: "economic_buyer" },
    ],
    deal: { deal_id: "d-1", title: "Fleet retrofit" },
  },
  next_meeting: {
    activity_id: "a-2",
    starts_at: daysAgo(-2),
    subject: "Fleet retrofit walkthrough",
  },
};

// The same reader, on a contact nobody has ever corresponded with. Every
// section came back and every one of them is genuinely empty — the only state
// entitled to the rail's negative verdicts.
const emptyButGranted: Contact360 = {
  as_of: granted.as_of,
  contact,
  sections_omitted: [],
  last_inbound_at: null,
  last_outbound_at: null,
  network: { colleagues: [] },
  employments: { data: [], page },
  activities: { data: [], page },
  commercial: { committee: [] },
};

// A reader whose grants cover none of the relationship sections. The fields are
// absent, which is how the server says it — the list beside them is the only
// thing that distinguishes this from the record above.
const withheld: Contact360 = {
  as_of: granted.as_of,
  contact,
  sections_omitted: [
    "last_touch",
    "activities",
    "commercial",
    "next_meeting",
    "network",
    "employments",
  ],
};

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

// Every body this rail POSTs, in order, so a test can assert what was SENT
// rather than what the component happened to render afterwards. The employment
// modal's whole contract with the server is the shape of one request.
const sent: Array<{ method: string; path: string; body: unknown }> = [];

function mount(view: Contact360) {
  sent.length = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(
        input instanceof Request ? input.url : String(input),
        "https://test",
      );
      // openapi-fetch hands `fetch` a Request, not (url, init), so the body is
      // on the Request and reading it needs a clone — consuming the original
      // would leave the real call with an empty body.
      const request = input instanceof Request ? input : null;
      const method = request?.method ?? init?.method ?? "GET";
      if (method === "POST" || method === "PATCH") {
        const raw = request
          ? await request.clone().text()
          : String(init?.body ?? "");
        if (raw !== "") {
          sent.push({ method, path: url.pathname, body: JSON.parse(raw) });
        }
      }
      // The session probe is the one route that must answer a real body: an
      // unroutable /me fails every grant closed, which changes the edit
      // affordances the rail draws and nothing about its readings.
      if (url.pathname.endsWith("/me")) {
        return json(meFixture({ allow: { contact: ["read", "update"] } }));
      }
      // Every write to an employment row pins If-Match, and the version comes
      // from this list read. Answering it empty is not a neutral stub: the rail
      // refuses to write unpinned, so an empty list turns a "mark as ended"
      // click into a refusal and no request at all.
      if (method === "GET" && url.pathname.endsWith("/relationships")) {
        return json({
          data: (view.employments?.data ?? []).map((employment) => ({
            ...employment,
            id: employment.relationship_id,
            started_at: employment.started_at?.slice(0, 10),
            ended_at: employment.ended_at?.slice(0, 10),
            version: 3,
          })),
          page,
        });
      }
      // The employer picker searches companies; without a candidate the
      // modal's Save stays disabled and there is no request to inspect.
      if (url.pathname.endsWith("/companies")) {
        return json({
          data: [{ id: "o-9", display_name: "Employer GmbH" }],
          page,
        });
      }
      // Each employment row reads its employer's own record for the logo and
      // the contacts/open-deals counts. Answered only for the one company the
      // fixtures above actually name: every other id falls to the catch-all
      // below, which is what keeps the OTHER employment suites (a fresh
      // company id per row) exercising the no-counts path unchanged.
      if (url.pathname.endsWith("/companies/o-1")) {
        return json({
          id: "o-1",
          display_name: "Brandt Automotive GmbH",
          contact_count: 2,
          open_deal_count: 0,
          logo_url: null,
        });
      }
      // The record's tags. The catch-all below would answer with no `withheld`
      // key at all, which the panel reads as visible-and-empty — a state this
      // suite would then be asserting by accident rather than by choice.
      if (url.pathname.endsWith("/tags")) {
        return json({
          data: [
            {
              tag_id: "t-1",
              name: "Champion",
              color: "teal",
              archived: false,
              assigned_at: "2026-03-03T10:00:00Z",
            },
          ],
          withheld: false,
        });
      }
      return json({ data: [], page });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const tree = (shown: Contact360) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactRail view={shown} guard={undefined} />
      </LocaleProvider>
    </QueryClientProvider>
  );
  const { rerender } = render(tree(view));
  // Re-rendering the SAME tree with different rows, which is what the record
  // page does when its query refetches. Mounting a fresh one would reset the
  // state a caller may be trying to observe surviving.
  return { rerender: (shown: Contact360) => rerender(tree(shown)) };
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  vi.unstubAllGlobals();
});

// A section is scoped through its own heading: "Hidden — your role cannot read
// this" is one sentence the rail may say about four different sections, and an
// unscoped match cannot tell which one said it.
//
// Matched on the heading's PREFIX rather than on the whole summary, because a
// section whose verbs the reader holds carries them inside the same summary —
// so an exact match would find the section only while the capability probe was
// still in flight.
function section(heading: string): HTMLElement {
  const titles = document.querySelectorAll<HTMLElement>(
    ".pe-rail > .panel > .panel-head h2",
  );
  for (const title of titles) {
    if ((title.textContent ?? "").startsWith(heading)) {
      const panel = title.closest<HTMLElement>(".panel");
      if (panel) {
        return panel;
      }
    }
  }
  throw new Error(`the rail drew no section headed "${heading}"`);
}

const WITHHELD_SENTENCE = "Hidden for your role";

// contactStanding is the one line of relationship standing the record's head
// carries: a pure derivation from the two touch timestamps, tested directly
// rather than through a mount: the words and the tone are what the head
// renders, and nothing about drawing them belongs to this rail any more.
describe("contactStanding", () => {
  const t: Translator = (key, params) => translate("en", key, params);

  it("reads a two-way relationship as strong, with the trend of its last touch", () => {
    const view: Contact360 = {
      ...emptyButGranted,
      last_inbound_at: daysAgo(3),
      last_outbound_at: daysAgo(8),
    };
    expect(contactStanding(view, t)).toEqual({
      words: "Strong · warming",
      tone: "success",
    });
  });

  it("reads a silence past the quiet threshold as at risk", () => {
    // 40 days outlasts contactquiet.ts's QUIET_AFTER_DAYS (14), so this is
    // unambiguously past the line rather than pinned to it.
    const view: Contact360 = {
      ...emptyButGranted,
      last_inbound_at: daysAgo(40),
    };
    const standing = contactStanding(view, t);
    expect(standing?.tone).toBe("danger");
    expect(standing?.words.startsWith("At risk")).toBe(true);
  });

  it("calls a contact nobody has ever heard from thin, with no trend to state", () => {
    const view: Contact360 = {
      ...emptyButGranted,
      last_inbound_at: null,
      last_outbound_at: null,
    };
    expect(contactStanding(view, t)).toEqual({
      words: "Thin",
      tone: undefined,
    });
  });

  it("gives no standing at all when the touch dates are withheld", () => {
    expect(contactStanding(withheld, t)).toBeNull();
  });
});

describe("the rail leaves conversation history in the overview and Timeline", () => {
  it.each([emptyButGranted, withheld, granted])(
    "does not duplicate recent activity",
    (view) => {
      mount(view);
      expect(
        screen.queryByRole("heading", { name: "Recent activity" }),
      ).toBeNull();
    },
  );
});

describe("the sibling sections governed by their own grants", () => {
  it("says the employments are withheld rather than that there are none", () => {
    mount(withheld);

    expect(
      within(section("Companies")).getByText(WITHHELD_SENTENCE),
    ).toBeTruthy();
  });

  it("lists the employer a reader may see", () => {
    mount(granted);

    const companies = section("Companies");
    expect(within(companies).getByText("Brandt Automotive GmbH")).toBeTruthy();
    expect(within(companies).queryByText(WITHHELD_SENTENCE)).toBeNull();
  });

  it("shows the employer's contact and open-deal counts once its record answers", async () => {
    mount(granted);

    const companies = section("Companies");
    await within(companies).findByText("2 contacts · no open deals");
  });
});

// What the modal SENDS, and whether the box the reader looked at agreed with it.
// The two must never part company. A modal that omits the field for a box the
// reader never touched leaves the server to decide, and the server's answer can
// be the opposite of the unticked box on screen — leaving no way to say "no"
// except ticking and unticking again. So the box states an answer from the
// moment it opens, and the answer it states is the one the record takes.
describe("adding a company", () => {
  // Its own instance per test, and told about the fake timers this suite
  // installs: a shared one carries pointer and keyboard state between tests, so
  // the second test to run would inherit the first one's.
  function driver() {
    return userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  }

  async function openAndPickEmployer(user: ReturnType<typeof driver>) {
    await user.click(screen.getByRole("button", { name: "Add company" }));
    await user.type(screen.getByRole("searchbox"), "emp");
    await user.click(await screen.findByText("Employer GmbH"));
  }

  // `.checked` directly rather than a jest-dom matcher: this suite does not
  // install them, and the assertion is about the box's own state.
  //
  // NARROWED, not asserted. `as HTMLInputElement` would be the unchecked cast T6
  // forbids, and here it would also lie usefully: a control that stopped being an
  // <input> would read `undefined`, which is falsy, so every "not ticked"
  // assertion below would keep passing over a checkbox that no longer exists.
  function currentEmployerTicked(): boolean {
    const box = screen.getByRole("checkbox", {
      name: "This is their current employer",
    });
    if (!(box instanceof HTMLInputElement)) {
      throw new Error(
        `the current-employer control is a ${box.tagName}, not an input`,
      );
    }
    return box.checked;
  }

  // Narrowed, not asserted: the recorded body is `unknown` because it came off
  // the wire, and casting it would be this suite claiming a shape it has not
  // checked — which is the whole class of thing these tests exist to catch.
  function asObject(body: unknown, t: string): Record<string, unknown> {
    if (typeof body !== "object" || body === null || Array.isArray(body)) {
      throw new Error(`${t} was not a JSON object: ${JSON.stringify(body)}`);
    }
    return { ...body };
  }

  async function employmentBody(): Promise<Record<string, unknown>> {
    // The mutation is in flight when the click returns, so the assertion waits
    // for the request rather than for anything the modal renders — what was
    // SENT is the only thing under test here.
    await waitFor(() => {
      if (!sent.some((request) => request.path.endsWith("/relationships"))) {
        throw new Error(
          `no employment was posted; requests were ${JSON.stringify(sent)}`,
        );
      }
    });
    const post = sent.find(
      (request) =>
        request.method === "POST" && request.path.endsWith("/relationships"),
    );
    return asObject(post?.body, "the posted employment");
  }

  it("starts ticked for somebody with no current job, and sends that", async () => {
    const user = driver();
    mount(emptyButGranted);
    await screen.findByRole("button", { name: "Add company" });
    await user.click(screen.getByRole("button", { name: "Add company" }));

    // The state the reader sees BEFORE touching anything — the reading that a
    // modal deciding the field only on interaction cannot honour.
    expect(currentEmployerTicked()).toBe(true);

    await user.type(screen.getByRole("searchbox"), "emp");
    await user.click(await screen.findByText("Employer GmbH"));
    await user.click(screen.getByRole("button", { name: "Create" }));

    expect((await employmentBody()).is_current_primary).toBe(true);
  });

  it("lets the reader say no, and sends that", async () => {
    const user = driver();
    mount(emptyButGranted);
    await screen.findByRole("button", { name: "Add company" });
    await openAndPickEmployer(user);
    await user.click(
      screen.getByRole("checkbox", { name: "This is their current employer" }),
    );
    expect(currentEmployerTicked()).toBe(false);
    await user.click(screen.getByRole("button", { name: "Create" }));

    // `false`, not absent. A reader who unticks a box has decided, and the
    // server must not derive over them.
    const body = await employmentBody();
    expect("is_current_primary" in body).toBe(true);
    expect(body.is_current_primary).toBe(false);
  });

  it("re-takes the default every time it opens, not once", async () => {
    // `useState` reads its initializer ONCE and this modal never remounts, so a
    // default taken at first render answers a question about the rows as they
    // were then. Open it for somebody who has an employer (unticked), close it,
    // and reopen after that employer is gone: the box has to be ticked, or the
    // save states a `false` about a contact with no current job — the very thing
    // the default exists to prevent.
    const user = driver();
    const { rerender } = mount(granted);
    await screen.findByRole("button", { name: "Add company" });
    await user.click(screen.getByRole("button", { name: "Add company" }));
    expect(currentEmployerTicked()).toBe(false);
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    rerender(emptyButGranted);
    await user.click(screen.getByRole("button", { name: "Add company" }));
    expect(currentEmployerTicked()).toBe(true);
  });

  // The start date is the field #415 is about: an employment with no window
  // lets the account walk resolve three-year-old mail to the company this
  // contact joined last month. This form is the only place a human is ever
  // asked for it, so what it SENDS is the assertion — and the month case is
  // the one that matters, because a reader who knows the month and not the day
  // must not have a day invented for them.
  it("sends the start date it was given, at the precision it was given", async () => {
    const user = driver();
    mount(emptyButGranted);
    await screen.findByRole("button", { name: "Add company" });
    await openAndPickEmployer(user);
    await user.type(screen.getByLabelText("Start date"), "2024-05");
    await user.click(screen.getByRole("button", { name: "Create" }));

    const body = await employmentBody();
    expect(body.started_at).toBe("2024-05-01");
    expect(body.started_precision).toBe("month");
  });

  it("says nothing about a start nobody gave", async () => {
    // Absent, not today. An unknown start is a real answer, and a form that
    // filled it in would write a fact nobody holds — onto the very column the
    // account walk trusts to bound a window.
    const user = driver();
    mount(emptyButGranted);
    await screen.findByRole("button", { name: "Add company" });
    await openAndPickEmployer(user);
    await user.click(screen.getByRole("button", { name: "Create" }));

    const body = await employmentBody();
    expect("started_at" in body).toBe(false);
    expect("started_precision" in body).toBe(false);
  });

  // The modal is never unmounted, so every field it holds survives a cancel
  // unless something puts it back. A start date is the worst one to leave
  // standing: it would be sent for the NEXT employer, dated from the last.
  it("does not carry a cancelled start date into the next employer", async () => {
    const user = driver();
    mount(emptyButGranted);
    await screen.findByRole("button", { name: "Add company" });
    await user.click(screen.getByRole("button", { name: "Add company" }));
    await user.type(screen.getByLabelText("Start date"), "2019-03-04");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await user.click(screen.getByRole("button", { name: "Add company" }));
    expect(screen.getByLabelText("Start date")).toHaveProperty("value", "");
  });

  // `datePatch` appends `-01` to any seven characters, so an unchecked field
  // turns a typo into a plausible-looking day — and the wire declares `date`,
  // which refuses the rest outright. The form says no before either happens.
  it("refuses to save a date that is not one", async () => {
    const user = driver();
    mount(emptyButGranted);
    await screen.findByRole("button", { name: "Add company" });
    await openAndPickEmployer(user);
    await user.type(screen.getByLabelText("Start date"), "not-a-date");

    expect(screen.getByRole("button", { name: "Create" })).toHaveProperty(
      "disabled",
      true,
    );
  });

  it("starts unticked for somebody who already has a current job", async () => {
    // The other half of the default, and the reason it is read off the rows
    // rather than hardcoded: which of two employers is the main one is not
    // this modal's to assume.
    const user = driver();
    mount(granted);
    await screen.findByRole("button", { name: "Add company" });
    await user.click(screen.getByRole("button", { name: "Add company" }));

    expect(currentEmployerTicked()).toBe(false);
  });
});

// The rail sorts the employer somebody still holds to the top and marks it
// current, and "ending" a job writes today as its last day. Both readings turn
// on the same rule as the server's: a job is still theirs until its last day has
// PASSED, so a notice period is not a departure and today's date is.
describe("which employer is the current one", () => {
  function withEmployments(
    rows: Array<{
      relationship_id: string;
      company_name: string;
      is_current_primary: boolean;
      ended_at: string | null;
    }>,
  ): Contact360 {
    return {
      ...granted,
      employments: {
        data: rows.map((row) => ({
          relationship_id: row.relationship_id,
          company_id: `o-${row.relationship_id}`,
          company_name: row.company_name,
          role: "Head of Fleet",
          is_current_primary: row.is_current_primary,
          started_at: "2022-03-01T00:00:00Z",
          ended_at: row.ended_at,
        })),
        page,
      },
    };
  }

  // NOW is pinned to 2026-08-18, so these are a fortnight either side of it.
  const future = "2026-09-01";
  const past = "2026-08-04";

  function employerOrder(): string[] {
    return Array.from(
      section("Companies").querySelectorAll(".pe-employment-company"),
    ).map((node) => node.textContent ?? "");
  }

  it("puts the job they still hold first, notice period and all", async () => {
    mount(
      withEmployments([
        {
          relationship_id: "rel-old",
          company_name: "Former GmbH",
          is_current_primary: false,
          ended_at: past,
        },
        {
          relationship_id: "rel-now",
          company_name: "Notice GmbH",
          is_current_primary: true,
          ended_at: future,
        },
      ]),
    );
    await screen.findByText("Notice GmbH");

    const [first, second] = employerOrder();
    expect(first).toContain("Notice GmbH");
    // Serving notice is still the current job, so the row says so.
    expect(first).toContain("Current");
    expect(second).toContain("Former GmbH");
    expect(second).not.toContain("Current");
  });

  it("stops calling it current once the last day has passed", async () => {
    // The flag still says this employer is the primary one — the record of WHICH
    // employer it was is not rewritten when the date passes. Currency is derived
    // from the date, so the marker goes even though the flag stayed.
    mount(
      withEmployments([
        {
          relationship_id: "rel-gone",
          company_name: "Former GmbH",
          is_current_primary: true,
          ended_at: past,
        },
        {
          relationship_id: "rel-older",
          company_name: "Earlier GmbH",
          is_current_primary: false,
          ended_at: past,
        },
      ]),
    );
    await screen.findByText("Former GmbH");

    // Nobody is current here, so the section marks nobody — a flag left over
    // from a job that has ended must not outrank a row that never claimed one.
    for (const row of employerOrder()) {
      expect(row).not.toContain("Current");
    }
  });

  it("ends an employment as of today, not tomorrow", async () => {
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
    mount(
      withEmployments([
        {
          relationship_id: "rel-now",
          company_name: "Brandt Automotive GmbH",
          is_current_primary: true,
          ended_at: null,
        },
      ]),
    );
    await screen.findByText("Brandt Automotive GmbH");

    await user.click(screen.getByRole("button", { name: "More actions" }));
    await user.click(screen.getByRole("button", { name: "Mark as ended" }));

    await waitFor(() => {
      expect(sent.some((request) => request.method === "PATCH")).toBe(true);
    });
    const patch = sent.find((request) => request.method === "PATCH");
    const body = patch?.body;
    if (typeof body !== "object" || body === null || Array.isArray(body)) {
      throw new Error(
        `the end-employment patch was not a JSON object: ${JSON.stringify(body)}`,
      );
    }
    // The LOCAL date, formatted by hand. Reaching for toISOString() here would
    // send yesterday's date to anybody west of UTC for most of their working day.
    expect((body as Record<string, unknown>).ended_at).toBe("2026-08-18");
  });
});

describe("how the contact is filed", () => {
  // The tags panel is shared with the company and deal pages, and it draws in
  // the rail's SECTION chrome here: the contact rail is one card of headed
  // sections, so a second card inside it would be a box in a box.
  it("draws the contact's tags in the rail", async () => {
    mount(granted);
    expect(await screen.findByText("Champion")).toBeTruthy();
  });
});

// A contact who still works somewhere and has no primary employer is a state
// the store reaches deliberately: a human clears the flag, or the primary is
// retired while two others remain and nothing picks between them on somebody's
// behalf. It used to arrive as a list with no "current" marker anywhere on it,
// which is indistinguishable from a value that failed to load.
describe("a contact with no primary employer", () => {
  const employment = (id: string, company: string, primary: boolean) => ({
    relationship_id: id,
    company_id: id,
    company_name: company,
    role: "Head of Fleet",
    is_current_primary: primary,
    started_at: "2022-03-01T00:00:00Z",
    ended_at: null,
  });
  const withEmployments = (
    rows: ReturnType<typeof employment>[],
  ): Contact360 => ({ ...granted, employments: { data: rows, page } });

  it("says so, and says it is a choice, when two are held and neither leads", async () => {
    mount(
      withEmployments([
        employment("rel-1", "Brandt Automotive GmbH", false),
        employment("rel-2", "Kessler Logistik AG", false),
      ]),
    );
    expect(
      await screen.findByText(en["contact.rail.noPrimaryEmployer"]),
    ).toBeTruthy();
  });

  it("stays quiet when one of them leads", async () => {
    mount(
      withEmployments([
        employment("rel-1", "Brandt Automotive GmbH", true),
        employment("rel-2", "Kessler Logistik AG", false),
      ]),
    );
    await screen.findByText("Brandt Automotive GmbH");
    expect(screen.queryByText(en["contact.rail.noPrimaryEmployer"])).toBeNull();
  });

  // The empty state is a different sentence about a different fact — nobody has
  // recorded where this contact works — and asking for a choice there would be
  // asking them to choose between nothing.
  it("leaves the empty state to say it when there are no employments", async () => {
    mount(emptyButGranted);
    expect(
      await screen.findByText(en["contact.rail.noEmployment"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["contact.rail.noPrimaryEmployer"])).toBeNull();
  });
});

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { SearchScreen } from "./search";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

// One result row, so a claim about what a hit does NOT print is made against
// the row rather than against the whole page — the group card and the search
// field are not places a badge or a figure would have appeared.
function hitRow(container: HTMLElement): HTMLElement {
  const row = container.querySelector<HTMLElement>(".search-hit");
  if (!row) {
    throw new Error("the results rendered no hit row at all");
  }
  return row;
}

// One email hit, as the server sends it: the canonical row rides the hit and
// the raw `snippet` is what the row REPLACES.
const emailHit = {
  type: "activity",
  id: "a1",
  title: "Rennsteig renewal terms",
  snippet: "…a raw body slice the canonical row replaces…",
  score: 0.88,
  trust_tier: "authoritative",
  email_summary: {
    activity_id: "a1",
    occurred_at: "2026-09-01T09:15:00Z",
    version: 3,
    subject: "Rennsteig renewal terms",
    preview: "The quote holds until Friday.",
    counterparty: "Dana Buyer",
    direction: "inbound",
    display_status: "team",
    move: "needs_reply",
    attachment_count: 1,
  },
};

// The detail read, as the contract shapes it: the parties are flat on the
// presentation and the access block is required. A fixture that omitted them
// would model a response the server cannot send, and the drawer would throw on
// a shape no user will ever meet.
const emailPresentation = {
  id: "a1",
  lifecycle: "delivered",
  occurred_at: "2026-09-01T09:15:00Z",
  summary: emailHit.email_summary,
  body: "The quote holds until Friday.",
  thread_key: "t1",
  from: [{ address: "dana@acme.test", display_name: "Dana Buyer" }],
  to: [{ address: "rep@demo.test", display_name: "Lena Fischer" }],
  cc: [],
  bcc: [],
  bcc_withheld: false,
  attachments: [],
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
};

describe("SearchScreen", () => {
  // An email hit is the canonical row, not a title and a raw body slice. The
  // subject, the preview the server composed and the access badge all come
  // from `email_summary`, and the row opens the same drawer every timeline
  // opens.
  it("renders an email hit as the canonical row and opens the drawer", async () => {
    // The URL off the Request, not String(input): openapi-fetch hands fetch a
    // Request object, whose String() is "[object Request]" and matches nothing.
    const urlOf = (input: RequestInfo | URL) =>
      input instanceof Request ? input.url : String(input);
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (urlOf(input).includes("email-presentation")) {
        return jsonResponse(emailPresentation);
      }
      return jsonResponse({
        data: [emailHit],
        page: { next_cursor: null, has_more: false },
      });
    });
    vi.stubGlobal("fetch", fetchMock);
    render(<SearchScreen q="renewal" />);

    await waitFor(() =>
      expect(screen.getByText("Rennsteig renewal terms")).toBeTruthy(),
    );
    // The server's preview, not the raw `snippet`: the snippet is a
    // 200-character slice of the body and drawing both would put two readings
    // of one message on one line.
    expect(screen.getByText("The quote holds until Friday.")).toBeTruthy();
    expect(screen.queryByText(/raw body slice/)).toBeNull();
    // The access badge rides the row, which is how a reader tells a limited
    // conversation from an open one without opening it.
    expect(screen.getByText("Team")).toBeTruthy();

    // Clicking the row opens the drawer over the results — which is search's
    // half of this. That it asks for THIS message's presentation is the claim;
    // what the drawer then draws is emaildetail's own contract.
    const row = screen.getByRole("button", { name: /Rennsteig renewal terms/ });
    expect(row.getAttribute("aria-haspopup")).toBe("dialog");
    await userEvent.click(row);
    await waitFor(() =>
      expect(
        fetchMock.mock.calls.some((call) =>
          urlOf(call[0]).includes("/activities/a1/email-presentation"),
        ),
      ).toBe(true),
    );
  });

  // A call, a note, a task and a meeting are activities too. The server sends
  // no `email_summary` for them and the row keeps the generic treatment it has
  // always had — the failure this catches is a screen branching on the KIND
  // word rather than on the field.
  it("leaves a non-email activity hit on its generic treatment", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              type: "activity",
              id: "a2",
              title: "Rennsteig kickoff call",
              snippet: "…what we agreed on the call…",
              score: 0.6,
              trust_tier: "authoritative",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<SearchScreen q="rennsteig" />);

    await waitFor(() =>
      expect(screen.getByText("Rennsteig kickoff call")).toBeTruthy(),
    );
    expect(screen.getByText(/what we agreed on the call/)).toBeTruthy();
    // Nothing openable: a row that looks like it opens a message and does not
    // teaches a reader the product is broken.
    expect(screen.queryByRole("button", { name: /kickoff call/ })).toBeNull();
  });

  it("groups hits by type and shows the snippet", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              type: "person",
              id: "p1",
              title: "Dana Buyer",
              snippet: "…Dana at Acme…",
              score: 0.91,
              trust_tier: "authoritative",
            },
            {
              type: "deal",
              id: "d1",
              title: "Acme expansion",
              snippet: "…platform…",
              score: 0.74,
              trust_tier: "authoritative",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<SearchScreen q="acme" />);
    // By ROLE, not by text: the type filter above the results names the same
    // groups the headings do, so a bare getByText("People") matches the pill —
    // which renders before the read settles, and would wait out nothing.
    await waitFor(() =>
      expect(screen.getByRole("heading", { name: "People" })).toBeTruthy(),
    );
    expect(screen.getByRole("heading", { name: "Deals" })).toBeTruthy();
    expect(screen.getByText(/Dana at Acme/)).toBeTruthy();
    // The hit title renders straight from the search result (no per-hit
    // record fetch) as a link to the record's 360.
    const hitLink = screen.getByText("Dana Buyer");
    expect(hitLink.tagName).toBe("BUTTON");
    expect(hitLink.className).toContain("entity-link");
  });

  // A stored record is `authoritative` in native mode — every one of them — so
  // a badge for that tier put the same pill on every row on the page and told a
  // reader nothing they did not already assume. The score is not drawn either:
  // the contract bounds it to nothing, so the retriever's raw figure reached the
  // page as a percentage over 100 that a reader can neither act on nor doubt.
  it("prints neither a tier badge nor a relevance figure for a stored record", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              type: "person",
              id: "p1",
              title: "Dana Buyer",
              snippet: "…Dana at Acme…",
              // Past 1, the way the seeded retriever actually scores: a hit that
              // renders its score as a percentage reads "relevance 280%" here.
              score: 2.8,
              trust_tier: "authoritative",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    const { container } = render(<SearchScreen q="acme" />);
    await waitFor(() => expect(screen.getByText("Dana Buyer")).toBeTruthy());
    const row = hitRow(container);
    // What the row still carries: the name and the matched text.
    expect(row.textContent).toContain("Dana at Acme");
    expect(row.querySelectorAll(".badge")).toHaveLength(0);
    expect(screen.queryByText("verified")).toBeNull();
    expect(row.textContent).not.toContain("%");
    expect(row.textContent).not.toMatch(/relevance/i);
  });

  it("badges an external-tier hit as mirrored", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              type: "person",
              id: "p1",
              title: "Dana Buyer",
              trust_tier: "external",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    const { container } = render(<SearchScreen q="acme" />);
    await waitFor(() => expect(screen.getByText("Dana Buyer")).toBeTruthy());
    // The tier covers every overlay and connector source, so the badge names
    // none of them: a hit carries no provider field, and a vendor name here
    // would be stamped on rows mirrored from a different system.
    expect(screen.getByText("from a connected system")).toBeTruthy();
    expect(screen.queryByText(/HubSpot/)).toBeNull();
    // authoritative's badge never renders alongside a mirrored hit.
    expect(screen.queryByText("verified")).toBeNull();
    // And it is the ONLY badge on the row: the mirrored mark is the one that
    // varies between hits, so a second pill beside it is what buried it.
    expect(hitRow(container).querySelectorAll(".badge")).toHaveLength(1);
  });

  // A tier the record CARRIES and the page does not draw reads as a record with
  // nothing to declare, which is the opposite of what unverified means.
  it("badges an unverified hit rather than leaving it unmarked", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              type: "person",
              id: "p2",
              title: "Sam Unknown",
              trust_tier: "unverified",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<SearchScreen q="acme" />);
    await waitFor(() => expect(screen.getByText("Sam Unknown")).toBeTruthy());
    expect(screen.getByText("unverified")).toBeTruthy();
    expect(screen.queryByText("verified")).toBeNull();
  });

  it("shows an honest empty state", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<SearchScreen q="zzz" />);
    await waitFor(() => expect(screen.getByText(/No matches/)).toBeTruthy());
  });
  // Finding the WORD is the step before finding the records carrying it, so a
  // tag hit has to be openable. It shipped as plain text — the group rendered,
  // and the result was a dead end.
  it("opens the tag page from a tag hit, and says what the word is on", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              type: "tag",
              id: "01a05ebd-b03d-7183-b2fb-c00bcb58b419",
              title: "Key Account",
              snippet: null,
              score: 2,
              carried_by: 7,
              trust_tier: "authoritative",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<SearchScreen q="key" />);

    const hit = await screen.findByText("Key Account");
    expect(hit.tagName).toBe("BUTTON");
    expect(screen.getByText("On 7 records")).toBeTruthy();

    await userEvent.setup().click(hit);
    expect(window.location.hash).toBe(
      "#/tags/01a05ebd-b03d-7183-b2fb-c00bcb58b419",
    );
  });

  // Absent is not zero. A server that sent no number has not said the word is
  // unused, and printing "On 0 records" would be a claim nobody made.
  it("prints no count when the answer carried none", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              type: "tag",
              id: "01a05ebd-b03d-7183-b2fb-c00bcb58b419",
              title: "Key Account",
              snippet: null,
              score: 2,
              trust_tier: "authoritative",
            },
          ],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<SearchScreen q="key" />);

    expect(await screen.findByText("Key Account")).toBeTruthy();
    expect(screen.queryByText(/On \d+ records/)).toBeNull();
  });
});

// The results screen dropped `project` hits for months: the server ranked and
// returned them, and the group list here — a hand-kept literal — never named
// the type, so they were filtered out on the way to the screen. Both new
// catalog types would have landed the same way. The list is derived now, and
// this is the assertion that says so for every member of it.
describe("SearchScreen — every hit type the contract can return", () => {
  const KINDS = [
    { type: "person", heading: "People" },
    { type: "organization", heading: "Organizations" },
    { type: "deal", heading: "Deals" },
    { type: "project", heading: "Projects" },
    { type: "product", heading: "Products" },
    { type: "offer_template", heading: "Offer templates" },
    { type: "lead", heading: "Leads" },
    { type: "tag", heading: "Tags" },
  ] as const;

  it.each(KINDS)(
    "groups a $type hit under $heading",
    async ({ type, heading }) => {
      vi.stubGlobal(
        "fetch",
        vi.fn(async () =>
          jsonResponse({
            data: [
              {
                type,
                id: "01a05ebd-b03d-7183-b2fb-c00bcb58b419",
                title: "Findable thing",
                score: 0.5,
                trust_tier: "authoritative",
              },
            ],
            page: { next_cursor: null, has_more: false },
          }),
        ),
      );
      render(<SearchScreen q="findable" />);
      expect(
        await screen.findByRole("heading", { name: heading }),
      ).toBeTruthy();
      expect(screen.getByText("Findable thing")).toBeTruthy();
    },
  );
});

describe("SearchScreen — narrowing by type", () => {
  // The pills are a SERVER dial: they put `types` on the wire rather than
  // hiding rows already drawn, so a narrowed search is a different answer and
  // not a smaller view of the same one.
  it("sends the narrowed type to the server and puts it in the address", async () => {
    const user = userEvent.setup();
    const fetchMock = vi.fn(async (_input: RequestInfo | URL) =>
      jsonResponse({
        data: [],
        page: { next_cursor: null, has_more: false },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<SearchScreen q="acme" />);

    await user.click(screen.getByRole("button", { name: "Products" }));

    await waitFor(() => {
      // openapi-fetch hands fetch a Request, so the URL is on the object rather
      // than being the first argument.
      const asked = fetchMock.mock.calls.map(([input]) =>
        input instanceof Request ? input.url : String(input),
      );
      expect(asked.some((url) => url.includes("types=product"))).toBe(true);
    });
    expect(globalThis.location.hash).toContain("type=product");
  });

  // The default is spelled by ABSENCE, so one view has exactly one address and
  // "everything" cannot be reached by two different links.
  it("clears the parameter rather than naming the default", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<SearchScreen q="acme" />);

    await user.click(screen.getByRole("button", { name: "Products" }));
    await waitFor(() =>
      expect(globalThis.location.hash).toContain("type=product"),
    );
    await user.click(screen.getByRole("button", { name: "Everything" }));
    await waitFor(() =>
      expect(globalThis.location.hash).not.toContain("type="),
    );
  });

  // A narrowing that found nothing keeps the control that got the reader there.
  // Losing it would leave them on an empty page with no way back but the
  // address bar.
  it("keeps the pills when the narrowed search finds nothing", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [],
          page: { next_cursor: null, has_more: false },
        }),
      ),
    );
    render(<SearchScreen q="zzz" />);
    expect(await screen.findByText(/No matches/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Everything" })).toBeTruthy();
  });
});

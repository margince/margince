/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent, { type UserEvent } from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { activityTimeline } from "../design-system/activitytimeline";
import { LocaleProvider } from "../i18n";
import { ContactsScreen } from "./contacts";

// B-EP09.10a acceptance: per-row provenance chips, row→360 navigation, and
// the honest error state. Lead-specific acceptance (score thresholds,
// promote eligibility, the §3.5 segregated LeadsScreen/LeadScreen) lives in
// leads.test.tsx.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function jsonResponse(body: unknown, status = 200) {
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

// What a dropdown offers, in order. The options live in a portalled popup that
// only exists while the control is open, and only one popup is open at a time —
// so a list is read by opening its own control and closing it again, which is
// also what lets two Type selects on the same form be compared.
async function optionTextOf(user: UserEvent, control: HTMLElement) {
  await user.click(control);
  const text = within(screen.getByRole("listbox"))
    .getAllByRole("option")
    .map((option) => option.textContent);
  // Closed by the trigger, not by Escape: these controls sit inside a modal,
  // and Escape reaches the dialog too.
  await user.click(control);
  return text;
}

const anna = {
  id: "p-1",
  full_name: "Anna Weber",
  title: "Head of Procurement",
  emails: [{ id: "e-1", email: "anna.weber@brandt.example", is_primary: true }],
  captured_by: "connector:gmail",
  source: "gmail",
  version: 1,
};

describe("ContactsScreen (B-EP09.10a)", () => {
  it("names the owner on each row and navigates to the contact 360", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        if (request.url.includes("/users")) {
          return jsonResponse({
            data: [
              { id: "u-9", email: "lena@x.test", display_name: "Lena F." },
            ],
            page: { next_cursor: null },
          });
        }
        return jsonResponse({
          data: [{ ...anna, owner_id: "u-9" }],
          page: { next_cursor: null },
        });
      }),
    );
    render(<ContactsScreen />);
    await waitFor(() => expect(screen.getByText("Anna Weber")).toBeTruthy());
    // The owner column answers "whose record is this?". The column it replaced
    // rendered "typed by a person" for every human-captured row — the same
    // string for every colleague, which named nobody.
    await waitFor(() => expect(screen.getByText("Lena F.")).toBeTruthy());
    await userEvent.click(screen.getByText("Anna Weber"));
    expect(window.location.hash).toBe("#/contacts/p-1");
  });

  it("says a row is unassigned rather than leaving the owner blank", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({ data: [anna], page: { next_cursor: null } }),
      ),
    );
    render(<ContactsScreen />);
    await waitFor(() => expect(screen.getByText("Anna Weber")).toBeTruthy());
    // Unowned is a fact with its own filter, not an absence: a blank cell
    // reads as "not loaded yet".
    expect(screen.getByText("Unassigned")).toBeTruthy();
  });

  it("names the company each contact works at, from the row alone", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        urls.push(request.url);
        return jsonResponse({
          data: [
            {
              ...anna,
              employer: {
                company_id: "o-1",
                company_name: "Brandt AG",
              },
            },
            { ...anna, id: "p-2", full_name: "Bruno Klein" },
          ],
          page: { next_cursor: null },
        });
      }),
    );
    render(<ContactsScreen />);
    await waitFor(() => expect(screen.getByText("Brandt AG")).toBeTruthy());
    // The name rides on the contact row, so the column costs no second read:
    // one company per contact would otherwise be one fetch per row.
    expect(urls.filter((url) => url.includes("/companies"))).toHaveLength(0);
    // A contact whose employer the wire withheld — no edge grant, no grant on
    // that account, or nobody has recorded one — states nothing. A dash would
    // read as "works nowhere", which is the one thing an absent field does
    // not say.
    const bruno = screen.getByText("Bruno Klein").closest("tr");
    expect(bruno?.textContent).not.toContain("Brandt AG");
    expect(bruno?.textContent).not.toContain("—");
  });

  // The row opens the CONTACT; the company cell opens the COMPANY. Two
  // destinations in one row, and the reader picks.
  //
  // The company used to be plain text, on the reasoning that the row is
  // already a link — but it is a link to the contact, so a reader scanning who
  // works for whom had to open a contact to reach the account behind them.
  it("sends the row and the company to different records", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse({
          data: [
            {
              ...anna,
              employer: {
                company_id: "o-1",
                company_name: "Brandt AG",
              },
            },
          ],
          page: { next_cursor: null },
        }),
      ),
    );
    render(<ContactsScreen />);

    const company = await screen.findByRole("link", { name: "Brandt AG" });
    expect(company.getAttribute("href")).toBe("#/companies/o-1");
    // The row's own identity link still goes to the contact. Asserting BOTH is
    // what says these are two destinations rather than one of them having
    // quietly taken the other's place.
    const links = await screen.findAllByRole("link");
    const hrefs = links.map((each) => each.getAttribute("href"));
    expect(hrefs).toContain("#/contacts/p-1");
  });

  it("renders the honest error state with the RFC7807 detail", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Forbidden",
            detail: "missing scope contacts:read",
          },
          403,
        ),
      ),
    );
    render(<ContactsScreen />);
    await waitFor(() =>
      expect(screen.getByText("Could not load this view. Reload the page.")).toBeTruthy(),
    );
    expect(screen.getByText("missing scope contacts:read")).toBeTruthy();
  });
});

// The dormant/no-interactions strength response — the default backstop for
// every stubFetch call below that isn't itself exercising the strength card
// (P-4): the Contact Overview now fires this GET unconditionally, and none of
// those pre-existing tests care about its shape, so they get an honest
// zero/dormant reading rather than a mismatched shape from the contact-fixture
// catch-all.
const dormantStrength = {
  score: 0,
  bucket: "none",
  factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
  last_interaction: null,
};

// A URL-capturing fetch stub shared across the P-14/15/16 wiring tests
// below: every request is recorded so a test can assert the params it
// carried, and a caller-supplied responder decides what comes back. Strength
// requests are answered with the dormant default up front (overridable via
// `strength`) so tests that don't care about relationship strength don't have
// to plumb a branch for it.
function stubFetch(
  responder: (
    url: string,
    method: string,
    request: Request,
  ) => Promise<Response>,
  options?: Readonly<{
    strength?: unknown;
    // The deals this contact sits on and the buying role they hold on each —
    // the identity rail's own section, empty for every test that isn't about it.
    dealRoles?: readonly components["schemas"]["Contact360DealRole"][];
  }>,
): { fetchMock: ReturnType<typeof vi.fn>; urls: string[] } {
  const urls: string[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    urls.push(request.url);
    const pathname = new URL(request.url).pathname;
    if (pathname.endsWith("/strength")) {
      return jsonResponse(options?.strength ?? dormantStrength);
    }
    if (pathname.endsWith("/360")) {
      return jsonResponse({
        as_of: "2026-08-04T09:00:00Z",
        contact: anna,
        sections_omitted: [],
        strength: options?.strength ?? dormantStrength,
        last_inbound_at: "2026-07-01T09:00:00Z",
        last_outbound_at: "2026-06-20T09:00:00Z",
        network: { colleagues: [] },
        deal_roles: {
          data: options?.dealRoles ?? [],
          page: { next_cursor: null, has_more: false },
        },
      });
    }
    if (pathname.endsWith("/context")) {
      return jsonResponse({
        anchor: { type: "contact", id: "p-1" },
        sections: [],
      });
    }
    const answer = await responder(request.url, request.method, request);
    // The record's verbs and the relationships panel ask the grant before
    // they draw, so a responder that never named a session gets one holding
    // what a rep working their own contacts holds. A spec that answers /me
    // itself — a refusal, say — is passed through untouched.
    if (pathname.endsWith("/me") && answer.ok) {
      const body: unknown = await answer.clone().json();
      if (typeof body === "object" && body !== null && "user" in body) {
        return answer;
      }
      return jsonResponse(
        meFixture({
          allow: {
            contact: ["read", "create", "update", "delete"],
            relationship: ["read", "create", "update", "delete"],
            activity: ["read", "create"],
          },
        }),
      );
    }
    return answer;
  });
  vi.stubGlobal("fetch", fetchMock);
  return { fetchMock, urls };
}

function emptyPage() {
  return jsonResponse({
    data: [],
    page: { next_cursor: null, has_more: false },
  });
}

describe("ContactsScreen — search/sort/pagination (P-14)", () => {
  it("carries the debounced search term into the next fetch", async () => {
    const { urls } = stubFetch(async () => emptyPage());
    render(<ContactsScreen />);
    await waitFor(() => expect(urls.length).toBeGreaterThan(0));

    vi.useFakeTimers();
    try {
      fireEvent.change(screen.getByPlaceholderText("Search"), {
        target: { value: "anna" },
      });
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }

    await waitFor(() =>
      expect(urls.some((url) => url.includes("q=anna"))).toBe(true),
    );
  });

  it("fetches the next cursor page when the pager steps past the loaded page", async () => {
    const { urls } = stubFetch(async (url) => {
      if (url.includes("cursor=c1")) {
        return jsonResponse({
          data: [{ ...anna, id: "p-2", full_name: "Otto Fischer" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      return jsonResponse({
        data: [anna],
        page: { next_cursor: "c1", has_more: true },
      });
    });
    render(<ContactsScreen />);
    await waitFor(() => expect(screen.getByText("Anna Weber")).toBeTruthy());

    const next = screen.getByRole("button", { name: "Next ›" });
    expect((next as HTMLButtonElement).disabled).toBe(false);
    await userEvent.click(next);

    await waitFor(() => expect(screen.getByText("Otto Fischer")).toBeTruthy());
    expect(urls.some((url) => url.includes("cursor=c1"))).toBe(true);
  });
});

describe("ContactsScreen — rich create (P-15)", () => {
  it("shows repeatable emails/phones, title, and a linkedin field", async () => {
    stubFetch(async () => emptyPage());
    render(<ContactsScreen />);
    await userEvent.click(screen.getByTestId("new-record"));
    expect(screen.getByLabelText("Title")).toBeTruthy();
    expect(screen.getByLabelText("LinkedIn")).toBeTruthy();
    expect(screen.getByText("Add email")).toBeTruthy();
    expect(screen.getByText("Add phone")).toBeTruthy();
  });

  // Regression: the email/phone "Type" select's options are keyed messages
  // (field.emailWork/…) resolved by contactCreateFields via useT() — the
  // rendered option text must be the translated word, never the raw
  // MessageKey string (fieldControl in create.tsx renders option.label
  // verbatim, so an untranslated key would leak straight to the DOM).
  it("shows translated Type option text, not the raw i18n key", async () => {
    const user = userEvent.setup();
    stubFetch(async () => emptyPage());
    render(<ContactsScreen />);
    await user.click(screen.getByTestId("new-record"));
    await user.click(screen.getByText("Add email"));
    await user.click(screen.getByText("Add phone"));
    const [emailType, phoneType] = screen.getAllByLabelText("Type");

    const emailOptionText = await optionTextOf(user, emailType);
    expect(emailOptionText).toEqual(["Not set", "Work", "Personal", "Other"]);
    expect(emailOptionText).not.toContain("field.emailWork");

    const phoneOptionText = await optionTextOf(user, phoneType);
    expect(phoneOptionText).toEqual([
      "Not set",
      "Work",
      "Mobile",
      "Home",
      "Other",
    ]);
    expect(phoneOptionText).not.toContain("field.phoneWork");
  });

  it("shows German Type option text under the de locale", async () => {
    const user = userEvent.setup();
    stubFetch(async () => emptyPage());
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="de">
          <ContactsScreen />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await user.click(screen.getByTestId("new-record"));
    await user.click(screen.getByText("E-Mail hinzufügen"));
    const optionText = await optionTextOf(user, screen.getByLabelText("Typ"));
    expect(optionText).toEqual([
      "Nicht gesetzt",
      "Geschäftlich",
      "Privat",
      "Sonstige",
    ]);
  });

  it("posts full_name + emails + source:manual on submit", async () => {
    let posted: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/contacts")) {
        posted = JSON.parse(await request.text());
        return jsonResponse({ ...anna, id: "p-new" }, 201);
      }
      return emptyPage();
    });
    render(<ContactsScreen />);
    await userEvent.click(screen.getByTestId("new-record"));
    await userEvent.type(screen.getByLabelText("Full name *"), "Otto Fischer");
    await userEvent.click(screen.getByText("Add email"));
    await userEvent.type(screen.getByLabelText("Email *"), "otto@example.test");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toMatchObject({
      full_name: "Otto Fischer",
      source: "manual",
      emails: [
        {
          email: "otto@example.test",
          email_type: "work",
          is_primary: false,
          position: 0,
        },
      ],
    });
  });
});

describe("ContactsScreen — archived marking (P-3)", () => {
  it("shows an Archived badge on a row with archived_at set", async () => {
    stubFetch(async () =>
      jsonResponse({
        data: [{ ...anna, archived_at: "2026-07-01T00:00:00Z" }],
        page: { next_cursor: null, has_more: false },
      }),
    );
    render(<ContactsScreen />);
    await waitFor(() => expect(screen.getByText("Anna Weber")).toBeTruthy());
    expect(screen.getByText("Archived")).toBeTruthy();
  });
});

describe("ContactsScreen — dedupe view-existing link (P-16)", () => {
  it("renders a link to the collided record on a duplicate_email 409", async () => {
    stubFetch(async (url, method) => {
      if (method === "POST" && url.includes("/contacts")) {
        return jsonResponse(
          {
            type: "about:blank",
            title: "Conflict",
            detail: "email already in use",
            code: "duplicate_email",
            details: { existing_id: "01X" },
          },
          409,
        );
      }
      return emptyPage();
    });
    render(<ContactsScreen />);
    await userEvent.click(screen.getByTestId("new-record"));
    await userEvent.type(screen.getByLabelText("Full name *"), "Dup Contact");
    await userEvent.click(screen.getByText("Add email"));
    await userEvent.type(screen.getByLabelText("Email *"), "dup@example.test");
    await userEvent.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() =>
      expect(screen.getByText("View existing record")).toBeTruthy(),
    );
    await userEvent.click(screen.getByText("View existing record"));
    expect(window.location.hash).toBe("#/contacts/01X");
  });
});

describe("activityTimeline", () => {
  const base = {
    id: "a-1",
    occurred_at: "2026-07-31T10:00:00Z",
    captured_by: "connector:telegram",
    source: "telegram:1:2:3",
    created_at: "2026-07-31T10:00:00Z",
    updated_at: "2026-07-31T10:00:00Z",
  };

  it("shows a channel message's text, not the word 'telegram'", () => {
    const [entry] = activityTimeline([
      { ...base, kind: "telegram", body: "hello from a real human" } as never,
    ]);
    expect(entry.title).toBe("hello from a real human");
  });

  it("keeps a mail activity's subject rather than its body", () => {
    const [entry] = activityTimeline([
      {
        ...base,
        kind: "email",
        subject: "Quarterly review",
        body: "the whole message body",
      } as never,
    ]);
    expect(entry.title).toBe("Quarterly review");
  });

  it("collapses newlines and cuts a long message so the row stays one line", () => {
    const [entry] = activityTimeline([
      {
        ...base,
        kind: "telegram",
        body: `first line\n\nsecond ${"x".repeat(200)}`,
      } as never,
    ]);
    expect(entry.title).not.toContain("\n");
    expect(entry.title.length).toBeLessThanOrEqual(140);
    expect(entry.title.endsWith("…")).toBe(true);
  });

  it("falls back to the kind when an activity carries no text at all", () => {
    const [entry] = activityTimeline([
      { ...base, kind: "note", subject: null, body: null } as never,
    ]);
    expect(entry.title).toBe("note");
  });
});

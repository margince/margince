/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  company,
  company360,
  companyBackstop,
  emptySection,
  jsonResponse,
} from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import { CompanyScreen } from "./companies";
import { openProfile, render } from "./companies.testkit";
import { renderSuggestions } from "./company360.testkit";
import { AssistantPanel } from "./companyassistant";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

// One stalled-deal suggestion, as the 360 serves it. The reason is the part
// the rep judges, so the reason is what the tests assert on.
const stalledSuggestion = {
  kind: "stalled_deal",
  reason:
    '"Fleet retrofit 2026" has had no activity long enough to count as stalled.',
  fingerprint: "fp-stalled-1",
  subject_type: "deal",
  subject_id: "d-1",
  evidence: [{ entity_type: "deal", entity_id: "d-1" }],
};

// The suggestion rows, through the Tasks tab the record page mounts them on.
function renderSuggestionsFor(three60: unknown) {
  renderSuggestions(three60 as never);
}

// The Latest card's section and its rows, once the 360 has landed.
function latestCard(): HTMLElement | null {
  return (
    screen.queryByRole("heading", { name: "Latest" })?.closest("section") ??
    null
  );
}

async function latestRows(): Promise<HTMLElement[]> {
  await waitFor(() => expect(latestCard()?.querySelector("li")).toBeTruthy());
  return Array.from(latestCard()?.querySelectorAll("li") ?? []);
}

// The page's own Tasks tab, where the advice reads.
async function openTasks() {
  await userEvent.click(await screen.findByRole("button", { name: /^Tasks/ }));
}

describe("CompanyScreen — next-step suggestions", () => {
  it("leads each suggestion with the reason the rule fired, and cites the record", async () => {
    const three60 = { ...company360, suggestions: [stalledSuggestion] };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    expect(screen.getByText("Stalled deal")).toBeTruthy();
    // The evidence is reachable: a suggestion the rep cannot check is a
    // verdict. It sits behind the reason rather than beside it, so checking
    // costs one gesture and reading the advice costs none.
    await userEvent.click(screen.getByText(stalledSuggestion.reason));
    expect(screen.getByRole("button", { name: "deal" })).toBeTruthy();
  });

  it("opens the composer on the message a draft-reply action names", async () => {
    // Through the page: the card only names the action, and the page is what
    // performs it.
    const unanswered = {
      ...stalledSuggestion,
      kind: "no_reply",
      fingerprint: "fp-reply-1",
      reason: "You reached out 15 days ago and nobody has come back.",
      action: { kind: "draft_reply", activity_id: "a-1" },
    };
    stubFetch(companyBackstop, {
      company360: { ...company360, suggestions: [unanswered] },
    });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });
    await openTasks();

    await userEvent.click(
      await screen.findByRole("button", { name: "Create draft" }),
    );
    // The composer stands over the record. The details it covers are still
    // where the reader left them.
    await screen.findByRole("button", { name: "Close" });
    expect(
      container.querySelector(".record-aside .recorddetails"),
    ).toBeTruthy();
  });

  it("goes to the deal an open-deal action names", async () => {
    stubFetch(companyBackstop, {
      company360: {
        ...company360,
        suggestions: [
          {
            ...stalledSuggestion,
            action: { kind: "open_deal", deal_id: "d-7" },
          },
        ],
      },
    });
    render(<CompanyScreen id="o-1" />);
    await openTasks();
    await userEvent.click(
      await screen.findByRole("button", { name: "Open deal" }),
    );
    await waitFor(() => expect(window.location.hash).toContain("d-7"));
  });

  it("names how many suggestions the card left out", async () => {
    const three60 = {
      ...company360,
      suggestions: [stalledSuggestion],
      suggestions_dropped: 3,
    };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    // A truncated list with no count reads as "that is everything".
    await waitFor(() =>
      expect(screen.getByText("3 more not shown.")).toBeTruthy(),
    );
  });

  it("stays silent about what it left out when there is nothing left out", async () => {
    // Zero is the ordinary case, so the "N more" line must not render on it.
    // Otherwise every card carries "0 more not shown."
    const three60 = {
      ...company360,
      suggestions: [stalledSuggestion],
      suggestions_dropped: 0,
    };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    expect(screen.queryByText(/more not shown/)).toBeNull();
  });

  it("stays silent about what it left out when the count is absent", async () => {
    // Absent means the section was never computed. A "0 more" line would state a
    // fact about an account this read did not look at.
    const three60 = {
      ...company360,
      suggestions: [stalledSuggestion],
      suggestions_dropped: undefined,
    };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    expect(screen.queryByText(/more not shown/)).toBeNull();
  });

  // An empty list would read "nothing needs you" beside a move the Next bar
  // still asks for, so the Tasks tab leaves the list out.
  it("offers no advice row when the account needs nothing", async () => {
    stubFetch(companyBackstop);
    renderSuggestionsFor(company360);

    await screen.findByRole("heading", { name: "Signals" });
    expect(
      screen.queryByRole("heading", { name: "Needs attention" }),
    ).toBeNull();
    expect(screen.queryByRole("button", { name: "Not now" })).toBeNull();
  });

  it("names the withheld advice rather than claiming nothing needs attention", async () => {
    const three60 = {
      ...company360,
      suggestions: undefined,
      sections_omitted: ["suggestions"],
    };
    stubFetch(companyBackstop, { company360: three60 });
    renderSuggestionsFor(three60);

    await screen.findByText(/^Not included: suggestions\./);
    expect(screen.queryByText("Nothing needs attention right now.")).toBeNull();
  });

  it("dismisses by fingerprint and leaves the row for the server to remove", async () => {
    let dismissed: unknown;
    stubFetch(
      async (url, method, request) => {
        if (method === "POST" && url.includes("/suggestions/dismiss")) {
          dismissed = await request.json();
          return new Response(null, { status: 204 });
        }
        return companyBackstop(url);
      },
      { company360: { ...company360, suggestions: [stalledSuggestion] } },
    );
    renderSuggestionsFor({ ...company360, suggestions: [stalledSuggestion] });

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Not now" }));

    await waitFor(() => expect(dismissed).toBeTruthy());
    // The server decides what survives: the card sends the fingerprint and
    // does not hide the row itself. Whether the surrounding page then re-reads
    // the 360 is the page's business, and this suite mounts the brief alone.
    expect(dismissed).toEqual({ fingerprint: "fp-stalled-1" });
    expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy();
  });

  it("says a dismissal failed instead of leaving the click looking like a miss", async () => {
    stubFetch(
      async (url, method) => {
        if (method === "POST" && url.includes("/suggestions/dismiss")) {
          return jsonResponse({ title: "nope" }, 500);
        }
        return companyBackstop(url);
      },
      { company360: { ...company360, suggestions: [stalledSuggestion] } },
    );
    renderSuggestionsFor({ ...company360, suggestions: [stalledSuggestion] });

    await waitFor(() =>
      expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Not now" }));

    await waitFor(() =>
      expect(screen.getByText(/was not dismissed/)).toBeTruthy(),
    );
    // The row is still there, which is what the notice is telling the reader.
    expect(screen.getByText(stalledSuggestion.reason)).toBeTruthy();
  });
});

describe("CompanyScreen — Ask Margince", () => {
  const answer = {
    company_id: "o-1",
    question: "whats_open",
    generated_at: "2026-06-01T09:00:00Z",
    generated_by: "model",
    sentences: [
      {
        text: "Two open deals, worth about 57000 EUR.",
        evidence: [{ entity_type: "deal", entity_id: "d-1" }],
      },
    ],
  };

  it("asks only the prepared questions, and shows which one the answer answers", async () => {
    let asked: unknown;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.endsWith("/ask")) {
        asked = await request.json();
        return jsonResponse(answer);
      }
      return companyBackstop(url);
    });
    render(<AssistantPanel companyId="o-1" onOpenRecord={() => {}} />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "What is open here?" }),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "What is open here?" }),
    );

    await waitFor(() => expect(asked).toEqual({ question: "whats_open" }));
    await waitFor(() =>
      expect(
        screen.getByText("Two open deals, worth about 57000 EUR."),
      ).toBeTruthy(),
    );
    // Which writer produced it is never implied.
    expect(screen.getByText("Written by Margince")).toBeTruthy();
    // The question is repeated over its answer, so a reader who has scrolled
    // cannot pair the wrong one with it.
    expect(screen.getAllByText("What is open here?").length).toBeGreaterThan(1);
  });

  it("says there is nothing to answer from rather than nothing at all", async () => {
    stubFetch(async (url, method) => {
      if (method === "POST" && url.endsWith("/ask")) {
        return jsonResponse({ ...answer, sentences: [] });
      }
      return companyBackstop(url);
    });
    render(<AssistantPanel companyId="o-1" onOpenRecord={() => {}} />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "What is open here?" }),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "What is open here?" }),
    );

    await waitFor(() =>
      expect(screen.getByText(/No records you can access/)).toBeTruthy(),
    );
  });

  it("reports a failed question instead of leaving the card blank", async () => {
    stubFetch(async (url, method) => {
      if (method === "POST" && url.endsWith("/ask")) {
        return jsonResponse({ title: "nope" }, 500);
      }
      return companyBackstop(url);
    });
    render(<AssistantPanel companyId="o-1" onOpenRecord={() => {}} />);

    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "What is open here?" }),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByRole("button", { name: "What is open here?" }),
    );

    await waitFor(() =>
      expect(screen.getByText(/could not be answered/)).toBeTruthy(),
    );
  });
});

// The Overview is the design's inventory and nothing else: the Next bar, How
// it stands, then four cards. Every other reading lives on the tab the design
// gives it, so a layout change is never an availability change.
describe("CompanyScreen — the Overview and the details beside it", () => {
  it("puts the account's details beside the work, as flat groups", async () => {
    stubFetch(companyBackstop, { company360 });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    const pane = container.querySelector(".record-aside");
    await waitFor(() =>
      expect(pane?.querySelector(".recorddetails")).toBeTruthy(),
    );
    // No titled cards in the details: groups, not a "Details" panel.
    expect(pane?.querySelector(".panel-title")).toBeNull();
    expect(pane?.textContent).toContain("Ownership");
    expect(pane?.textContent).toContain("Registration");
  });

  it("draws exactly the four cards under How it stands", async () => {
    stubFetch(companyBackstop, { company360 });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    const stack = container.querySelector(".co-overview-stack");
    await waitFor(() =>
      expect(stack?.querySelector(".co-overview-grid")).toBeTruthy(),
    );
    const cards = Array.from(
      stack?.querySelectorAll(
        ".co-overview-grid .panel-head-flat .panel-title",
      ) ?? [],
    ).map((title) => title.textContent);
    expect(cards).toEqual([
      "Also on your list",
      "Deals and projects",
      "Latest",
      "Contacts",
    ]);
    for (const gone of [
      "Growth fit",
      "Signals",
      "Needs attention",
      "Company overview",
      "Ask about this company",
      "Active deals",
      "Key contacts",
    ]) {
      expect(stack?.textContent).not.toContain(gone);
    }
  });

  it("keeps the readings it moved reachable on their own tabs", async () => {
    stubFetch(companyBackstop, { company360 });
    render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    await openTasks();
    expect(
      await screen.findByRole("heading", { name: "Signals" }),
    ).toBeTruthy();
    await openProfile();
    expect(
      await screen.findByRole("heading", { name: "Ask about this company" }),
    ).toBeTruthy();
  });

  it("keeps the description out of the details and edits it on Profile", async () => {
    const described = { ...company, description: "Runs fleet depots." };
    stubFetch(
      async (url) =>
        url.endsWith("/companies/o-1")
          ? jsonResponse(described)
          : companyBackstop(url),
      { company360: { ...company360, company: described } },
    );
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });
    const aside = () => container.querySelector(".record-aside");
    await waitFor(() =>
      expect(aside()?.querySelector(".recorddetails")).toBeTruthy(),
    );
    expect(aside()?.textContent).not.toContain("Runs fleet depots.");

    await openProfile();
    expect(await screen.findByText("Runs fleet depots.")).toBeTruthy();
  });

  it("titles the Next bar with the task the moment cites, not its headline", async () => {
    stubFetch(companyBackstop, {
      company360: {
        ...company360,
        moment: {
          claim_key: "moment:account_promise_task:t-1",
          evidence_fingerprint: "c".repeat(64),
          rule: "overdue_promise",
          headline: "You owe them: Send the renewal paperwork",
          why_now: "Due 3 days ago and still open.",
          confidence: "observed_fact",
          evidence: [{ type: "task", id: "t-1", label: "Renewal" }],
          recommended_action: {
            kind: "complete_task",
            label: "Open it from the task list",
            state: "blocked",
          },
        },
        next_steps: {
          ...emptySection,
          data: [
            {
              activity_id: "t-1",
              subject: "Send the renewal paperwork",
              due_at: "2026-05-29T09:00:00Z",
              overdue: true,
            },
          ],
        },
      },
    });
    render(<CompanyScreen id="o-1" />);

    // The task also stands on the timeline, so it is found more than once.
    expect(
      (await screen.findAllByText("Send the renewal paperwork")).length,
    ).toBeGreaterThan(0);
    expect(
      screen.queryByText("You owe them: Send the renewal paperwork"),
    ).toBeNull();
  });

  // The drawer opens over the record, not into a column of it. The details stay
  // where the reader left them while it stands and after it closes.
  it("leaves the details standing while a composer is open", async () => {
    stubFetch(companyBackstop, { company360 });
    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });
    const details = () =>
      container.querySelector(".record-aside .recorddetails");
    await waitFor(() => expect(details()).toBeTruthy());

    await userEvent.click(screen.getByRole("button", { name: "Email" }));
    await screen.findByRole("button", { name: "Close" });
    expect(details()).toBeTruthy();

    await userEvent.click(screen.getByRole("button", { name: "Close" }));
    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Close" })).toBeNull(),
    );
    expect(details()).toBeTruthy();
  });

  // A call or a note often carries no subject. Dropping the unsubjected rows
  // would draw "nothing logged" on an account called five times.
  it("lists a logged call in Latest, even with no subject", async () => {
    stubFetch(companyBackstop, {
      company360: {
        ...company360,
        activities: {
          data: [
            {
              id: "a-1",
              kind: "call",
              occurred_at: "2026-06-01T08:30:00Z",
              direction: "inbound",
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
    });
    render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    const rows = await latestRows();
    expect(rows[0]?.textContent).toContain("Call");
    expect(latestCard()?.textContent).not.toContain(en["co.recent.empty"]);
  });

  // The newest exchange is the one a reader came back for, so it leads.
  it("lists the newest exchange first in Latest", async () => {
    stubFetch(companyBackstop, {
      company360: {
        ...company360,
        // Latest lists what already happened, so the read is dated after both.
        as_of: "2026-06-03T09:00:00Z",
        activities: {
          data: [
            {
              id: "a-new",
              kind: "email",
              subject: "Rate sheet sent",
              occurred_at: "2026-06-02T09:00:00Z",
              direction: "outbound",
            },
            {
              id: "a-old",
              kind: "call",
              subject: "Quarterly review",
              occurred_at: "2026-06-01T08:30:00Z",
              direction: "inbound",
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
    });
    render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });

    const rows = await latestRows();
    expect(rows[0]?.textContent).toContain("Rate sheet sent");
    expect(rows[1]?.textContent).toContain("Quarterly review");
  });

  // A name and a link say somebody exists. To pick which contact to call, a
  // reader would open all three to find the buyer. The list carries that.
  it("carries what a listed contact does and how to write to them", async () => {
    stubFetch(companyBackstop, {
      // The reader has a mailbox to send from, so the address is the
      // composer's; without one it would be their own mail client's.
      connectors: {
        data: [
          { id: "g1", provider: "gmail", status: "connected", scopes: [] },
        ],
      },
      company360: {
        ...company360,
        contacts: {
          data: [
            {
              contact_id: "p-1",
              full_name: "Anna Brandt",
              title: "Head of Procurement",
              primary_email: "anna.brandt@brandt-automotive.de",
              deal_roles: [],
              consent: {},
              strength: {
                score: 0,
                bucket: "none",
                factors: {
                  recency: 0,
                  frequency: 0,
                  reciprocity: 0,
                  direction: 0,
                },
              },
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
    });
    render(<CompanyScreen id="o-1" />);

    // The name opens the record and is the link's whole name. The mark beside
    // it draws initials as text, which would otherwise be announced first.
    // The page names this contact in the roster card and the references beside
    // it. Every one is asserted, since a single lookup misses a stray link.
    const names = await screen.findAllByRole("link", { name: "Anna Brandt" });
    for (const each of names) {
      expect(each.getAttribute("href")).toBe("/#/contacts/p-1");
    }
    const name = names[0];
    // The title shares its line with the company's name.
    expect(screen.getAllByText(/Head of Procurement/).length).toBeGreaterThan(
      0,
    );

    // The address is its own control, a sibling of the name, not nested in it.
    // A control inside a link is a press nobody can predict. It is a button
    // into the product's composer, never a link the reader's mail client takes.
    const write = (
      await screen.findAllByRole("button", {
        name: "anna.brandt@brandt-automotive.de",
      })
    )[0];
    expect(write.hasAttribute("href")).toBe(false);
    expect(name.contains(write)).toBe(false);
  });

  // None of the reference cards comes from the 360; each runs its own read.
  // So they must be on the page before that read lands, and stay if it never does.
  it("offers the reference cards before the 360 read lands", async () => {
    let releaseView: (() => void) | undefined;
    const held = new Promise<void>((resolve) => {
      releaseView = resolve;
    });
    const { fetchMock } = stubFetch(companyBackstop);
    const held360 = vi.fn(async (request: Request) => {
      if (new URL(request.url).pathname.endsWith("/360")) {
        await held;
      }
      return fetchMock(request);
    });
    vi.stubGlobal("fetch", held360);

    const { container } = render(<CompanyScreen id="o-1" />);
    await screen.findByRole("heading", { name: company.display_name });
    await openProfile();

    // Asserted on the panel headings the tab is built from. These words also
    // appear in the app's navigation, so a bare text match would pass on a
    // page that lost every one of these sections.
    const headings = () =>
      Array.from(container.querySelectorAll(".panel-head")).map((el) =>
        el.textContent?.trim(),
      );
    // A panel's header band carries its description line and its title.
    // So these match the title at the start, not the whole band.
    const opensWith = (title: string) =>
      headings().some((one) => one?.startsWith(title));
    await waitFor(() => expect(opensWith("How they buy")).toBe(true));
    expect(opensWith("Company facts")).toBe(true);
    // The roll-up is one of the panes that runs its own read, so it arrives
    // on its own clock rather than with the ones above.
    await waitFor(() => expect(opensWith("Roll-up")).toBe(true));
    releaseView?.();
  });
});

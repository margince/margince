/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { CompanyTab } from "../../lib/companytab";
import { RecordZoneProvider } from "../../lib/recordzone";
import { StandingFigures } from "./companystanding.figures";

// The four figures under How it stands. Each draws even when it has no
// reading. A card that returned null would leave the row one short and the
// reader unable to tell which reading went missing. Only the words differ.

type Company360 = components["schemas"]["Company360"];
type StateStripSection = components["schemas"]["Company360StateStrip"];
type FinanceSummary = components["schemas"]["CompanyFinanceSummary"];

const NO_CONNECTION: FinanceSummary = {
  company_id: "o-1",
  state: "no_connection",
};

const company: components["schemas"]["Company"] = {
  id: "o-1",
  display_name: "Brandt Automotive GmbH",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const customer: StateStripSection = {
  account: { lifecycle: "customer", relationship_types: ["customer"] },
  commercial: {
    open_count: 1,
    stalled_count: 0,
    priced_count: 1,
    converted_count: 0,
    open_pipeline_minor_base: 100_000,
    base_currency: "EUR",
    next_close_on: "2026-09-30",
  },
};

const prospect: StateStripSection = {
  ...customer,
  account: { lifecycle: "prospect", relationship_types: [] },
};

function view(overrides: Partial<Company360> = {}): Company360 {
  return {
    as_of: "2026-08-18T09:00:00Z",
    company,
    sections_omitted: [],
    state_strip: customer,
    ...overrides,
  };
}

// The money cell reads the finance summary, a query of its own. An unstubbed
// fetch is a rejected promise. A cell reading "could not be read" for that
// reason would pass a test written about a connection that is not set up.
function stubFinance(summary: FinanceSummary = NO_CONNECTION) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const { pathname } = new URL(request.url);
      if (pathname.endsWith("/finance-summary")) {
        return new Response(JSON.stringify(summary), {
          status: 200,
          headers: { "content-type": "application/json" },
        });
      }
      throw new Error(`the figures asked for ${pathname}, which no test stubs`);
    }),
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function renderFigures(
  three60: Company360,
  onOpenTab?: (tab: CompanyTab) => void,
  zone?: string,
): Promise<HTMLElement> {
  stubFinance();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const figures = (
    <StandingFigures companyId="o-1" view={three60} onOpenTab={onOpenTab} />
  );
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {zone ? (
          <RecordZoneProvider zone={zone}>{figures}</RecordZoneProvider>
        ) : (
          figures
        )}
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return screen.findByRole("region", { name: en["co.strip.title"] });
}

const labelsOf = (plate: HTMLElement): (string | null)[] =>
  [...plate.children].map(
    (card) => card.querySelector(".stat-card-label")?.textContent ?? null,
  );

describe("the figures under How it stands", () => {
  it("draws next meeting, last contact, revenue and open deals, in that order", async () => {
    const plate = await renderFigures(view());

    expect(plate.dataset.testid).toBe("company-figures");
    expect(labelsOf(plate)).toEqual([
      en["co.strip.nextMeeting"],
      en["co.strip.lastTouch"],
      en["co.strip.netInvoiced"],
      en["co.strip.pipeline"],
    ]);
    for (const card of plate.children) {
      expect(card.classList.contains("stat-card")).toBe(true);
    }
  });

  it("draws the same four for a prospect and never an overdue cell", async () => {
    const plate = await renderFigures(view({ state_strip: prospect }));

    expect(plate.childElementCount).toBe(4);
    expect(within(plate).queryByText(en["finance.overdue"])).toBeNull();
  });

  it("reads a money figure only once the account is billed", async () => {
    const plate = await renderFigures(view({ state_strip: prospect }));

    expect(
      within(plate).getByText(en["co.strip.fin.neverInvoiced"]),
    ).toBeTruthy();
    expect(
      within(plate).getByText(en["company.lifecycle.prospect"]),
    ).toBeTruthy();
  });

  it("draws the open-deals figure compactly, never at full length", async () => {
    const plate = await renderFigures(
      view({
        state_strip: {
          ...customer,
          commercial: {
            ...customer.commercial,
            open_count: 1,
            stalled_count: 0,
            priced_count: 1,
            converted_count: 0,
            open_pipeline_minor_base: 14_360_000,
            base_currency: "EUR",
          },
        },
      }),
    );

    expect(within(plate).getByText(/143(\.6)?K/i)).toBeTruthy();
    expect(plate.textContent).not.toContain("143,600.00");
  });

  it("is withheld as a whole when the reader may not see the strip", async () => {
    stubFinance();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <LocaleProvider initial="en">
          <StandingFigures
            companyId="o-1"
            view={view({
              state_strip: undefined,
              sections_omitted: ["state_strip"],
            })}
          />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    expect(await screen.findByText(en["co.section.restricted"])).toBeTruthy();
  });
});

describe("the next-meeting cell", () => {
  const meeting = {
    activity_id: "00000000-0000-4000-8000-0000000000bb",
    occurred_precision: "instant" as const,
    subject: "Walkthrough",
    participants: [],
  };

  function cellOf(plate: HTMLElement): HTMLElement {
    const card = within(plate)
      .getByText(en["co.strip.nextMeeting"])
      .closest(".stat-card");
    if (!(card instanceof HTMLElement)) {
      throw new Error("the next-meeting reading has no card");
    }
    return card;
  }

  it("names the day and the subject of the meeting booked ahead", async () => {
    const plate = await renderFigures(
      view({
        next_meeting: { ...meeting, starts_at: "2026-08-20T09:00:00Z" },
      }),
    );

    expect(cellOf(plate).textContent).toContain("20 Aug");
    expect(cellOf(plate).textContent).toContain("Walkthrough");
  });

  it("says none is booked when there is no meeting", async () => {
    const plate = await renderFigures(view());

    expect(
      within(cellOf(plate)).getByText(en["co.strip.next.none"]),
    ).toBeTruthy();
  });

  // A meeting already held is history. A past date on a cell that says "next"
  // would send a rep to chase a meeting that is over.
  it("does not offer a meeting that is already past as the next one", async () => {
    const plate = await renderFigures(
      view({
        next_meeting: { ...meeting, starts_at: "2026-08-10T09:00:00Z" },
      }),
    );

    expect(
      within(cellOf(plate)).getByText(en["co.strip.next.none"]),
    ).toBeTruthy();
  });

  it("says restricted, not none, when the reader may not see meetings", async () => {
    const plate = await renderFigures(
      view({ sections_omitted: ["next_meeting"] }),
    );

    expect(
      within(cellOf(plate)).getByText(en["reading.restricted"]),
    ).toBeTruthy();
    expect(cellOf(plate).textContent).not.toContain(en["co.strip.next.none"]);
  });
});

describe("the last-contact cell", () => {
  function cellOf(plate: HTMLElement): HTMLElement {
    const card = within(plate)
      .getByText(en["co.strip.lastTouch"])
      .closest(".stat-card");
    if (!(card instanceof HTMLElement)) {
      throw new Error("the last-contact reading has no card");
    }
    return card;
  }

  // The server's last contact counts a meeting and a call as well as mail.
  it("names a meeting when no mail was ever exchanged", async () => {
    const plate = await renderFigures(
      view({
        last_contact: {
          at: "2026-08-13T09:00:00Z",
          kind: "meeting",
          activity_id: "00000000-0000-4000-8000-0000000000aa",
        },
      }),
    );

    expect(cellOf(plate).textContent).toContain("5 d");
    expect(cellOf(plate).textContent).toContain(en["co.spine.kind.meeting"]);
  });

  // 15:00 the day before, read at 01:00 in UTC+7: yesterday for the reader,
  // the same day for UTC. The count follows the zone the date beside it is
  // drawn in.
  it.each([
    ["Asia/Bangkok", "1 d"],
    ["UTC", en["co.strip.lastTouch.today"]],
  ])("counts the day in %s", async (zone, shown) => {
    const plate = await renderFigures(
      view({
        as_of: "2026-08-17T18:00:00Z",
        last_contact: {
          at: "2026-08-17T08:00:00Z",
          kind: "meeting",
          activity_id: "00000000-0000-4000-8000-0000000000aa",
        },
      }),
      undefined,
      zone,
    );

    expect(within(cellOf(plate)).getByText(shown)).toBeTruthy();
  });

  it("says none when the server names no last contact", async () => {
    const plate = await renderFigures(view());

    expect(
      within(cellOf(plate)).getByText(en["co.strip.lastTouch.never"]),
    ).toBeTruthy();
  });
});

// Every door on the row carries the same word, "Open". What tells them apart
// for a screen reader is the description: the reading's own label.
function door(label: string): HTMLElement {
  return screen.getByRole("button", {
    name: en["stat.open"],
    description: label,
  });
}

describe("a figure offers the tab it is a figure of", () => {
  it("sends the reader to history, finance and deals from their own doors", async () => {
    const opened: CompanyTab[] = [];
    await renderFigures(view(), (tab) => opened.push(tab));

    for (const label of [
      en["co.strip.nextMeeting"],
      en["co.strip.lastTouch"],
      en["co.strip.netInvoiced"],
      en["co.strip.pipeline"],
    ]) {
      await userEvent.click(door(label));
    }

    expect(opened).toEqual(["timeline", "timeline", "finance", "deals"]);
  });

  it("draws no door when the caller has nowhere to send them", async () => {
    await renderFigures(view());

    expect(screen.queryAllByRole("button", { name: /^Open / })).toHaveLength(0);
  });
});

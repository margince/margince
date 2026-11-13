/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider, translate } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import { company } from "./company360.testkit";
import { GrowthFitPanel } from "./companygrowthfit";

// The panel's job is to stop one misreading: `unknown` means "we could not
// judge", and a reader must never be able to mistake it for "a poor fit".
// Those are opposite conclusions, so the tests below are mostly about what the
// panel refuses to render on its own.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type GrowthFit = components["schemas"]["CompanyGrowthFit"];
type GrowthFitInput = components["schemas"]["GrowthFitInput"];

const INPUTS: GrowthFitInput[] = [
  "offer_summary",
  "icp",
  "industry",
  "buying_center",
  "buying_intents",
  "employee_range",
  "technology",
];

// Every input in the server's order, the named ones not counting.
function states(missing: readonly GrowthFitInput[]) {
  return INPUTS.map((input) => ({ input, present: !missing.includes(input) }));
}

// A COMPLETE CompanyGrowthFit, not a cast one. A fixture asserted into
// the contract type can drop a required field and still compile, so the test
// would go on passing after the wire shape moved under it.
const ABSTAINED: GrowthFit = {
  company_id: "o-1",
  band: "unknown",
  data_completeness: {
    present: 2,
    expected: 7,
    needed: 4,
    inputs: states(["industry", "employee_range"]),
    missing: ["industry", "employee_range"],
  },
  next_step: "gather_inputs",
  generated_at: "2026-08-08T09:00:00Z",
  generated_by: "deterministic",
};

function serving(fit: GrowthFit) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(fit), {
          status: 200,
          headers: { "content-type": "application/json" },
        }),
    ),
  );
}

async function show(fit: GrowthFit, locale: "en" | "de" | "vi" = "en") {
  serving(fit);
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial={locale}>
        <GrowthFitPanel company={company} onOpenReceipt={() => {}} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  // The "as of" stamp only renders once the query has resolved, so waiting for
  // it is what makes the synchronous assertions below run against loaded
  // content. The panel heading is static and would match while the skeleton is
  // up.
  await screen.findByText(/as of |Stand |tính đến/i);
}

describe("the wait before the first assessment", () => {
  // A cold assessment is assembled on the request, model call included, and
  // that runs into tens of seconds. For all of it the panel drew its heading
  // and then a mute grey rectangle, which reads as a panel that is broken
  // rather than one that is working — and to a screen reader it read as
  // nothing at all.
  it("says what it is waiting for, out loud and to assistive tech", () => {
    // A fetch that never settles IS the pending state; there is no clock here
    // to advance and nothing to wait for.
    vi.stubGlobal(
      "fetch",
      vi.fn(() => new Promise<Response>(() => {})),
    );
    const { container } = render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <GrowthFitPanel company={company} onOpenReceipt={() => {}} />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    const live = container.querySelector("[role='status'][aria-busy='true']");
    expect(live).toBeTruthy();
    expect(live?.textContent).toContain("Assessing this company");
    // And it must not have resolved into any of the panel's verdicts.
    expect(screen.queryByText("Not enough data")).toBeNull();
  });
});

describe("how well this company fits what we sell", () => {
  it("says it could not judge, rather than showing a low score", async () => {
    await show(ABSTAINED);

    expect(screen.getByText("Not enough data")).toBeTruthy();
    // The three real bands are verdicts. None of them may appear beside an
    // abstention, because a reader scanning for a verdict would take it.
    expect(screen.queryByText("Weak fit")).toBeNull();
    expect(screen.queryByText("Moderate fit")).toBeNull();
    expect(screen.queryByText("Strong fit")).toBeNull();
  });

  it("reports both completeness counts, never a bare proportion", async () => {
    await show(ABSTAINED);

    // "2 of 7" and "2 of 40" describe different levels of knowledge, and a
    // denominator-less figure renders them identically.
    expect(screen.getByText(/2 of 7 inputs recorded/)).toBeTruthy();
  });

  it("names what is missing and what to do about it", async () => {
    await show(ABSTAINED);

    // The checklist marks each gap where it sits.
    expect(screen.getAllByText(/^(Missing|Out of date)$/)).toHaveLength(2);
    expect(
      screen.getByText(
        /Next: find out their industry and how big they are before this fit can be judged/,
      ),
    ).toBeTruthy();
  });

  it("words the missing inputs and the next step in the reader's language", async () => {
    await show(ABSTAINED, "vi");

    expect(
      screen.getByText(
        "Tiếp theo: tìm hiểu ngành nghề và quy mô công ty, rồi mới đánh giá được mức độ phù hợp.",
      ),
    ).toBeTruthy();
    expect(screen.queryByText(/their industry/)).toBeNull();
    expect(screen.queryByText(/gather_inputs/)).toBeNull();
  });

  it("words the missing inputs in German", async () => {
    await show(ABSTAINED, "de");

    expect(screen.getByText(/ihre Branche und wie groß sie sind/)).toBeTruthy();
    expect(screen.queryByText(/employee_range/)).toBeNull();
  });

  it("says why a band was held back, when one was", async () => {
    await show({
      ...ABSTAINED,
      band: "moderate",
      band_capped_reason: "we have not confirmed what this workspace sells",
      next_step: "confirm_offering",
      data_completeness: {
        present: 7,
        expected: 7,
        needed: 4,
        inputs: states([]),
      },
    });

    expect(screen.getByText("Moderate fit")).toBeTruthy();
    // A band lowered without a reason is a number the reader cannot argue
    // with, which is the one thing a capped band must never be.
    expect(
      screen.getByText(/we have not confirmed what this workspace sells/),
    ).toBeTruthy();
  });

  it("marks an assessment apart from a fact, and cites both", async () => {
    await show({
      ...ABSTAINED,
      band: "strong",
      data_completeness: {
        present: 7,
        expected: 7,
        needed: 4,
        inputs: states([]),
      },
      next_step: undefined,
      generated_by: "model",
      positive_factors: [
        {
          text: "They run SAP.",
          nature: "fact",
          evidence: [{ entity_type: "fact", entity_id: "f-1" }],
        },
        {
          text: "Their stack matches who we sell to.",
          nature: "assessment",
          evidence: [{ entity_type: "company", entity_id: "o-1" }],
        },
      ],
    });

    expect(screen.getByText("Supporting factors")).toBeTruthy();
    // A judgment that read as a stored fact would be the one claim the reader
    // could not check, so only the assessment carries a label. The nature word
    // sits in the sentence's own line with a trailing colon
    // (NatureBadge, components/citations.list.tsx). So the matcher needs only the
    // word, not the exact rendered text.
    expect(screen.getByText(/Assessment/)).toBeTruthy();
    // "Fact" is the label a fact WOULD carry if facts were labelled, so its
    // absence is what proves the badge is reserved for judgments. Asserting a
    // string the panel never renders under any nature would prove nothing.
    expect(screen.queryByText("Fact")).toBeNull();
  });

  it("distinguishes a payload it cannot read from a company it knows nothing about", async () => {
    // A schema skew, not an empty account. Rendering it as "0 of 7 inputs"
    // would send the reader off to gather data that is already there.
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify({ company_id: "o-1" }), {
            status: 200,
            headers: { "content-type": "application/json" },
          }),
      ),
    );
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <GrowthFitPanel company={company} onOpenReceipt={() => {}} />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    expect(
      await screen.findByText(/Assessment could not be loaded/),
    ).toBeTruthy();
    expect(screen.queryByText(/inputs recorded/)).toBeNull();
  });
});

// The status region is mounted before the press, so wait for its text.
async function announced(): Promise<string> {
  const region = screen.getByRole("status");
  await waitFor(() => expect(region.textContent).not.toBe(""));
  return region.textContent ?? "";
}

describe("Assess again", () => {
  it("holds its status region open before the press, empty", async () => {
    await show(ABSTAINED);
    expect(screen.getByRole("status").textContent).toBe("");
  });

  it("drops the outcome when the panel moves to another company", async () => {
    serving(ABSTAINED);
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    const panel = (companyId: string) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <GrowthFitPanel
            company={{ ...company, id: companyId }}
            onOpenReceipt={() => {}}
          />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const { rerender } = render(panel("o-1"));
    await screen.findByText(/as of /i);
    screen.getByRole("button", { name: "Assess again" }).click();
    expect(await announced()).toMatch(/Result unchanged/);
    serving({ ...ABSTAINED, company_id: "o-2" });
    rerender(panel("o-2"));
    await waitFor(() => expect(screen.getAllByRole("status")).toHaveLength(1));
    expect(screen.getByRole("status").textContent).toBe("");
  });

  it("says the result did not change when the same reading comes back", async () => {
    await show(ABSTAINED);
    screen.getByRole("button", { name: "Assess again" }).click();
    expect(await announced()).toBe(
      "Assessed again. Result unchanged: Not enough data, 2 of 7 inputs recorded.",
    );
  });

  it("calls a reading unchanged when only the run's time moved", async () => {
    await show(ABSTAINED);
    serving({ ...ABSTAINED, generated_at: "2026-08-09T10:00:00Z" });
    screen.getByRole("button", { name: "Assess again" }).click();
    expect(await announced()).toMatch(/Result unchanged/);
  });

  it("calls a reading new when the counts hold but the missing inputs moved", async () => {
    await show(ABSTAINED);
    serving({
      ...ABSTAINED,
      data_completeness: {
        present: 2,
        expected: 7,
        needed: 4,
        inputs: states(["industry", "technology"]),
        missing: ["industry", "technology"],
      },
    });
    screen.getByRole("button", { name: "Assess again" }).click();
    expect(await announced()).toMatch(/^Assessed again\. New result: /);
  });

  it("names the new result when the reading moved", async () => {
    await show(ABSTAINED);
    serving({
      ...ABSTAINED,
      band: "weak",
      data_completeness: {
        present: 5,
        expected: 7,
        needed: 4,
        inputs: states([]),
        missing: [],
      },
      next_step: undefined,
    });
    screen.getByRole("button", { name: "Assess again" }).click();
    expect(await announced()).toMatch(
      /^Assessed again\. New result: .+, 5 of 7 inputs recorded\.$/,
    );
  });
});

describe("what the fit is judged on", () => {
  // Three of seven inputs: offer, buyers and size are recorded, four are not.
  const THREE_OF_SEVEN: GrowthFit = {
    ...ABSTAINED,
    data_completeness: {
      present: 3,
      expected: 7,
      needed: 4,
      inputs: states(["icp", "industry", "buying_intents", "technology"]),
      missing: ["icp", "industry", "buying_intents", "technology"],
    },
  };

  function route(fit: GrowthFit) {
    const answer = (body: unknown) =>
      new Response(JSON.stringify(body), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const path = new URL(request.url).pathname;
        if (path.endsWith("/me")) {
          return answer(meFixture({ allow: { company: ["read", "update"] } }));
        }
        if (path.endsWith("/profile-fields")) {
          return answer({
            data: [
              {
                field: "offer_summary",
                value: "Electric delivery vans",
                source: "human",
                version: 1,
                updated_at: "2026-08-01T09:00:00Z",
              },
              {
                field: "buying_center",
                value: "Head of Fleet",
                source: "human",
                version: 1,
                updated_at: "2026-08-01T09:00:00Z",
              },
            ],
          });
        }
        if (path.endsWith("/facts")) {
          return answer({
            data: [
              {
                id: "f-1",
                category: "company",
                field: "employee_range",
                value: "51-200",
                source: "human",
                version: 1,
                updated_at: "2026-08-01T09:00:00Z",
              },
            ],
          });
        }
        return answer(fit);
      }),
    );
  }

  async function showFor(record: typeof company) {
    route(THREE_OF_SEVEN);
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <GrowthFitPanel company={record} onOpenReceipt={() => {}} />
        </LocaleProvider>
      </QueryClientProvider>,
    );
    await screen.findByText("Head of Fleet");
  }

  it("lists all seven inputs, the three recorded with their values", async () => {
    await showFor({ ...company, industry: null });

    for (const label of [
      "What they offer",
      "Who they sell to",
      "Industry",
      "Who does the buying",
      "What they buy for",
      "Company size",
      "Technology they run",
    ]) {
      expect(screen.getByText(label)).toBeTruthy();
    }
    expect(screen.getByText("Electric delivery vans")).toBeTruthy();
    expect(await screen.findByText(/51-200/)).toBeTruthy();
    expect(screen.getAllByText("Missing")).toHaveLength(4);
    expect(
      screen.getByText("At least 4 of 7 inputs are needed to judge the fit."),
    ).toBeTruthy();
    // Technology has no form: it comes from reading the site.
    expect(screen.getByText("Found when the website is read.")).toBeTruthy();
    // A gap opens the Profile tab's own editor for that field.
    expect(
      await screen.findByRole("button", { name: /Who they sell to/ }),
    ).toBeTruthy();
  });

  it("tells a reader who cannot edit the account why, once", async () => {
    await showFor({
      ...company,
      industry: null,
      writable: false,
      owner_id: "u-2",
    });

    expect(
      screen.getAllByText(translate("en", "record.notYoursToChange")),
    ).toHaveLength(1);
    // The gaps still show, with no editor that could only fail at the save.
    expect(screen.getAllByText("Missing")).toHaveLength(4);
    expect(
      screen.queryByRole("button", { name: /Who they sell to/ }),
    ).toBeNull();
  });
});

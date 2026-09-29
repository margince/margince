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
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { pickOption } from "../design-system/select-testing";
import { ToastProvider, ToastRegion } from "../design-system/toast";
import { LocaleProvider } from "../i18n";
import { DealsScreen } from "./deals";
import {
  acceptBody,
  CompanySuggestions,
  DealSuggestionCard,
} from "./dealsuggestion";
import type { DealSuggestion } from "./dealsuggestions.queries";

// A Deal Scout suggestion on the three surfaces that draw one, and the two
// answers a rep gives it. The board case is the one the feature must never
// break: a suggestion sits beside a column's deals and changes none of its
// figures.

type Deal = components["schemas"]["Deal"];
type Stage = components["schemas"]["Stage"];

const suggestion: DealSuggestion = {
  id: "sg-1",
  kind: "open_deal",
  state: "open",
  company_id: "co-1",
  company_name: "Acme GmbH",
  pipeline_id: "pl",
  stage_id: "s1",
  name_hint: "proposal_sent",
  amount_minor: 1_250_000,
  currency: "EUR",
  confidence: 0.9,
  created_at: "2026-09-27T09:00:00Z",
  evidence: [
    {
      kind: "attachment",
      attachment_id: "at-1",
      occurred_at: "2026-09-26T15:00:00Z",
      title: "Angebot_2026.pdf",
    },
    {
      kind: "meeting",
      activity_id: "ac-1",
      occurred_at: "2026-09-24T10:00:00Z",
      title: "Scoping workshop",
    },
  ],
};

const stages: Stage[] = [
  {
    id: "s1",
    pipeline_id: "pl",
    name: "Qualify",
    position: 1,
    semantic: "open",
    win_probability: 20,
  },
  {
    id: "s2",
    pipeline_id: "pl",
    name: "Proposal",
    position: 2,
    semantic: "open",
    win_probability: 50,
  },
];

const openDeal: Deal = {
  id: "d1",
  name: "Fleet retrofit",
  amount_minor: 4_800_000,
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: "s1",
  status: "open",
  source: "manual",
  captured_by: "human:u1",
  writable: true,
  version: 4,
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

type Sent = { method: string; path: string; body: unknown; key: string | null };

function stubBackend(opts: {
  suggestions: DealSuggestion[];
  mayDecide?: boolean;
  sent?: Sent[];
  // Answers every decision with this status and problem code instead.
  refuse?: { status: number; code: string };
}) {
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = input instanceof Request ? input : null;
    const url = new URL(
      String(request ? request.url : input),
      "http://localhost",
    );
    const method = request ? request.method : (init?.method ?? "GET");
    const path = url.pathname.replace(/^\/v1/, "");
    if (method === "POST" && path.startsWith("/deal-suggestions/")) {
      const text = request ? await request.text() : String(init?.body ?? "");
      opts.sent?.push({
        method,
        path,
        body: text ? JSON.parse(text) : undefined,
        key: request?.headers.get("Idempotency-Key") ?? null,
      });
      if (opts.refuse) {
        return jsonResponse(
          {
            code: opts.refuse.code,
            title: opts.refuse.code,
            status: opts.refuse.status,
          },
          opts.refuse.status,
        );
      }
      return path.endsWith("/accept")
        ? jsonResponse({
            suggestion: { ...suggestion, state: "accepted" },
            deal_id: "d-new",
            unlinked_activity_ids: [],
            acknowledged_signals: 0,
          })
        : jsonResponse({ ...suggestion, state: "dismissed" });
    }
    if (path === "/deal-suggestions") {
      return jsonResponse({
        data: opts.suggestions,
        page: { has_more: false },
      });
    }
    if (path === "/users") {
      return jsonResponse({
        data: [
          {
            id: "u-2",
            email: "kim@acme.test",
            display_name: "Kim Seller",
            status: "active",
            is_agent: false,
          },
        ],
        page: { next_cursor: null },
      });
    }
    if (path === "/pipelines") {
      return jsonResponse({
        data: [
          { id: "pl", name: "Sales", is_default: true, position: 0, stages },
        ],
        page: { next_cursor: null },
      });
    }
    if (method === "POST" && path.startsWith("/reports/")) {
      return jsonResponse({
        report: "deals-by-stage",
        plan: {},
        columns: [],
        rows: [
          {
            stage_id: "s1",
            currency: "EUR",
            deals: 1,
            amount_minor: 4_800_000,
            weighted_amount_minor: 960_000,
          },
        ],
      });
    }
    if (path === "/me") {
      return jsonResponse({
        user: {
          id: "u-me",
          email: "me@acme.test",
          display_name: "Me",
          status: "active",
          is_agent: false,
        },
        roles: ["rep"],
        teams: [],
        authorization: meFixture({
          allow:
            opts.mayDecide === false
              ? { deal: ["read"], company: ["read"] }
              : { deal: ["read", "create", "update"], company: ["read"] },
        }).authorization,
      });
    }
    if (path === "/deals") {
      return jsonResponse({ data: [openDeal], page: { next_cursor: null } });
    }
    return jsonResponse({ data: [], page: { next_cursor: null } });
  });
}

// The paths the stub has been asked for, without the API prefix.
function fetchedPaths(fetched: { mock: { calls: unknown[][] } }): string[] {
  return fetched.mock.calls.map((call) => {
    const first = call[0];
    return new URL(
      String(first instanceof Request ? first.url : first),
      "http://localhost",
    ).pathname.replace(/^\/v1/, "");
  });
}

function draw(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ToastProvider>
          <RecordShell>{ui}</RecordShell>
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("what an acceptance sends", () => {
  const untouched = {
    name: "Acme GmbH: proposal sent",
    amountMinor: 1_250_000,
    currency: "EUR",
    stageId: "s1",
    ownerId: "",
    closeDate: "",
  };

  it("sends only the name where the reader changed nothing else", () => {
    expect(
      acceptBody(
        { ...suggestion, amount_minor: null, currency: null },
        { ...untouched, amountMinor: 0 },
      ),
    ).toEqual({ name: "Acme GmbH: proposal sent" });
  });

  // Omitting the amount keeps the suggested one on the server, so an amount
  // the reader cleared has to be said out loud.
  it("drops the suggested amount when the reader empties the field", () => {
    expect(acceptBody(suggestion, { ...untouched, amountMinor: 0 })).toEqual({
      name: "Acme GmbH: proposal sent",
      no_amount: true,
    });
  });

  it("sends the amount only with its currency", () => {
    expect(acceptBody(suggestion, { ...untouched, currency: "USD" })).toEqual({
      name: "Acme GmbH: proposal sent",
      amount_minor: 1_250_000,
      currency: "USD",
    });
  });

  it("sends each correction the reader made", () => {
    expect(
      acceptBody(suggestion, {
        ...untouched,
        name: "  Acme rollout  ",
        stageId: "s2",
        ownerId: "u-2",
        closeDate: "2026-12-01",
      }),
    ).toEqual({
      name: "Acme rollout",
      amount_minor: 1_250_000,
      currency: "EUR",
      stage_id: "s2",
      owner_id: "u-2",
      close_date: "2026-12-01",
    });
  });
});

describe("a suggestion's card", () => {
  it("names the deal, its amount and every piece of evidence", async () => {
    vi.stubGlobal("fetch", stubBackend({ suggestions: [] }));
    draw(<DealSuggestionCard suggestion={suggestion} />);
    expect(screen.getByText("Acme GmbH: proposal sent")).toBeTruthy();
    expect(screen.getByText("€12,500.00")).toBeTruthy();
    expect(screen.getByText("Sent: Angebot_2026.pdf")).toBeTruthy();
    expect(screen.getByText("Meeting held: Scoping workshop")).toBeTruthy();
    expect(
      await screen.findByRole("button", { name: "Open this deal" }),
    ).toBeTruthy();
  });

  it("offers no answer to a reader who may not create deals", async () => {
    vi.stubGlobal("fetch", stubBackend({ suggestions: [], mayDecide: false }));
    draw(<DealSuggestionCard suggestion={suggestion} />);
    // The answers wait on the reader's grants; the positive case above shows
    // them arriving, so their absence once /me has answered is the refusal.
    await waitFor(() =>
      expect(fetchedPaths(vi.mocked(globalThis.fetch)).includes("/me")).toBe(
        true,
      ),
    );
    await screen.findByText("Automated by Deal Scout");
    expect(screen.queryByRole("button", { name: "Open this deal" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Not a deal" })).toBeNull();
  });

  it("opens the deal with the reader's corrections under one key", async () => {
    const user = userEvent.setup();
    const sent: Sent[] = [];
    vi.stubGlobal("fetch", stubBackend({ suggestions: [], sent }));
    draw(<DealSuggestionCard suggestion={suggestion} />);

    await user.click(
      await screen.findByRole("button", { name: "Open this deal" }),
    );
    const name = screen.getByLabelText(/^Deal name/);
    await user.clear(name);
    await user.type(name, "Acme rollout");
    const amount = screen.getByLabelText("Amount");
    await user.clear(amount);
    await user.type(amount, "20000");
    await user.type(screen.getByLabelText("Expected close"), "2026-12-01");
    await user.click(screen.getByRole("button", { name: "Open deal" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0].path).toBe("/deal-suggestions/sg-1/accept");
    expect(sent[0].body).toEqual({
      name: "Acme rollout",
      amount_minor: 2_000_000,
      currency: "EUR",
      close_date: "2026-12-01",
    });
    expect(sent[0].key).toBeTruthy();
    expect(await screen.findByText("Deal opened: Acme rollout.")).toBeTruthy();
  });

  it("dismisses the suggestion for everyone", async () => {
    const user = userEvent.setup();
    const sent: Sent[] = [];
    vi.stubGlobal("fetch", stubBackend({ suggestions: [], sent }));
    draw(<DealSuggestionCard suggestion={suggestion} />);

    await user.click(await screen.findByRole("button", { name: "Not a deal" }));
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0].path).toBe("/deal-suggestions/sg-1/dismiss");
    expect(sent[0].key).toBeTruthy();
  });
});

describe("a suggestion on the pipeline board", () => {
  // The column's head and sub are what a reader compares across the board, and
  // a suggestion is not a deal: both must read the same with one beside them.
  async function columnFigures(suggestions: DealSuggestion[]) {
    vi.stubGlobal("fetch", stubBackend({ suggestions }));
    draw(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    const column = screen.getByRole("region", { name: "Qualify" });
    const figures = [".board-col-head", ".board-col-sub"].map(
      (part) => column.querySelector(part)?.textContent ?? "",
    );
    return { column, figures };
  }

  it("draws the suggestion in the stage it would open in", async () => {
    const { column } = await columnFigures([suggestion]);
    expect(
      await within(column).findByText("Acme GmbH: proposal sent"),
    ).toBeTruthy();
    const proposal = screen.getByRole("region", { name: "Proposal" });
    expect(within(proposal).queryByTestId("deal-suggestion")).toBeNull();
  });

  it("leaves the column's count and totals as the deals alone make them", async () => {
    const withSuggestion = await columnFigures([suggestion]);
    await within(withSuggestion.column).findByTestId("deal-suggestion");
    const shown = withSuggestion.figures;
    cleanup();
    vi.unstubAllGlobals();
    const without = await columnFigures([]);
    expect(shown).toEqual(without.figures);
    expect(shown[0]).toContain("1");
  });
});

describe("the accept dialog's pickers and the refusals", () => {
  it("carries the stage, owner and currency the reader picked", async () => {
    const user = userEvent.setup();
    const sent: Sent[] = [];
    vi.stubGlobal("fetch", stubBackend({ suggestions: [], sent }));
    draw(<DealSuggestionCard suggestion={suggestion} />);

    await user.click(
      await screen.findByRole("button", { name: "Open this deal" }),
    );
    await pickOption(user, screen.getByLabelText("Currency"), "USD");
    await pickOption(user, await screen.findByLabelText("Stage"), "Proposal");
    await pickOption(user, screen.getByLabelText("Owner"), "Kim Seller");
    await user.click(screen.getByRole("button", { name: "Open deal" }));

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0].body).toMatchObject({
      amount_minor: 1_250_000,
      currency: "USD",
      stage_id: "s2",
      owner_id: "u-2",
    });
  });

  it("says a suggestion somebody decided first was already decided", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      stubBackend({
        suggestions: [],
        refuse: { status: 409, code: "conflict" },
      }),
    );
    draw(<DealSuggestionCard suggestion={suggestion} />);

    await user.click(await screen.findByRole("button", { name: "Not a deal" }));
    expect(
      await screen.findByText(
        "Someone already decided this suggestion. Reload to see where it stands.",
      ),
    ).toBeTruthy();
  });

  it("asks for another try when a dismissal simply failed", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      stubBackend({
        suggestions: [],
        refuse: { status: 500, code: "internal" },
      }),
    );
    draw(<DealSuggestionCard suggestion={suggestion} />);

    await user.click(await screen.findByRole("button", { name: "Not a deal" }));
    expect(
      await screen.findByText("That did not go through. Try again."),
    ).toBeTruthy();
  });
});

describe("a suggestion on its company's page", () => {
  it("draws the account's suggestion with its evidence, worded by kind", async () => {
    const signalled: DealSuggestion = {
      ...suggestion,
      name_hint: "opportunity_signalled",
      confidence: 0.5,
      amount_minor: null,
      currency: null,
      evidence: [
        {
          kind: "signal",
          signal_id: "sg-e",
          occurred_at: "2026-09-25T10:00:00Z",
          title: "They asked for a second phase",
        },
      ],
    };
    vi.stubGlobal("fetch", stubBackend({ suggestions: [signalled] }));
    draw(<CompanySuggestions companyId="co-1" />);

    expect(await screen.findByText("Suggested deal")).toBeTruthy();
    expect(screen.getByText("Acme GmbH: buying signals")).toBeTruthy();
    expect(
      screen.getByText("Signal: They asked for a second phase"),
    ).toBeTruthy();
  });

  it("draws nothing for an account with no suggestion", async () => {
    const fetched = stubBackend({ suggestions: [] });
    vi.stubGlobal("fetch", fetched);
    const { container } = draw(<CompanySuggestions companyId="co-1" />);
    await waitFor(() => expect(fetched).toHaveBeenCalled());
    expect(container.querySelector("[data-testid=deal-suggestion]")).toBeNull();
  });
});

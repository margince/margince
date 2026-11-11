/** @vitest-environment happy-dom */
import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { type Translator, translate } from "../../i18n";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealAside } from "./dealaside";
import { probabilityReason } from "./dealprobability";

// The probability row draws the figure as a ring. A tip beside it says why the
// deal has that figure, and the row keeps its edit.

// The signed-in member of meRoute, who owns the deal.
const VIEWER = "00000000-0000-4000-8000-000000000001";

type Receipt = components["schemas"]["Receipt"];

const deal: components["schemas"]["Deal"] = {
  id: "d1",
  name: "Fleet retrofit",
  status: "open",
  writable: true,
  version: 4,
  pipeline_id: "p1",
  stage_id: "s1",
  source: "manual",
  captured_by: "human:u",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  win_probability: 35,
  win_probability_source: "margince",
  stage_win_probability: 50,
  owner_id: VIEWER,
};

const stages = [
  {
    id: "s1",
    pipeline_id: "p1",
    name: "Proposal",
    position: 2,
    semantic: "open" as const,
    win_probability: 50,
  },
];

const QUIET = "No reply from Dana Ruiz since 2 October.";

afterEach(cleanup);

// Receipts as the deal's changes list sends them: kind and close-date
// values, never the moved fields.
const verdict: Receipt = {
  id: "r1",
  kind: "deal_probability",
  summary: 'Changed the win probability on "Fleet retrofit": 50% → 35%',
  reason: QUIET,
  occurred_at: "2026-11-01T02:00:00Z",
};
const dateOnly: Receipt = {
  id: "r2",
  kind: "close_date_correction",
  summary: 'Changed the close date on "Fleet retrofit"',
  reason: "Dana Ruiz moved the kickoff to March.",
  occurred_at: "2026-11-02T02:00:00Z",
  close_date_change: { date_changed: true, forecast_changed: false },
};

function mount(
  current: components["schemas"]["Deal"],
  changes: Receipt[] = [verdict],
) {
  const asked: string[] = [];
  installFetchStub({
    "GET /me": meRoute({ deal: ["read", "update"] }, { seat: "full" }),
    "GET /deals/d1/margince-changes": () => {
      asked.push("changes");
      return jsonResponse({ changes });
    },
  });
  render(
    <StoryProviders>
      <DealAside deal={current} stages={stages} />
    </StoryProviders>,
  );
  return asked;
}

async function tipText(): Promise<string> {
  const mark = await screen.findByRole("button", {
    name: "Why this probability",
  });
  act(() => mark.focus());
  return (await screen.findByRole("tooltip")).textContent ?? "";
}

it("quotes Margince's own reason for the figure it lowered, and keeps the edit", async () => {
  mount(deal, [dateOnly, verdict]);
  // The edit arrives with the reader's grants; the mark is focused after it.
  const edit = await screen.findByRole("button", {
    name: /^Change Probability .*35%/,
  });
  const mark = await screen.findByRole("button", {
    name: "Why this probability",
  });
  act(() => mark.focus());
  const tip = await screen.findByRole("tooltip");
  expect(await screen.findByText(QUIET, { exact: false })).toBe(tip);
  expect(tip.textContent).toBe(
    `Margince set this. ${QUIET} Without it, the stage Proposal gives 50%. Type your own figure to override it.`,
  );
  expect(edit.isConnected).toBe(true);
});

it("quotes a close-date sweep that also lowered the forecast", async () => {
  const sweep: Receipt = {
    ...dateOnly,
    reason: QUIET,
    close_date_change: { date_changed: true, forecast_changed: true },
  };
  mount(deal, [sweep]);
  await screen.findByRole("button", { name: /^Change Probability .*35%/ });
  await tipText();
  expect(await screen.findByText(QUIET, { exact: false })).toBe(
    screen.getByRole("tooltip"),
  );
});

it("gives a reader who does not own the deal the rule, and asks for no changes", async () => {
  const asked = mount({ ...deal, owner_id: "u-mia" });
  await screen.findByRole("button", { name: /^Change Probability .*35%/ });
  expect(await tipText()).toContain(
    "Margince may lower a figure nobody set by hand",
  );
  expect(asked).toEqual([]);
});

it("names the colleague who set the figure and asks Margince for nothing", async () => {
  const asked = mount({
    ...deal,
    win_probability_source: "human",
    win_probability_set_by_user_id: "u-mia",
    win_probability_set_by_name: "Mia Chen",
  });
  await screen.findByRole("button", { name: /^Change Probability .*35%/ });
  const mark = await screen.findByRole("button", {
    name: "Why this probability",
  });
  act(() => mark.focus());
  expect((await screen.findByRole("tooltip")).textContent).toBe(
    "Mia Chen set this by hand. Without it, the stage Proposal gives 50%. Clear the figure to follow the stage again.",
  );
  expect(asked).toEqual([]);
});

describe("probabilityReason", () => {
  const t: Translator = (key, params) => translate("en", key, params);

  it("says a closed deal's stage fixes the figure", () => {
    expect(
      probabilityReason(
        { ...deal, status: "won", win_probability: 100 },
        { stageName: "Won", t, locale: "en", editable: false },
      ),
    ).toBe("The deal is closed, so the stage Won fixes the figure.");
  });

  it("gives Margince's rule where no reason may be read", () => {
    expect(
      probabilityReason(deal, {
        stageName: undefined,
        t,
        locale: "en",
        editable: true,
      }),
    ).toBe(
      "Margince set this. Margince may lower a figure nobody set by hand when the deal goes quiet or its emails point lower. Without it, the deal’s stage gives 50%. Type your own figure to override it.",
    );
  });

  it("names nothing for a deal with no probability", () => {
    expect(
      probabilityReason(
        { ...deal, win_probability: undefined },
        { stageName: "Proposal", t, locale: "en", editable: true },
      ),
    ).toBeUndefined();
  });

  it("offers no change to a reader who may not make one", () => {
    expect(
      probabilityReason(
        { ...deal, win_probability_source: "stage", win_probability: 50 },
        { stageName: "Proposal", t, locale: "en", editable: false },
      ),
    ).toBe("This is the figure the stage Proposal gives.");
  });
});

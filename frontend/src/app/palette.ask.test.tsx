/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import { steppedClock } from "../testing/steppedclock";
import { type Command, CommandPalette } from "./palette";

// A question typed into the search box is asked of the handbook, and the answer
// shows above the record rows. A name is still looked up, and Enter on it
// still goes where it went before.

const HANDBOOK_ID = "00000000-0000-4000-8000-0000000000a1";
const DEFAULT_ID = "00000000-0000-4000-8000-0000000000a2";
const DOC_ID = "00000000-0000-4000-8000-0000000000b1";
const QUESTION = "How do I change my time zone?";

const ASKER: GrantSpec = { knowledge_corpus: ["read"] };

function set(over: Record<string, unknown>) {
  return {
    topic_statement: "How Margince works.",
    min_similarity: 0.35,
    default_ask: false,
    shipped: false,
    created_at: "2026-08-01T00:00:00Z",
    coverage: {
      documents_total: 1,
      chunks_total: 4,
      chunks_embedded: 4,
      embed_state: "complete",
    },
    ...over,
  };
}

const HANDBOOK = set({
  id: HANDBOOK_ID,
  name: "Margince handbook",
  shipped: true,
});
const DEFAULT = set({
  id: DEFAULT_ID,
  name: "Sales playbook",
  default_ask: true,
});

const TIME_ZONE_CLAIM = {
  chunk_id: "00000000-0000-4000-8000-0000000000c1",
  document_id: DOC_ID,
  document_name: "your-own-settings.md",
  line: 156,
  column: 1,
  text: "Pick your time zone on the Meetings page.",
  quote: "open Settings → **Meetings**",
  section: { title: "How do I change my time zone?", line: 154 },
  opens: { settings_page: "meetings" },
};

function answer(
  corpus: Record<string, unknown>,
  over: Record<string, unknown> = {},
) {
  return {
    outcome: "answered",
    generated_by: "model",
    corpus: {
      id: corpus.id,
      name: corpus.name,
      topic_statement: corpus.topic_statement,
      shipped: corpus.shipped,
    },
    coverage: (corpus as { coverage: unknown }).coverage,
    summary: "Pick your time zone on the Meetings page. [1]",
    claims: [TIME_ZONE_CLAIM],
    ...over,
  };
}

function notCovered(corpus: Record<string, unknown>, summary: string) {
  return answer(corpus, { outcome: "not_covered", summary, claims: [] });
}

function json(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function backend({
  allow = ASKER,
  sets = [HANDBOOK, DEFAULT],
  replies = {},
  setsArrive = Promise.resolve(),
}: {
  allow?: GrantSpec;
  sets?: unknown[];
  replies?: Record<string, unknown>;
  // Holds the set list back until it settles, as a slow network does.
  setsArrive?: Promise<void>;
} = {}) {
  const asked: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return json(meFixture({ allow }));
      }
      if (req.url.includes("/ask")) {
        const id = req.url.split("/corpora/")[1]?.split("/")[0] ?? "";
        asked.push(id);
        const reply = replies[id] ?? answer(HANDBOOK);
        if ((reply as { status?: number }).status === 500) {
          return new Response(
            JSON.stringify({ title: "Internal Server Error", status: 500 }),
            {
              status: 500,
              headers: { "Content-Type": "application/problem+json" },
            },
          );
        }
        return json(reply);
      }
      if (req.url.includes("/knowledge/corpora")) {
        await setsArrive;
        return json({ items: sets });
      }
      if (req.url.includes("/knowledge/documents/")) {
        return new Response("# Your own settings\n", { status: 200 });
      }
      return json({ data: [], page: { next_cursor: null, has_more: false } });
    }),
  );
  return asked;
}

const commands: Command[] = [
  {
    id: "screen:deals",
    label: "Deals",
    keywords: ["pipeline"],
    type: "screen",
    route: { screen: "deals" },
  },
];

function renderPalette(onClose = () => {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <CommandPalette open onClose={onClose} commands={commands} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function answerPanel() {
  return screen.findByRole("region", { name: "Answer from Margince" });
}

afterEach(() => {
  cleanup();
  window.location.hash = "";
  vi.unstubAllGlobals();
});

describe("asking the handbook from the search box", () => {
  it("asks a question on Enter and shows the answer with a button to its setting", async () => {
    const user = steppedClock();
    const asked = backend();
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await screen.findByRole("button", {
      name: /^Ask Margince: “How do I change my time zone\?”/,
    });
    await user.keyboard("{Enter}");
    const panel = await answerPanel();
    expect(within(panel).getByText("From Margince handbook")).toBeVisible();
    expect(
      within(panel).getByText(
        "Handbook section: How do I change my time zone?",
      ),
    ).toBeVisible();
    expect(asked).toEqual([HANDBOOK_ID]);
    await user.click(
      within(panel).getByRole("button", { name: "Open Settings → Meetings" }),
    );
    expect(window.location.hash).toBe("#/settings/meetings");
  });

  it("asks on an Enter pressed before the list of sets has arrived", async () => {
    const user = steppedClock();
    let arrive = () => {};
    const asked = backend({
      setsArrive: new Promise<void>((resolve) => {
        arrive = resolve;
      }),
    });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await screen.findByRole("button", {
      name: /^Ask Margince: “How do I change my time zone\?”/,
    });
    await user.keyboard("{Enter}");
    arrive();
    await answerPanel();
    expect(asked).toEqual([HANDBOOK_ID]);
    expect(window.location.hash).toBe("");
  });

  it("says the question was not answered when every ask fails, and keeps the rows", async () => {
    const user = steppedClock();
    backend({
      replies: {
        [HANDBOOK_ID]: { status: 500 },
        [DEFAULT_ID]: { status: 500 },
      },
    });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Question not answered")).toBeVisible();
    expect(
      screen.getByRole("button", {
        name: /^See all results for “How do I change my time zone\?”/,
      }),
    ).toBeVisible();
  });

  it("asks the default set when the handbook ask fails", async () => {
    const user = steppedClock();
    const asked = backend({
      replies: {
        [HANDBOOK_ID]: { status: 500 },
        [DEFAULT_ID]: answer(DEFAULT),
      },
    });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    const panel = await answerPanel();
    expect(within(panel).getByText("From Sales playbook")).toBeVisible();
    expect(asked).toEqual([HANDBOOK_ID, DEFAULT_ID]);
  });

  it("asks once when Enter is pressed again on the same question", async () => {
    const user = steppedClock();
    const asked = backend();
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    await answerPanel();
    await user.keyboard("{Enter}");
    await answerPanel();
    expect(asked).toEqual([HANDBOOK_ID]);
  });

  it("says there is no document set when the list turns out empty", async () => {
    const user = steppedClock();
    let arrive = () => {};
    const asked = backend({
      sets: [],
      setsArrive: new Promise<void>((resolve) => {
        arrive = resolve;
      }),
    });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await screen.findByRole("button", { name: /^Ask Margince:/ });
    await user.keyboard("{Enter}");
    arrive();
    expect(await screen.findByText("No document sets")).toBeVisible();
    expect(asked).toEqual([]);
  });

  it("keeps the record rows below the answer", async () => {
    const user = steppedClock();
    backend();
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    await answerPanel();
    expect(
      screen.getByRole("button", {
        name: /^See all results for “How do I change my time zone\?”/,
      }),
    ).toBeVisible();
  });

  it("asks the default set when the handbook does not cover the question, and says so", async () => {
    const user = steppedClock();
    const asked = backend({
      replies: {
        [HANDBOOK_ID]: notCovered(HANDBOOK, "Your handbook does not say."),
        [DEFAULT_ID]: answer(DEFAULT, {
          claims: [
            { ...TIME_ZONE_CLAIM, section: undefined, opens: undefined },
          ],
        }),
      },
    });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    const panel = await answerPanel();
    expect(
      within(panel).getByText("Not in the handbook. From Sales playbook"),
    ).toBeVisible();
    expect(asked).toEqual([HANDBOOK_ID, DEFAULT_ID]);
    expect(
      within(panel).queryByRole("button", { name: /Open Settings/ }),
    ).toBeNull();
  });

  it("says the question was not answered when the handbook does not cover it and the default set fails", async () => {
    const user = steppedClock();
    backend({
      replies: {
        [HANDBOOK_ID]: notCovered(HANDBOOK, "Your handbook does not say."),
        [DEFAULT_ID]: { status: 500 },
      },
    });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    expect(await screen.findByText("Question not answered")).toBeVisible();
    expect(
      screen.queryByText(
        "Margince answers from its handbook and your documents. Ask an administrator or a colleague.",
      ),
    ).toBeNull();
  });

  it("says who to ask when neither set covers the question", async () => {
    const user = steppedClock();
    backend({
      replies: {
        [HANDBOOK_ID]: notCovered(HANDBOOK, "Your handbook does not say."),
        [DEFAULT_ID]: notCovered(
          DEFAULT,
          "Your playbook does not cover time zones.",
        ),
      },
    });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    const panel = await answerPanel();
    expect(
      within(panel).getByText("Your playbook does not cover time zones."),
    ).toBeVisible();
    expect(
      within(panel).getByText(
        "Margince answers from its handbook and your documents. Ask an administrator or a colleague.",
      ),
    ).toBeVisible();
  });

  it("asks the handbook once when it is also the default set", async () => {
    const user = steppedClock();
    const asked = backend({
      sets: [{ ...HANDBOOK, default_ask: true }],
      replies: {
        [HANDBOOK_ID]: notCovered(HANDBOOK, "Your handbook does not say."),
      },
    });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    await answerPanel();
    expect(asked).toEqual([HANDBOOK_ID]);
  });

  it("hides the settings button for a page the reader may not open", async () => {
    const user = steppedClock();
    backend({
      replies: {
        [HANDBOOK_ID]: answer(HANDBOOK, {
          claims: [{ ...TIME_ZONE_CLAIM, opens: { settings_page: "company" } }],
        }),
      },
    });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    const panel = await answerPanel();
    expect(
      within(panel).queryByRole("button", { name: /Open Settings/ }),
    ).toBeNull();
  });

  it("drops the answer once the question in the box changes", async () => {
    const user = steppedClock();
    backend();
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    await answerPanel();
    await user.type(screen.getByRole("searchbox"), " now");
    expect(
      screen.queryByRole("region", { name: "Answer from Margince" }),
    ).toBeNull();
  });

  it("offers no ask row to a reader who may not read document sets", async () => {
    const user = steppedClock();
    backend({ allow: {} });
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await screen.findByRole("button", {
      name: /^See all results for “How do I change my time zone\?”/,
    });
    expect(screen.queryByRole("button", { name: /^Ask Margince:/ })).toBeNull();
  });

  it("still goes to the results on Enter for a name", async () => {
    const user = steppedClock();
    const asked = backend();
    renderPalette();
    await user.type(screen.getByRole("searchbox"), "Acme GmbH");
    await user.keyboard("{Enter}");
    expect(window.location.hash).toBe("#/search/Acme%20GmbH");
    expect(asked).toEqual([]);
  });

  it("opens the cited handbook page beside the answer", async () => {
    const user = steppedClock();
    backend();
    renderPalette();
    await user.type(screen.getByRole("searchbox"), QUESTION);
    await user.keyboard("{Enter}");
    const panel = await answerPanel();
    await user.click(
      within(panel).getByRole("button", {
        name: /your-own-settings\.md, line 156/,
      }),
    );
    expect(await within(panel).findByText("Your own settings")).toBeVisible();
  });
});

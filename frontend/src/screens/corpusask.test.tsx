/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { type Locale, LocaleProvider } from "../i18n";
import { AskMarginceModal } from "./corpusask";

// Ask AI → Ask your documents.
//
// This file exists for ONE property, and it is the property the whole endpoint
// was designed around: the three refusals are three different statements and
// the screen must never collapse them.
//
//   not_covered            about the QUESTION — and the reader is told what the
//                          set DOES cover, so they learn where to go next
//   not_ready              about the SET
//   retrieval_unavailable  about the INSTALLATION
//
// A surface that drew one "no answer" state for all three would undo the
// distinction the backend refuses to collapse, and the reader would go looking
// for a document to upload when the truth was that nothing was searched.
//
// The second property, which arrived with the dialog: an answer is the
// writer's own paragraph, and every sentence of it is FOLLOWABLE. A bracketed
// number becomes a control that names the document it opens and opens it
// beside the answer; a number naming a passage the answer does not carry is
// dropped rather than drawn as a button that goes nowhere.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const ASKER: GrantSpec = { knowledge_corpus: ["read"] };
const SET_ID = "00000000-0000-4000-8000-0000000000a1";
const OTHER_SET_ID = "00000000-0000-4000-8000-0000000000a2";
const DOC_ID = "00000000-0000-4000-8000-0000000000b1";
const OTHER_DOC_ID = "00000000-0000-4000-8000-0000000000b2";

const SET = {
  id: SET_ID,
  name: "How-to",
  topic_statement: "How this product is operated, day to day.",
  min_similarity: 0.35,
  default_ask: true,
  created_at: "2026-08-01T00:00:00Z",
  coverage: { documents_total: 1, chunks_total: 4, chunks_embedded: 4 },
};

// A second set, so the picker exists at all: it is offered only where there is
// a choice to make.
const OTHER_SET = {
  ...SET,
  id: OTHER_SET_ID,
  name: "Pricing",
  topic_statement: "What each plan costs and what it includes.",
  default_ask: false,
};

const KEPT = {
  chunk_id: "00000000-0000-4000-8000-0000000000c1",
  document_id: DOC_ID,
  document_name: "operating.md",
  line: 14,
  column: 3,
  text: "Captured messages are kept for 400 days.",
  quote: "kept for 400 days from the day they arrive",
};

const PURGED = {
  chunk_id: "00000000-0000-4000-8000-0000000000c2",
  document_id: OTHER_DOC_ID,
  document_name: "retention.md",
  line: 6,
  column: 1,
  text: "Older messages are purged nightly.",
  quote: "purged in the nightly sweep",
};

// The documents themselves, as the endpoint serves them: the bytes a citation
// points INTO, each holding its claim's quote verbatim so the pane has
// something real to mark.
const DOCUMENTS: Record<string, string> = {
  [DOC_ID]:
    "# Operating\n\nMessages are kept for 400 days from the day they arrive.\n",
  [OTHER_DOC_ID]:
    "# Retention\n\nAnything older than that is purged in the nightly sweep.\n",
};

// The default shape of an answered ask: one written sentence carrying one
// citation marker, which is what the model produces and what the dialog is
// built to render.
function answer(over: Record<string, unknown> = {}) {
  return {
    outcome: "answered",
    generated_by: "model",
    corpus: {
      id: SET_ID,
      name: SET.name,
      topic_statement: SET.topic_statement,
    },
    coverage: SET.coverage,
    summary: "Captured messages are kept for 400 days. [1]",
    claims: [KEPT],
    ...over,
  };
}

function backendFor(
  allow: GrantSpec,
  opts: { sets?: unknown[]; reply?: unknown } = {},
) {
  const asked: unknown[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.includes("/ask")) {
        asked.push(await req.json());
        return jsonResponse(opts.reply ?? answer());
      }
      if (req.url.includes("/knowledge/documents/")) {
        const text = DOCUMENTS[req.url.split("/").pop() ?? ""];
        return text === undefined
          ? new Response("no such document", { status: 404 })
          : new Response(text, { status: 200 });
      }
      if (req.url.includes("/knowledge/corpora")) {
        return jsonResponse({ items: opts.sets ?? [SET] });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, asked };
}

const render = (ui: ReactNode, locale: Locale = "en") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return {
    // Handed back so a case can move a grant the way the product does — by
    // re-reading /me — rather than by re-rendering the card with a prop the
    // product has no way to change.
    client,
    ...rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial={locale}>{ui}</LocaleProvider>
      </QueryClientProvider>,
    ),
  };
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  // The key tests spy on console.error, and a spy left standing is the next
  // test's silence.
  vi.restoreAllMocks();
});

type User = ReturnType<typeof userEvent.setup>;

async function askAbout(user: User, question: string) {
  await user.type(await screen.findByLabelText(/your question/i), question);
  await user.click(screen.getByRole("button", { name: /^ask$/i }));
}

/** The pane beside the answer, which is where a pressed citation lands. */
function documentPane() {
  const pane = document.querySelector(".ask-modal-doc");
  if (!(pane instanceof HTMLElement)) {
    throw new Error("the dialog drew no document pane");
  }
  return pane;
}

/** Whether a document is open at all. The pane exists only while one is. */
function documentPaneIsOpen(): boolean {
  return document.querySelector(".ask-modal-doc") !== null;
}

describe("AskMarginceModal", () => {
  it("answers with the written sentence, and names what each citation opens", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backendFor(ASKER).fetchMock);
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");

    expect(
      await screen.findByText("Captured messages are kept for 400 days."),
    ).toBeTruthy();
    // The marker became a control, and its accessible name says where it goes:
    // a bare "1" tells a reader nothing, and a reader on a screen reader gets
    // no tooltip to fall back on.
    expect(
      screen.getByRole("button", { name: "1: operating.md, line 14" }),
    ).toBeTruthy();
    // And the whole surface says who wrote the sentence, because a paragraph
    // presented without that is read as the workspace's own words.
    expect(screen.getByText("AI-assisted")).toBeTruthy();
  });

  // The numbers come from a model, so one of them can name a claim the answer
  // never carried. A button that opens nothing is worse than a sentence that is
  // merely uncited: the reader presses it, gets nothing, and stops trusting the
  // ones that do work.
  // The summary is the one sentence on this surface nothing checks: every claim
  // is welded to a verbatim quote, that is not. A passage that talks the writer
  // into a link would otherwise put a live one inside the answer panel, under
  // the badge saying a model wrote it.
  it("renders no link in the summary, whatever the writer put there", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          summary:
            "Ask an admin, or [verify your seat](https://evil.example/login). [1]",
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");

    expect(
      await screen.findByText(/verify your seat/, { exact: false }),
    ).toBeTruthy();
    expect(document.querySelector(".ask-summary a")).toBeNull();
  });

  it("drops a citation marker that names a passage the answer does not carry", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          summary:
            "Captured messages are kept for 400 days. [1] Older ones are purged nightly. [2]",
          claims: [KEPT],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");

    const written = await screen.findByText(/Older ones are purged nightly/);
    // The sentence survives whole; only the dead number is gone, and the one
    // live citation is still pressable.
    expect(written.textContent).not.toContain("[2]");
    expect(within(written).getAllByRole("button")).toHaveLength(1);
    expect(
      within(written).getByRole("button", { name: "1: operating.md, line 14" }),
    ).toBeTruthy();
  });

  // Two things in one answer can share a claim's identity: a sentence may cite
  // the same claim twice, and two claims may be quoted from ONE chunk. Keyed on
  // that identity, React saw duplicate siblings — which it warns about, and
  // which on a re-render lets it reuse the wrong element and leave the pressed
  // state sitting on a citation the reader did not press.
  it("draws every citation when one claim is cited twice", async () => {
    const user = userEvent.setup();
    const warned: unknown[] = [];
    vi.spyOn(console, "error").mockImplementation((...args) =>
      warned.push(args[0]),
    );
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          summary: "Kept for 400 days. [1] Four hundred, to be exact. [1]",
          claims: [KEPT],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");

    const written = await screen.findByText(/Four hundred, to be exact/);
    expect(within(written).getAllByRole("button")).toHaveLength(2);
    expect(warned.join(" ")).not.toContain("same key");
  });

  it("draws both claims when they were quoted from one chunk", async () => {
    const user = userEvent.setup();
    const warned: unknown[] = [];
    vi.spyOn(console, "error").mockImplementation((...args) =>
      warned.push(args[0]),
    );
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          // No summary, so the claims stand on their own as a list — which is
          // the other place the identity was the key.
          summary: "",
          claims: [KEPT, { ...KEPT, line: 15, quote: "kept for 400 days" }],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");

    expect(
      await screen.findByRole("button", { name: "1: operating.md, line 14" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "2: operating.md, line 15" }),
    ).toBeTruthy();
    expect(warned.join(" ")).not.toContain("same key");
  });

  it("opens the cited document beside the answer, and closes it again", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          summary:
            "Captured messages are kept for 400 days. [1] Older ones are purged nightly. [2]",
          claims: [KEPT, PURGED],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");

    // Nothing is open until the reader picks a citation, and the pane is not
    // drawn at all until then: an empty column spends half the dialog saying
    // nothing, and on a refusal it would say nothing for good.
    expect(documentPaneIsOpen()).toBe(false);

    await user.click(
      await screen.findByRole("button", { name: "1: operating.md, line 14" }),
    );
    // The document itself, with the quoted span marked where it was written —
    // which is the part that lets the reader judge whether the sentence is a
    // fair reading of it.
    expect(
      await within(documentPane()).findByText("operating.md"),
    ).toBeTruthy();
    await waitFor(() =>
      expect(documentPane().querySelector("mark")?.textContent).toContain(
        "kept for 400 days from the day they arrive",
      ),
    );

    // A second citation opens ITS document, not the first one again.
    await user.click(
      screen.getByRole("button", { name: "2: retention.md, line 6" }),
    );
    expect(
      await within(documentPane()).findByText("retention.md"),
    ).toBeTruthy();
    expect(within(documentPane()).queryByText("operating.md")).toBeNull();

    // And pressing the open one again gives the reader the width back.
    await user.click(
      screen.getByRole("button", { name: "2: retention.md, line 6" }),
    );
    await waitFor(() => expect(documentPaneIsOpen()).toBe(false));
  });

  // An answer belongs to the set it was asked of. useMutation keeps its last
  // result across a change of selection, so without the guard a reader who asks
  // one set and switches to another reads the FIRST set's answer under the
  // second set's name — and the citations under it point into documents the
  // named set does not hold.
  it("drops the answer when the reader moves to another set", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, { sets: [SET, OTHER_SET] }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");
    expect(
      await screen.findByText("Captured messages are kept for 400 days."),
    ).toBeTruthy();

    await user.click(screen.getByRole("combobox", { name: /document set/i }));
    await user.click(
      within(screen.getByRole("listbox")).getByRole("option", {
        name: "Pricing",
      }),
    );

    await waitFor(() =>
      expect(
        screen.queryByText("Captured messages are kept for 400 days."),
      ).toBeNull(),
    );
    expect(
      screen.queryByRole("button", { name: "1: operating.md, line 14" }),
    ).toBeNull();
  });

  // The passages, standing on their own. A deterministic answer wrote no prose,
  // and that is honest: the grounded part was never a sentence. They are still
  // followable, which is the only thing that makes them worth showing.
  it("offers the passages themselves when nothing wrote a summary", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          generated_by: "deterministic",
          summary: undefined,
          claims: [KEPT],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");

    expect(
      await screen.findByText(/Captured messages are kept for 400 days/),
    ).toBeTruthy();
    await user.click(
      screen.getByRole("button", { name: "1: operating.md, line 14" }),
    );
    expect(
      await within(documentPane()).findByText("operating.md"),
    ).toBeTruthy();
  });

  it("says in the writer's own words what the set covers instead", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          outcome: "not_covered",
          summary:
            "Nothing here priced a plan; this set is about running the product.",
          claims: [],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "what does the Professional plan cost");

    expect(await screen.findByText(/not covered by this set/i)).toBeTruthy();
    // The writer's sentence LEADS, because it answers the reader's next
    // question — where to go — in words about this question rather than in the
    // set's standing blurb.
    expect(
      screen.getByText(
        "Nothing here priced a plan; this set is about running the product.",
      ),
    ).toBeTruthy();
    // And the blurb is not printed beside it: one paragraph saying where to go,
    // not two competing for the reader's least patient moment.
    expect(
      screen.queryByText("How this product is operated, day to day."),
    ).toBeNull();
  });

  it("falls back to the set's own topic statement when no sentence was written", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          outcome: "not_covered",
          summary: undefined,
          claims: [],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "what does the Professional plan cost");

    expect(await screen.findByText(/not covered by this set/i)).toBeTruthy();
    // The statement itself, verbatim: with nothing written, it is the only
    // thing on screen that tells the reader what this set IS for.
    expect(
      screen.getByText("How this product is operated, day to day."),
    ).toBeTruthy();
  });

  it("says the SET is not ready, not that the question is uncovered", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          outcome: "not_ready",
          summary: undefined,
          claims: [],
          coverage: { documents_total: 2, chunks_total: 9, chunks_embedded: 4 },
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");

    expect(
      await screen.findByRole("heading", {
        name: "This set is still being read",
      }),
    ).toBeTruthy();
    // Nothing is wrong with the question, and the reader is not sent looking
    // for a document to file.
    expect(
      screen.getByText(/passages are searchable\. Retry shortly/i),
    ).toBeTruthy();
    expect(screen.getByText(/The question is not the problem\./)).toBeTruthy();
    expect(screen.queryByText(/not covered by this set/i)).toBeNull();
  });

  it("says nothing was searched when the installation has no index", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          outcome: "retrieval_unavailable",
          summary: undefined,
          claims: [],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "how long are messages kept");

    expect(await screen.findByText(/nothing was searched/i)).toBeTruthy();
    // Neither of the other two refusals, because neither is true: the question
    // is fine and so is the set.
    expect(screen.queryByText(/not covered by this set/i)).toBeNull();
    expect(screen.queryAllByText(/still being read/i)).toHaveLength(0);
  });

  it("says nothing read the passages when no writer was in the path", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          outcome: "unreviewed",
          generated_by: "deterministic",
          summary: undefined,
          // The passage the search ranked nearest, carrying no written
          // sentence — which is the shape a claim takes when nobody wrote one.
          claims: [{ ...KEPT, text: undefined }],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "what is the boiling point of nitrogen");

    // The reader is TOLD, above the passage, rather than left to infer it from
    // a badge — a passage presented like an answer is read as one.
    expect(await screen.findByText(/have not been reviewed/i)).toBeTruthy();
    // And the passage is still reachable: the search did find it, and throwing
    // it away would throw away the only thing the ask produced.
    await user.click(
      screen.getByRole("button", { name: "1: operating.md, line 14" }),
    );
    await waitFor(() =>
      expect(documentPane().querySelector("mark")?.textContent).toContain(
        "kept for 400 days from the day they arrive",
      ),
    );
    // It is not dressed as a refusal either. The set was searched in full and
    // the question may well be covered — nothing checked.
    expect(screen.queryByText(/not covered by this set/i)).toBeNull();
  });

  // The palette FILLS the box; it does not press Ask. A question typed into a
  // palette is one still being composed — the row matched mid-word — so asking
  // it would spend a model call on a fragment and answer something the reader
  // had not finished saying.
  it("fills the box with the question the palette carried, and waits", async () => {
    const backend = backendFor(ASKER);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(
      <AskMarginceModal
        open
        onClose={() => {}}
        carriedQuestion="how long are messages kept"
      />,
    );

    await waitFor(() =>
      expect(screen.getByLabelText(/your question/i)).toHaveValue(
        "how long are messages kept",
      ),
    );
    // Nothing was asked: the reader still has the press to make.
    expect(backend.asked).toHaveLength(0);
    expect(
      screen.queryByText("Captured messages are kept for 400 days."),
    ).toBeNull();

    await userEvent.setup().click(screen.getByRole("button", { name: "Ask" }));
    // Longer than the default second, and measured rather than guessed: this is
    // the only assertion in the file that waits on a press, a fetch and a render
    // together, and under coverage instrumentation that chain runs past 1000ms.
    // It is waiting for something that does arrive — the same assertion passes
    // uninstrumented every time — so the budget is the thing that was wrong.
    expect(
      await screen.findByText(
        "Captured messages are kept for 400 days.",
        undefined,
        {
          timeout: 5000,
        },
      ),
    ).toBeTruthy();
    expect(backend.asked).toHaveLength(1);
  });

  it("asks nothing, and reaches for no sets, for a reader who holds no grant", async () => {
    const user = userEvent.setup();
    const backend = backendFor({});
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AskMarginceModal open onClose={() => {}} />);

    // What the reader is told is that they may not OPEN these documents, not
    // that the company has none: the second is a statement about somebody
    // else's data, and it is one this reader has no way to check.
    expect(
      await screen.findByText(/cannot open this company’s documents/i),
    ).toBeTruthy();
    expect(screen.queryByText(/filed no documents yet/i)).toBeNull();
    await user.type(
      screen.getByLabelText(/your question/i),
      "how long are messages kept",
    );
    expect(screen.getByRole("button", { name: /^ask$/i })).toBeDisabled();
    expect(backend.asked).toEqual([]);
    // And it never even listed the sets. A reader with no grant would get a
    // 403, which reads as a fault in the installation rather than as a
    // permission.
    expect(
      backend.fetchMock.mock.calls.some(([input]) =>
        String(input).includes("/knowledge/corpora"),
      ),
    ).toBe(false);
  });

  // A document that will not open is said so plainly, with the quote kept: the
  // reader came to read that span, and the quote is the part we already have.
  it("says so when the cited document will not open, and keeps the quote", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          summary: "Older messages are purged nightly. [1]",
          claims: [
            { ...PURGED, document_id: "00000000-0000-4000-8000-00000000ffff" },
          ],
        }),
      }).fetchMock,
    );
    render(<AskMarginceModal open onClose={() => {}} />);
    await askAbout(user, "when are messages purged");

    await user.click(
      await screen.findByRole("button", { name: "1: retention.md, line 6" }),
    );
    expect(
      await within(documentPane()).findByText(/could not be opened/i),
    ).toBeTruthy();
    expect(
      within(documentPane()).getByText("purged in the nightly sweep"),
    ).toBeTruthy();
  });
});

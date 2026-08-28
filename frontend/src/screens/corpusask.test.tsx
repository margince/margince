/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { type Locale, LocaleProvider } from "../i18n";
import { CorpusAskCard } from "./corpusask";

// Ask AI → Ask your documents.
//
// This file exists for ONE property, and it is the property the whole endpoint
// was designed around: the three refusals are three different statements and
// the screen must never collapse them.
//
//   not_covered            about the QUESTION — and the set's topic statement
//                          is quoted back, so the reader learns what it is for
//   not_ready              about the SET
//   retrieval_unavailable  about the INSTALLATION
//
// A surface that drew one "no answer" state for all three would undo the
// distinction the backend refuses to collapse, and the reader would go looking
// for a document to upload when the truth was that nothing was searched.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const ASKER: GrantSpec = { knowledge_corpus: ["read"] };
const SET_ID = "00000000-0000-4000-8000-0000000000a1";

const SET = {
  id: SET_ID,
  name: "How-to",
  topic_statement: "How this product is operated, day to day.",
  min_similarity: 0.35,
  default_ask: true,
  created_at: "2026-08-01T00:00:00Z",
  coverage: { documents_total: 1, chunks_total: 4, chunks_embedded: 4 },
};

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
    claims: [
      {
        chunk_id: "00000000-0000-4000-8000-0000000000c1",
        document_id: "00000000-0000-4000-8000-0000000000b1",
        document_name: "operating.md",
        line: 14,
        column: 3,
        text: "Captured messages are kept for 400 days.",
        quote: "kept for 400 days from the day they arrive",
      },
    ],
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
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function askAbout(question: string) {
  await userEvent.type(
    await screen.findByLabelText(/your question/i),
    question,
  );
  await userEvent.click(screen.getByRole("button", { name: /^ask$/i }));
}

describe("CorpusAskCard", () => {
  it("answers with the sentence and the passage it rests on", async () => {
    vi.stubGlobal("fetch", backendFor(ASKER).fetchMock);
    render(<CorpusAskCard />);
    await askAbout("how long are messages kept");

    expect(
      await screen.findByText("Captured messages are kept for 400 days."),
    ).toBeTruthy();
    // The quote and the file it came from are BOTH on screen. A sentence
    // without them is a claim a reader cannot check, which is the whole thing
    // this endpoint refuses to produce.
    expect(
      screen.getByText("kept for 400 days from the day they arrive"),
    ).toBeTruthy();
    expect(screen.getByText("operating.md")).toBeTruthy();
  });

  it("says a model wrote the sentences", async () => {
    vi.stubGlobal("fetch", backendFor(ASKER).fetchMock);
    render(<CorpusAskCard />);
    await askAbout("how long are messages kept");

    expect(await screen.findByText(/written from the passages/i)).toBeTruthy();
  });

  it("says nobody wrote a summary when the answer is the passages themselves", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          generated_by: "deterministic",
          claims: [
            {
              chunk_id: "00000000-0000-4000-8000-0000000000c1",
              document_id: "00000000-0000-4000-8000-0000000000b1",
              document_name: "operating.md",
              quote: "kept for 400 days from the day they arrive",
            },
          ],
        }),
      }).fetchMock,
    );
    render(<CorpusAskCard />);
    await askAbout("how long are messages kept");

    // The reader is TOLD which writer produced this, rather than being left to
    // infer it from the absence of a sentence.
    expect(await screen.findByText(/nobody wrote a summary/i)).toBeTruthy();
    expect(
      screen.getByText("kept for 400 days from the day they arrive"),
    ).toBeTruthy();
  });

  it("quotes the set's own topic statement back when it does not cover the question", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({ outcome: "not_covered", claims: [] }),
      }).fetchMock,
    );
    render(<CorpusAskCard />);
    await askAbout("what does the Professional plan cost");

    expect(await screen.findByText(/not covered by this set/i)).toBeTruthy();
    // The statement itself, verbatim: it is the only thing on screen that tells
    // the reader what this set IS for, and they are reading it at their least
    // patient moment.
    expect(
      screen.getByText("How this product is operated, day to day."),
    ).toBeTruthy();
  });

  it("says the SET is not ready, not that the question is uncovered", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          outcome: "not_ready",
          claims: [],
          coverage: { documents_total: 2, chunks_total: 9, chunks_embedded: 4 },
        }),
      }).fetchMock,
    );
    render(<CorpusAskCard />);
    await askAbout("how long are messages kept");

    expect(await screen.findByText(/not finished being read/i)).toBeTruthy();
    // And it says how far it got, so "try again shortly" is a claim the reader
    // can check rather than a hope.
    expect(screen.getByText(/4 of 9 passages/i)).toBeTruthy();
    expect(screen.queryByText(/not covered by this set/i)).toBeNull();
  });

  it("says nothing was searched when the installation has no index", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({ outcome: "retrieval_unavailable", claims: [] }),
      }).fetchMock,
    );
    render(<CorpusAskCard />);
    await askAbout("how long are messages kept");

    expect(await screen.findByText(/nothing was searched/i)).toBeTruthy();
    // Neither of the other two refusals, because neither is true: the question
    // is fine and so is the set.
    expect(screen.queryByText(/not covered by this set/i)).toBeNull();
    expect(screen.queryByText(/not finished being read/i)).toBeNull();
  });

  it("says nothing read the passages when no writer was in the path", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          outcome: "unreviewed",
          generated_by: "deterministic",
          // The passage the search ranked nearest, carrying no written
          // sentence — which is the shape a claim takes when nobody wrote one.
          claims: [
            {
              chunk_id: "00000000-0000-4000-8000-0000000000c1",
              document_id: "00000000-0000-4000-8000-0000000000b1",
              document_name: "operating.md",
              line: 14,
              column: 3,
              quote: "kept for 400 days from the day they arrive",
            },
          ],
        }),
      }).fetchMock,
    );
    render(<CorpusAskCard />);
    await askAbout("what is the boiling point of nitrogen");

    // The reader is TOLD, above the passage, rather than left to infer it from
    // a badge — a passage presented like an answer is read as one.
    expect(
      await screen.findByText(/nothing has read these passages/i),
    ).toBeTruthy();
    // And the passage is still on screen: the search did find it, and hiding it
    // would throw away the only thing the ask produced.
    expect(
      screen.getByText(/kept for 400 days from the day they arrive/i),
    ).toBeTruthy();
    // It is not dressed as a refusal either. The set was searched in full and
    // the question may well be covered — nothing checked.
    expect(screen.queryByText(/not covered by this set/i)).toBeNull();
  });

  it("asks the question the palette carried, without making the reader retype it", async () => {
    const backend = backendFor(ASKER);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<CorpusAskCard carriedQuestion="how long are messages kept" />);

    // Already in the box, not printed beside an empty one.
    const box = await screen.findByLabelText(/your question/i);
    expect(box).toHaveValue("how long are messages kept");
    await userEvent.click(screen.getByRole("button", { name: /^ask$/i }));
    expect(backend.asked).toEqual([{ question: "how long are messages kept" }]);
  });

  // The positive half is what makes the negative half mean anything.
  //
  // Asserting only "the card is absent" passes BEFORE the list has arrived, so
  // written that way it stayed green against a set that was present — the case
  // was passing and empty, twice, until it was checked against its own
  // opposite. Rendering with a set first proves the query path reaches the DOM;
  // the rejection that follows is then a statement about the empty list.
  it("offers the box for a set, and nothing at all when there is none", async () => {
    vi.stubGlobal("fetch", backendFor(ASKER).fetchMock);
    render(<CorpusAskCard />);
    expect(await screen.findByLabelText(/your question/i)).toBeTruthy();

    cleanup();
    vi.unstubAllGlobals();
    vi.stubGlobal("fetch", backendFor(ASKER, { sets: [] }).fetchMock);
    render(<CorpusAskCard />);

    // A box that answers nothing is worse than no box: a reader offered an
    // input has been told a question will be answered.
    await expect(screen.findByLabelText(/your question/i)).rejects.toThrow();
  });

  it("offers nothing to a reader who holds no grant on document sets", async () => {
    const backend = backendFor({});
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<CorpusAskCard />);

    await expect(screen.findByLabelText(/your question/i)).rejects.toThrow();
    // And it never asked. A reader with no grant would get a 403, which reads
    // as a fault in the installation rather than as a permission.
    expect(
      backend.fetchMock.mock.calls.some(([input]) =>
        String(input).includes("/knowledge/corpora"),
      ),
    ).toBe(false);
  });

  it("offers the cited document as a download, and says where in it the quote sits", async () => {
    vi.stubGlobal("fetch", backendFor(ASKER).fetchMock);
    render(<CorpusAskCard />);
    await askAbout("how long are messages kept");

    // The FILE, downloadable. A citation nobody can follow is a citation in
    // name only: the reader has the sentence and the quote, and this is what
    // lets them open the document and see it in place.
    const link = await screen.findByRole("link", { name: /operating\.md/i });
    expect(link).toHaveAttribute(
      "href",
      "/v1/knowledge/documents/00000000-0000-4000-8000-0000000000b1",
    );
    expect(link).toHaveAttribute("download", "operating.md");

    // And WHERE in it, so following the link lands on the sentence rather than
    // on the top of a file the reader then has to search.
    expect(screen.getByText(/line 14, column 3/i)).toBeTruthy();
  });

  it("says nothing about where when the passage could not locate the quote", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ASKER, {
        reply: answer({
          claims: [
            {
              chunk_id: "00000000-0000-4000-8000-0000000000c1",
              document_id: "00000000-0000-4000-8000-0000000000b1",
              document_name: "operating.md",
              text: "Messages are kept for 400 days.",
              quote: "kept for 400 days from the day they arrive",
            },
          ],
        }),
      }).fetchMock,
    );
    render(<CorpusAskCard />);
    await askAbout("how long are messages kept");

    // The download still stands — the file is openable whether or not the line
    // is known. Only the location is withheld, because a line number pointing
    // at the wrong line is worse than none.
    expect(
      await screen.findByRole("link", { name: /operating\.md/i }),
    ).toBeTruthy();
    expect(screen.queryByText(/line \d/i)).toBeNull();
  });
});

/** @vitest-environment jsdom */
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
import { KnowledgeCard } from "./knowledge";

// Settings → Knowledge → Document sets.
//
// Two properties this file exists for, and both are about telling a reader the
// truth rather than about the happy path.
//
// A reader who may ASK but not administer sees the sets and NO verbs — the
// seeded posture, where read is the ask and every role that reads records holds
// it. And the three states a set or a document can be in that are not "fine"
// each say something DIFFERENT: being re-read, a threshold that is now a
// leftover, and a file that could not be read at all. Collapsing any two of
// them is the failure the backend spent a whole design avoiding, and a screen
// that renders one badge for all three would undo it here.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const ADMIN: GrantSpec = {
  knowledge_corpus: ["create", "read", "update", "delete"],
  knowledge_document: ["create", "read", "update", "delete"],
};
const ASKER: GrantSpec = {
  knowledge_corpus: ["read"],
  knowledge_document: ["read"],
};
const OUTSIDER: GrantSpec = {};

const SET_ID = "00000000-0000-4000-8000-0000000000a1";
const DOC_ID = "00000000-0000-4000-8000-0000000000b1";

function corpus(over: Record<string, unknown> = {}) {
  return {
    id: SET_ID,
    name: "How-to",
    topic_statement: "How this product is operated.",
    min_similarity: 0.35,
    default_ask: false,
    created_at: "2026-08-01T00:00:00Z",
    coverage: { documents_total: 1, chunks_total: 4, chunks_embedded: 4 },
    ...over,
  };
}

function document(over: Record<string, unknown> = {}) {
  return {
    id: DOC_ID,
    corpus_id: SET_ID,
    filename: "operating.md",
    content_type: "text/markdown",
    byte_size: 128,
    ingest_status: "done",
    chunk_count: 4,
    created_at: "2026-08-01T00:00:00Z",
    ...over,
  };
}

function backendFor(
  allow: GrantSpec,
  opts: {
    sets?: ReturnType<typeof corpus>[];
    documents?: ReturnType<typeof document>[];
  } = {},
) {
  const posts: string[] = [];
  const deletes: string[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.includes("/knowledge/documents/")) {
        deletes.push(req.url);
        return new Response(null, { status: 204 });
      }
      if (req.url.includes("/documents")) {
        if (req.method === "POST") {
          posts.push(req.url);
          return jsonResponse(document({ ingest_status: "queued" }), 202);
        }
        return jsonResponse({ items: opts.documents ?? [document()] });
      }
      if (req.url.includes("/knowledge/corpora")) {
        if (req.method === "POST") {
          posts.push(req.url);
          return jsonResponse(corpus(), 201);
        }
        if (req.method === "DELETE") {
          deletes.push(req.url);
          return new Response(null, { status: 204 });
        }
        return jsonResponse({ items: opts.sets ?? [corpus()] });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, posts, deletes };
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

describe("KnowledgeCard", () => {
  it("names each set and says how much of it is searchable", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    render(<KnowledgeCard />);

    expect(await screen.findByText("How-to")).toBeTruthy();
    // The topic statement is on screen because it is what a refusal quotes
    // back: an administrator who cannot see what they wrote cannot fix it.
    expect(screen.getByText("How this product is operated.")).toBeTruthy();
    expect(await screen.findByText(/4 of 4 passages/i)).toBeTruthy();
  });

  it("offers no verbs to a reader who may ask but not administer", async () => {
    vi.stubGlobal("fetch", backendFor(ASKER).fetchMock);
    render(<KnowledgeCard />);

    // The set is visible: read IS the ask, and every seeded role holds it.
    expect(await screen.findByText("How-to")).toBeTruthy();
    // And nothing that changes it is offered.
    expect(screen.queryByRole("button", { name: /archive set/i })).toBeNull();
    expect(screen.queryByRole("button", { name: /create set/i })).toBeNull();
  });

  it("withholds the list from a reader with no grant, rather than showing an empty one", async () => {
    vi.stubGlobal("fetch", backendFor(OUTSIDER).fetchMock);
    render(<KnowledgeCard />);

    // "Not yours to see" and "there are none" are different statements, and an
    // empty card would make a claim about the DATA that nobody checked.
    expect(await screen.findByText(/not yours to see/i)).toBeTruthy();
    expect(screen.queryByText("How-to")).toBeNull();
  });

  it("says a set is being re-read rather than that it is not ready", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, { sets: [corpus({ reindexing: true })] }).fetchMock,
    );
    render(<KnowledgeCard />);

    // There is nothing for the reader to do but wait, and the copy has to say
    // so — "not ready" would send them looking for a document to finish
    // uploading.
    expect(await screen.findByText(/being re-read/i)).toBeTruthy();
  });

  it("says a threshold is a leftover when it was tuned against another index", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, { sets: [corpus({ tuning_stale: true })] }).fetchMock,
    );
    render(<KnowledgeCard />);

    expect(await screen.findByText(/leftover/i)).toBeTruthy();
    // And it is NOT the re-reading message: the two are different facts about
    // the set, and only one of them is the reader's to act on.
    expect(screen.queryByText(/being re-read/i)).toBeNull();
  });

  it("names the file that could not be read, and why", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        documents: [
          document({
            ingest_status: "failed",
            ingest_detail: "This document could not be read into passages.",
          }),
        ],
      }).fetchMock,
    );
    render(<KnowledgeCard />);

    await userEvent.click(
      await screen.findByRole("button", { name: /show documents/i }),
    );

    expect(await screen.findByText("operating.md")).toBeTruthy();
    // The STATE and the REASON are two separate things on screen, and the case
    // asserts both: the badge alone would leave a reader knowing a file failed
    // and not which remedy it wants.
    expect(screen.getByText("Could not be read")).toBeTruthy();
    expect(
      screen.getByText("This document could not be read into passages."),
    ).toBeTruthy();
  });

  it("refuses to create a set without both a name and a topic statement", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    render(<KnowledgeCard />);

    const create = await screen.findByRole("button", { name: /create set/i });
    expect(create).toBeDisabled();

    await userEvent.type(await screen.findByLabelText(/^name$/i), "Handbook");
    // Still refused: a set with no topic statement has nothing to quote back
    // when it refuses a question, which is the one moment it is read.
    expect(create).toBeDisabled();

    await userEvent.type(
      screen.getByLabelText(/what this set covers/i),
      "How the product is operated.",
    );
    expect(create).toBeEnabled();
  });

  it("asks before archiving a set, and sends nothing until the reader agrees", async () => {
    const backend = backendFor(ADMIN);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<KnowledgeCard />);

    await userEvent.click(
      await screen.findByRole("button", { name: /archive set/i }),
    );
    expect(backend.deletes).toHaveLength(0);

    // The dialog's own confirm, not the row button that opened it: both carry
    // the same verb, which is right on screen and ambiguous to a query.
    await userEvent.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^archive set$/i,
      }),
    );
    await waitFor(() => expect(backend.deletes.length).toBeGreaterThan(0));
  });

  it("says what a document may be before one is chosen", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    render(<KnowledgeCard />);

    await userEvent.click(
      await screen.findByRole("button", { name: /show documents/i }),
    );
    // Named up front rather than after a refusal: there is no reader for a PDF
    // here, and a person who has just watched an upload fail has learned it the
    // expensive way.
    expect(
      await screen.findByText(/plain text, markdown, csv or json/i),
    ).toBeTruthy();
  });
});

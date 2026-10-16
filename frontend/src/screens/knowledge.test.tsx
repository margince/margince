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
import { type Locale, LocaleProvider } from "../i18n";
import { type GrantSpec, meFixture } from "../testing/mefixture";
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

// The server's own detail is English; the screen words each code itself.
function shippedProblem(code: string) {
  return jsonResponse(
    { type: "about:blank", title: "Conflict", status: 409, code, detail: code },
    409,
  );
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

function document(over: Record<string, unknown> = {}) {
  return {
    id: DOC_ID,
    corpus_id: SET_ID,
    filename: "operating.md",
    title: null,
    shipped: false,
    content_type: "text/markdown",
    byte_size: 128,
    ingest_status: "done",
    chunk_count: 4,
    chunks_embedded: 4,
    embed_state: "complete",
    created_at: "2026-08-01T00:00:00Z",
    ...over,
  };
}

function backendFor(
  allow: GrantSpec,
  opts: {
    sets?: ReturnType<typeof corpus>[];
    documents?: ReturnType<typeof document>[];
    refuse?: Partial<Record<"archive" | "upload" | "delete", () => Response>>;
  } = {},
) {
  const posts: string[] = [];
  const deletes: string[] = [];
  let sets = opts.sets ?? [corpus()];
  let documents = opts.documents ?? [document()];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.includes("/knowledge/documents/")) {
        deletes.push(req.url);
        if (opts.refuse?.delete) return opts.refuse.delete();
        documents = documents.filter((doc) => !req.url.endsWith(doc.id));
        return new Response(null, { status: 204 });
      }
      if (req.url.includes("/documents")) {
        if (req.method === "POST") {
          posts.push(req.url);
          if (opts.refuse?.upload) return opts.refuse.upload();
          return jsonResponse(document({ ingest_status: "queued" }), 202);
        }
        return jsonResponse({ items: documents });
      }
      if (req.url.includes("/knowledge/corpora")) {
        if (req.method === "POST") {
          posts.push(req.url);
          return jsonResponse(corpus(), 201);
        }
        if (req.method === "DELETE") {
          deletes.push(req.url);
          if (opts.refuse?.archive) return opts.refuse.archive();
          sets = sets.filter((set) => !req.url.endsWith(set.id));
          return new Response(null, { status: 204 });
        }
        return jsonResponse({ items: sets });
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
  it("names each set with one searchability badge, and its figures once opened", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    const row = await screen.findByTestId(`knowledge-set-${SET_ID}`);
    expect(within(row).getByText("How-to")).toBeTruthy();
    expect(within(row).getByText("Searchable")).toBeTruthy();
    expect(screen.queryByText(/4 of 4 passages/i)).toBeNull();
    await user.click(
      within(row).getByRole("button", { name: "Documents in How-to" }),
    );
    expect(await screen.findByText(/4 of 4 passages/i)).toBeTruthy();
    expect(screen.getByText("How this product is operated.")).toBeTruthy();
  });

  it("does not call a read document searchable while none of its set's passages are", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        sets: [
          corpus({
            coverage: {
              documents_total: 1,
              chunks_total: 4,
              chunks_embedded: 0,
              embed_state: "no_embedding_model",
            },
          }),
        ],
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    expect(await screen.findByText(/0 of 4 passages searchable/)).toBeTruthy();
    // The set says why nothing is searchable, and a read document draws no
    // green "Imported" beside it to contradict that.
    expect(screen.getByText("Not searchable")).toBeTruthy();
    expect(screen.getByText(/no embedding model is set up/i)).toBeTruthy();
    expect(await screen.findByText("operating.md")).toBeTruthy();
    expect(screen.queryByText("Imported")).toBeNull();
  });

  it("offers no verbs to a reader who may ask but not administer", async () => {
    vi.stubGlobal("fetch", backendFor(ASKER).fetchMock);
    render(<KnowledgeCard />);

    // The set is visible: read IS the ask, and every seeded role holds it.
    expect(await screen.findByText("How-to")).toBeTruthy();
    // And nothing that changes it is offered.
    expect(
      screen.queryByRole("button", { name: "Actions for How-to" }),
    ).toBeNull();
    expect(
      screen.queryByRole("button", { name: "New document set" }),
    ).toBeNull();
  });

  it("says no set is filed in one line, with no table of headings", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN, { sets: [] }).fetchMock);
    render(<KnowledgeCard />);

    expect(await screen.findByText("No document sets yet.")).toBeTruthy();
    expect(screen.queryByRole("table")).toBeNull();
  });

  it("states why a set is not searchable on its own line under the figures", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        sets: [
          corpus({
            coverage: {
              documents_total: 1,
              chunks_total: 4,
              chunks_embedded: 0,
              embed_state: "indexing",
            },
          }),
        ],
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    const reason = await screen.findByText("Passages are still being indexed.");
    expect(reason.textContent).toBe("Passages are still being indexed.");
  });

  it("withholds the list from a reader with no grant, rather than showing an empty one", async () => {
    vi.stubGlobal("fetch", backendFor(OUTSIDER).fetchMock);
    render(<KnowledgeCard />);

    // "Not yours to see" and "there are none" are different statements, and an
    // empty card would make a claim about the DATA that nobody checked.
    expect(
      await screen.findByText(
        /do not have access to the list of document sets/i,
      ),
    ).toBeTruthy();
    expect(screen.queryByText("How-to")).toBeNull();
  });

  it("says a set is being re-read rather than that it is not ready", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, { sets: [corpus({ reindexing: true })] }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    // There is nothing for the reader to do but wait, and the copy has to say
    // so — "not ready" would send them looking for a document to finish
    // uploading.
    const row = await screen.findByTestId(`knowledge-set-${SET_ID}`);
    expect(within(row).getByText("Reindexing")).toBeTruthy();
    expect(within(row).queryByText("Not searchable")).toBeNull();
    await user.click(
      within(row).getByRole("button", { name: "Documents in How-to" }),
    );
    expect(await screen.findByText(/reindexing this set/i)).toBeTruthy();
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
      await screen.findByRole("button", { name: "Documents in How-to" }),
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
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "New document set" }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    const create = dialog.getByRole("button", { name: "Create" });
    expect(create).toBeDisabled();

    await user.type(dialog.getByLabelText(/^name/i), "Handbook");
    // Still refused: a set with no topic statement has nothing to quote back
    // when it refuses a question, which is the one moment it is read.
    expect(create).toBeDisabled();

    await user.type(
      dialog.getByLabelText(/what this set covers/i),
      "How the product is operated.",
    );
    expect(create).toBeEnabled();
  });

  it("asks before archiving a set, and sends nothing until the reader agrees", async () => {
    const backend = backendFor(ADMIN);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<KnowledgeCard />);

    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Actions for How-to" }),
    );
    await user.click(screen.getByRole("button", { name: /archive set/i }));
    expect(
      within(screen.getByRole("dialog")).getByText("Archive How-to?"),
    ).toBeTruthy();
    expect(backend.deletes).toHaveLength(0);

    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^archive set$/i,
      }),
    );
    await waitFor(() => expect(backend.deletes.length).toBeGreaterThan(0));
  });

  // The archived row takes its menu with it, so focus cannot go back there.
  it("hands focus to the card's title once a set is archived", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    render(<KnowledgeCard />);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "Actions for How-to" }),
    );
    await user.click(screen.getByRole("button", { name: /archive set/i }));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: /^archive set$/i,
      }),
    );
    await waitFor(() => expect(screen.queryByText("How-to")).toBeNull());
    expect(globalThis.document.activeElement?.textContent).toBe(
      "Document sets",
    );
  });

  it("returns focus to the menu when the archive is called off", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    render(<KnowledgeCard />);
    const user = userEvent.setup();
    const menu = await screen.findByRole("button", {
      name: "Actions for How-to",
    });
    await user.click(menu);
    await user.click(screen.getByRole("button", { name: /archive set/i }));
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Cancel",
      }),
    );
    await waitFor(() => expect(globalThis.document.activeElement).toBe(menu));
  });

  it("keeps two files that share a name, because the server dedupes on content", async () => {
    // The client used to drop the second file whose NAME matched an earlier
    // one. The server refuses a duplicate by CHECKSUM and names the document
    // already holding those bytes, so two notes.md from different folders are
    // two documents this product accepts. A client stricter than the thing it
    // is a client of — and silent about it — loses a reader's file with nothing
    // on screen to say so.
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    render(<KnowledgeCard />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    const input = await screen.findByLabelText(/add document/i);
    await userEvent.upload(
      input,
      new File(["first folder"], "notes.md", { type: "text/markdown" }),
    );
    await userEvent.upload(
      input,
      new File(["second folder, different bytes"], "notes.md", {
        type: "text/markdown",
      }),
    );

    // The button's own count is what the reader checks before pressing it, so
    // it is what this asserts: two held, not one.
    expect(
      await screen.findByRole("button", { name: /add 2 documents/i }),
    ).toBeTruthy();
  });

  it("says what a document may be before one is chosen", async () => {
    vi.stubGlobal("fetch", backendFor(ADMIN).fetchMock);
    render(<KnowledgeCard />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    // Named up front rather than after a refusal: there is no reader for a PDF
    // here, and a contact who has just watched an upload fail has learned it the
    // expensive way.
    expect(
      await screen.findByText(/plain text, markdown, csv or json/i),
    ).toBeTruthy();
  });
});

describe("KnowledgeCard documents", () => {
  it("lists a set's documents A to Z with their passages and size", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        documents: [
          document({ id: "b", filename: "zeta.md", chunk_count: 12 }),
          document({ id: "a", filename: "alpha.md", byte_size: 2_048 }),
        ],
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    const table = await screen.findByRole("table", {
      name: "Documents in How-to",
    });
    const names = within(table)
      .getAllByRole("row")
      .slice(1)
      .map((row) => within(row).getAllByRole("cell")[0]?.textContent);
    expect(names).toEqual(["alpha.md", "zeta.md"]);
    expect(within(table).getByText("2 kB")).toBeTruthy();
  });

  it("asks before deleting a document, naming it", async () => {
    const backend = backendFor(ADMIN);
    vi.stubGlobal("fetch", backend.fetchMock);
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Actions for operating.md" }),
    );
    await user.click(screen.getByRole("button", { name: "Delete" }));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Delete operating.md?")).toBeTruthy();
    expect(backend.deletes).toHaveLength(0);
    await user.click(dialog.getByRole("button", { name: "Delete" }));
    await waitFor(() => expect(backend.deletes).toHaveLength(1));
    await waitFor(() => expect(screen.queryByText("operating.md")).toBeNull());
    expect(globalThis.document.activeElement).toBe(
      screen.getByRole("button", { name: "Documents in How-to" }),
    );
  });

  it("names a shipped page by its title, its file under it where it differs, and offers no delete", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        documents: [
          document({ id: "u", filename: "pricing.md" }),
          document({
            id: "s",
            filename: "approvals.md",
            title: "Approving agent proposals",
            shipped: true,
          }),
          document({
            id: "t",
            filename: "your-week.md",
            title: "Your week",
            shipped: true,
          }),
        ],
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    const table = await screen.findByRole("table", {
      name: "Documents in How-to",
    });
    const rowOf = (text: string) =>
      within(table)
        .getAllByRole("row")
        .find((row) => row.textContent?.includes(text));
    const shipped = rowOf("Approving agent proposals");
    const uploaded = rowOf("pricing.md");
    const sameName = rowOf("Your week");
    if (!shipped || !uploaded || !sameName) {
      throw new Error("expected three document rows");
    }
    expect(within(shipped).getByText("approvals.md")).toHaveClass("t-caption");
    expect(within(shipped).queryByRole("button")).toBeNull();
    // A file named after its title adds nothing under it.
    expect(within(sameName).queryByText("your-week.md")).toBeNull();
    // An upload has no title: its file is its name, once.
    expect(within(uploaded).getAllByText("pricing.md")).toHaveLength(1);
    expect(
      within(uploaded).getByRole("button", { name: "Actions for pricing.md" }),
    ).toBeInTheDocument();
  });

  it("marks each document that is not searchable when its set's rows differ", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        documents: [
          document({ id: "a", filename: "a-searchable.md" }),
          document({
            id: "b",
            filename: "b-unbound.md",
            chunks_embedded: 0,
            embed_state: "no_embedding_model",
          }),
          document({
            id: "c",
            filename: "c-partial.md",
            chunks_embedded: 1,
            embed_state: "indexing",
          }),
          document({
            id: "d",
            filename: "d-queued.md",
            ingest_status: "queued",
            chunks_embedded: 0,
            embed_state: "indexing",
          }),
        ],
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    const table = await screen.findByRole("table", {
      name: "Documents in How-to",
    });
    const status = (filename: string) => {
      const row = within(table)
        .getAllByRole("row")
        .find((one) => one.textContent?.includes(filename));
      return [...(row?.querySelectorAll(".badge") ?? [])].map(
        (badge) => badge.textContent,
      );
    };
    expect(status("a-searchable.md")).toEqual([]);
    expect(status("b-unbound.md")).toEqual(["Not searchable"]);
    expect(status("c-partial.md")).toEqual(["Indexing"]);
    // Still importing says so, and only so.
    expect(status("d-queued.md")).toEqual(["Queued"]);
  });

  it("repeats no searchability state that its set states for every row", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        sets: [
          corpus({
            coverage: {
              documents_total: 2,
              chunks_total: 8,
              chunks_embedded: 0,
              embed_state: "no_embedding_model",
            },
          }),
        ],
        documents: [
          document({
            id: "a",
            filename: "a-unbound.md",
            chunks_embedded: 0,
            embed_state: "no_embedding_model",
          }),
          document({
            id: "b",
            filename: "b-queued.md",
            ingest_status: "queued",
            chunks_embedded: 0,
            embed_state: "no_embedding_model",
          }),
        ],
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    const table = await screen.findByRole("table", {
      name: "Documents in How-to",
    });
    const badges = [...table.querySelectorAll(".badge")].map(
      (badge) => badge.textContent,
    );
    expect(badges).toEqual(["Queued"]);
    // The set says it once, with its reason, above its documents.
    expect(screen.getAllByText("Not searchable")).toHaveLength(1);
  });

  it("marks the one stale document under a set still indexing", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        sets: [
          corpus({
            coverage: {
              documents_total: 2,
              chunks_total: 8,
              chunks_embedded: 5,
              embed_state: "indexing",
            },
          }),
        ],
        documents: [
          document({ id: "a", filename: "a-searchable.md" }),
          document({
            id: "b",
            filename: "b-stale.md",
            chunks_embedded: 1,
            embed_state: "indexing",
          }),
        ],
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    const table = await screen.findByRole("table", {
      name: "Documents in How-to",
    });
    const stale = within(table)
      .getAllByRole("row")
      .find((row) => row.textContent?.includes("b-stale.md"));
    expect(stale && within(stale).getByText("Indexing")).toBeInTheDocument();
    expect(table.querySelectorAll(".badge")).toHaveLength(1);
  });

  it("says why a delete was refused when the page turned out to ship", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        refuse: { delete: () => shippedProblem("document_shipped") },
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    await user.click(
      await screen.findByRole("button", { name: "Actions for operating.md" }),
    );
    await user.click(screen.getByRole("button", { name: "Delete" }));
    const dialog = within(screen.getByRole("dialog"));
    await user.click(dialog.getByRole("button", { name: "Delete" }));
    expect(
      await dialog.findByText(
        "This page ships with Margince, so it cannot be deleted.",
      ),
    ).toBeInTheDocument();
  });
});

describe("KnowledgeCard, a set Margince ships", () => {
  it("offers no archive and no upload on it", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, { sets: [corpus({ shipped: true })] }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    await screen.findByRole("table", { name: "Documents in How-to" });
    expect(
      screen.queryByRole("button", { name: "Actions for How-to" }),
    ).toBeNull();
    expect(screen.queryByLabelText(/add document/i)).toBeNull();
  });

  it("says why an archive was refused when the set turned out to ship", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        refuse: { archive: () => shippedProblem("corpus_shipped") },
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Actions for How-to" }),
    );
    await user.click(screen.getByRole("button", { name: /archive set/i }));
    const dialog = within(screen.getByRole("dialog"));
    await user.click(dialog.getByRole("button", { name: /^archive set$/i }));
    expect(
      await dialog.findByText(
        "This set ships with Margince, so it cannot be archived.",
      ),
    ).toBeInTheDocument();
  });

  it("says why an upload was refused when the set turned out to ship", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(ADMIN, {
        refuse: { upload: () => shippedProblem("corpus_shipped") },
      }).fetchMock,
    );
    const user = userEvent.setup();
    render(<KnowledgeCard />);

    await user.click(
      await screen.findByRole("button", { name: "Documents in How-to" }),
    );
    await user.upload(
      await screen.findByLabelText(/add document/i),
      new File(["notes"], "notes.md", { type: "text/markdown" }),
    );
    await user.click(screen.getByRole("button", { name: "Add document" }));
    expect(
      await screen.findByText(
        "notes.md: This set ships with Margince, so only Margince adds documents to it.",
      ),
    ).toBeInTheDocument();
  });
});

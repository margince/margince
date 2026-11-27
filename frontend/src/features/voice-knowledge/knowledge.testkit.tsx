// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender } from "@testing-library/react";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { type Locale, LocaleProvider } from "../../i18n";
import { type GrantSpec, meFixture } from "../../testing/mefixture";

// The fake server and fixtures the Knowledge card's tests share.

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The server's own detail is English; the screen words each code itself.
export function shippedProblem(code: string) {
  return jsonResponse(
    { type: "about:blank", title: "Conflict", status: 409, code, detail: code },
    409,
  );
}

export const ADMIN: GrantSpec = {
  knowledge_corpus: ["create", "read", "update", "delete"],
  knowledge_document: ["create", "read", "update", "delete"],
};
export const ASKER: GrantSpec = {
  knowledge_corpus: ["read"],
  knowledge_document: ["read"],
};
export const OUTSIDER: GrantSpec = {};

export const SET_ID = "00000000-0000-4000-8000-0000000000a1";
const DOC_ID = "00000000-0000-4000-8000-0000000000b1";

export function corpus(over: Record<string, unknown> = {}) {
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

export function document(over: Record<string, unknown> = {}) {
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

type BackendOpts = {
  sets?: ReturnType<typeof corpus>[];
  documents?: ReturnType<typeof document>[];
  refuse?: Partial<
    Record<"create" | "archive" | "upload" | "delete", () => Response>
  >;
  maxDocumentBytes?: number;
};

// The fake server's state, and one handler per route it answers.
function knowledgeServer(opts: BackendOpts) {
  const posts: string[] = [];
  const deletes: string[] = [];
  const patches: unknown[] = [];
  let sets = opts.sets ?? [corpus()];
  let archived: ReturnType<typeof corpus>[] = [];
  let documents = opts.documents ?? [document()];

  const deleteDocument = (req: Request) => {
    deletes.push(req.url);
    if (opts.refuse?.delete) return opts.refuse.delete();
    documents = documents.filter((doc) => !req.url.endsWith(doc.id));
    return new Response(null, { status: 204 });
  };
  const documentsOfSet = (req: Request) => {
    if (req.method !== "POST") return jsonResponse({ items: documents });
    posts.push(req.url);
    if (opts.refuse?.upload) return opts.refuse.upload();
    return jsonResponse(document({ ingest_status: "queued" }), 202);
  };
  const restore = (req: Request) => {
    posts.push(req.url);
    const back = archived.filter((set) => req.url.includes(set.id));
    archived = archived.filter((set) => !req.url.includes(set.id));
    sets = [...sets, ...back.map((set) => ({ ...set, archived_at: null }))];
    return jsonResponse(sets[sets.length - 1]);
  };
  const patch = async (req: Request) => {
    const body = await req.json();
    patches.push(body);
    sets = sets.map((set) =>
      req.url.endsWith(set.id) ? { ...set, ...body } : set,
    );
    return jsonResponse(sets[0]);
  };
  const create = (req: Request) => {
    posts.push(req.url);
    if (opts.refuse?.create) return opts.refuse.create();
    return jsonResponse(corpus(), 201);
  };
  const archive = (req: Request) => {
    deletes.push(req.url);
    if (opts.refuse?.archive) return opts.refuse.archive();
    archived = [
      ...archived,
      ...sets
        .filter((set) => req.url.endsWith(set.id))
        .map((set) => ({ ...set, archived_at: "2026-08-02T00:00:00Z" })),
    ];
    sets = sets.filter((set) => !req.url.endsWith(set.id));
    return new Response(null, { status: 204 });
  };
  const list = (req: Request) => {
    const withArchived = new URL(req.url).searchParams.get("include_archived");
    return jsonResponse({
      items: withArchived === "true" ? [...sets, ...archived] : sets,
    });
  };
  const corpora: Record<
    string,
    (req: Request) => Response | Promise<Response>
  > = { PATCH: patch, POST: create, DELETE: archive, GET: list };
  return {
    posts,
    deletes,
    patches,
    deleteDocument,
    documentsOfSet,
    restore,
    corpora,
  };
}

export function backendFor(allow: GrantSpec, opts: BackendOpts = {}) {
  const server = knowledgeServer(opts);
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.endsWith("/installation/settings")) {
        return jsonResponse({
          max_knowledge_document_bytes: opts.maxDocumentBytes,
        });
      }
      if (req.url.includes("/knowledge/documents/")) {
        return server.deleteDocument(req);
      }
      if (req.url.includes("/documents")) return server.documentsOfSet(req);
      if (req.url.endsWith("/restore")) return server.restore(req);
      const corpora = server.corpora[req.method];
      if (req.url.includes("/knowledge/corpora") && corpora) {
        return corpora(req);
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  const { posts, deletes, patches } = server;
  return { fetchMock, posts, deletes, patches };
}

export const render = (ui: ReactNode, locale: Locale = "en") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

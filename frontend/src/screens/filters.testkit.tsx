// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A fake server for the Filters and views suites: /me, the vocabulary, the
// preview, /views, /lists and /exports, answering as the contract says and
// remembering its writes, so a rename is read back renamed and a delete is
// read back gone. Every request is recorded with its method, address,
// If-Match and body, so a suite asserts what was asked rather than inferring
// it from what drew.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import type { FilterResource, FilterVocabulary } from "./filterdata";
import type { List } from "./lists.queries";
import type { SavedView } from "./savedviews.queries";

export type Sent = Readonly<{
  method: string;
  url: string;
  ifMatch: string | null;
  body: unknown;
}>;

export type FiltersServer = Readonly<{
  listsOn?: boolean;
  /** Held open until it resolves, for a case about the wait itself. */
  meAnswered?: Promise<void>;
  views?: readonly Readonly<Record<string, unknown>>[];
  viewsAnswered?: Promise<void>;
  /** What `GET /views/{id}` answers; otherwise the row from `views`. */
  view?: unknown;
  lists?: readonly List[];
  /** What `GET /lists/{id}` answers; otherwise the row from `lists`. */
  list?: List;
  /** What `POST /lists` answers. */
  created?: Partial<List>;
  listsFail?: boolean;
  viewsFail?: boolean;
  /** Both catalog reads say they stopped at the server's cap. */
  truncated?: boolean;
  /** The most rows one `GET /lists` answers before it says there are more. */
  listsCap?: number;
  /** Held open until it resolves: a `GET /lists` asking for archived lists too. */
  archivedAnswered?: Promise<void>;
  preview?: Readonly<{
    match_count: number;
    columns?: readonly string[];
    rows?: readonly Record<string, unknown>[];
  }>;
  vocabularies?: Partial<Record<FilterResource, FilterVocabulary>>;
  vocabularyAnswered?: Promise<void>;
  /** The reader's teams, each answered by `GET /teams` under its name. */
  teams?: Readonly<Record<string, string>>;
}>;

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

const refused = () =>
  new Response(JSON.stringify({ title: "Server error", status: 500 }), {
    status: 500,
    headers: { "Content-Type": "application/problem+json" },
  });

const page = (data: readonly unknown[], hasMore: boolean) =>
  json({ data, page: { next_cursor: null, has_more: hasMore } });

/** A rendered file, served under a name the client did not compose itself. */
const exported = () =>
  new Response("id,full_name\np1,Ann Lee\n", {
    status: 200,
    headers: {
      "Content-Type": "text/csv",
      "Content-Disposition": 'attachment; filename="contacts-slice.csv"',
    },
  });

async function sentOf(input: RequestInfo | URL, init?: RequestInit) {
  const request = input instanceof Request ? input : null;
  const method = request?.method ?? init?.method ?? "GET";
  let body: unknown;
  if (method !== "GET" && method !== "DELETE") {
    body = request
      ? await request.clone().json()
      : JSON.parse(String(init?.body));
  }
  return {
    method,
    url: String(request ? request.url : input),
    ifMatch: request?.headers.get("If-Match") ?? null,
    body,
  };
}

type Row = Readonly<Record<string, unknown>>;

function isRow(value: unknown): value is Row {
  return typeof value === "object" && value !== null;
}

const notFound = () => json({ title: "Not found", status: 404 }, 404);

const searchOf = (sent: Sent) =>
  new URL(sent.url, "https://x.local").searchParams;

/** The saved views, as a server holding them: writes change what reads see. */
function viewsServer(server: FiltersServer) {
  let views: Row[] = [...(server.views ?? [])];
  const write = (sent: Sent, id: string | undefined): Response => {
    const body = isRow(sent.body) ? sent.body : {};
    if (sent.method === "POST") {
      const made = { id: "v-new", owner_id: "u-1", version: 1, ...body };
      views = [...views, made];
      return json(made, 201);
    }
    if (sent.method === "PATCH") {
      const was = views.find((row) => row.id === id) ?? {};
      const now = { ...was, ...body, version: Number(was.version ?? 0) + 1 };
      views = views.map((row) => (row.id === id ? now : row));
      return json(now);
    }
    views = views.filter((row) => row.id !== id);
    return new Response(null, { status: 204 });
  };
  const read = async (sent: Sent, id: string | undefined) => {
    await server.viewsAnswered;
    if (server.viewsFail) {
      return refused();
    }
    if (id !== undefined) {
      const found = server.view ?? views.find((row) => row.id === id);
      return found ? json(found) : notFound();
    }
    const resource = searchOf(sent).get("resource");
    const rows = resource
      ? views.filter((row) => row.resource === resource)
      : views;
    return page(rows, server.truncated === true);
  };
  return (sent: Sent, path: string) => {
    const id = /^\/views\/([^/]+)$/.exec(path)?.[1];
    return sent.method === "GET" ? read(sent, id) : write(sent, id);
  };
}

/** The lists catalog, honouring `q`, `include_archived` and the cap. */
async function listsCatalog(server: FiltersServer, sent: Sent) {
  const query = searchOf(sent);
  const archived = query.get("include_archived") === "true";
  if (archived) {
    await server.archivedAnswered;
  }
  const q = (query.get("q") ?? "").toLowerCase();
  const rows = (server.lists ?? []).filter(
    (row) =>
      (archived || !row.archived_at) &&
      `${row.name} ${row.purpose ?? ""}`.toLowerCase().includes(q),
  );
  const cap = server.listsCap ?? rows.length;
  return page(
    rows.slice(0, cap),
    server.truncated === true || rows.length > cap,
  );
}

/** The lists: the catalog, one list by id, and a create. */
function listsServer(server: FiltersServer) {
  const lists = server.lists ?? [];
  return (sent: Sent, path: string) => {
    const id = /^\/lists\/([^/]+)$/.exec(path)?.[1];
    if (sent.method === "POST") {
      const body = isRow(sent.body) ? sent.body : {};
      return json({ id: "new-list", ...body, ...server.created }, 201);
    }
    if (server.listsFail) {
      return refused();
    }
    if (id !== undefined) {
      const found = server.list ?? lists.find((row) => row.id === id);
      return found ? json(found) : notFound();
    }
    return listsCatalog(server, sent);
  };
}

/** Everything else the pages read, one answer per address. */
function readsServer(
  server: FiltersServer,
): Readonly<Record<string, (sent: Sent) => Promise<Response> | Response>> {
  return {
    "/me": async () => {
      await server.meAnswered;
      return json({
        ...meFixture({
          settingsAvailability: { lists: server.listsOn === true },
        }),
        teams: Object.keys(server.teams ?? {}),
      });
    },
    "/teams": () =>
      page(
        Object.entries(server.teams ?? {}).map(([id, name]) => ({ id, name })),
        false,
      ),
    "/filters/vocabulary": async (sent) => {
      await server.vocabularyAnswered;
      const resource = searchOf(sent).get("resource");
      const held = Object.entries(server.vocabularies ?? {}).find(
        ([named]) => named === resource,
      );
      return json(held?.[1] ?? { resource, fields: [] });
    },
    "/filters/preview": () =>
      json({
        resource: "contact",
        match_count: server.preview?.match_count ?? 0,
        columns: server.preview?.columns ?? ["id"],
        rows: server.preview?.rows ?? [],
        truncated: false,
      }),
    "/exports": exported,
  };
}

export function mountFilters(server: FiltersServer = {}) {
  const seen: Sent[] = [];
  const written: Sent[] = [];
  const views = viewsServer(server);
  const lists = listsServer(server);
  const reads = readsServer(server);
  const answer = (sent: Sent) => {
    const path = new URL(sent.url, "https://x.local").pathname.replace(
      /^\/v1/,
      "",
    );
    if (path.startsWith("/views")) {
      return views(sent, path);
    }
    if (path.startsWith("/lists")) {
      return lists(sent, path);
    }
    return reads[path]?.(sent) ?? page([], false);
  };

  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const sent = await sentOf(input, init);
      seen.push(sent);
      // The preview is a read sent as a POST; it changes nothing to assert.
      if (sent.method !== "GET" && !sent.url.includes("/filters/preview")) {
        written.push(sent);
      }
      return answer(sent);
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  return { seen, written, wrapper, client };
}

/** A stored saved view whose `query` holds the given filter tree. */
export function filterView(
  id: string,
  name: string,
  resource: SavedView["resource"],
  filter: unknown,
): SavedView {
  return {
    id,
    owner_id: "u-1",
    resource,
    name,
    shared_scope: "private",
    query: { filter },
    version: 1,
  };
}

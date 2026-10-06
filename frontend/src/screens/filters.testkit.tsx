// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A fake server for the Filters and views suites: /me, the vocabulary, the
// preview, a proposal, /views, /lists and /exports, answering as the contract
// says and remembering its writes, so a rename is read back renamed, a delete
// is read back gone, and a write held to a stale version is refused. Every
// request is recorded with its method, address, If-Match and body, so a suite
// asserts what was asked rather than inferring it from what drew. Beside it,
// the shell's unsaved guard, for a suite that leaves the page.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { useHash } from "../app/router";
import { UnsavedGuard } from "../app/unsaved";
import { ToastProvider, ToastRegion } from "../design-system/toast";
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
  /** Held open until it resolves: every `PATCH /views/{id}`. */
  viewPatched?: Promise<void>;
  lists?: readonly List[];
  /** Held open until it resolves: one list read by id. */
  listAnswered?: Promise<void>;
  /** What `POST /lists` answers. */
  created?: Partial<List>;
  /** A problem `POST /views` and `POST /lists` answer instead of a create. */
  createRefused?: Readonly<{ status: number; detail: string }>;
  /** Held open until it resolves: every `POST /views` and `POST /lists`. */
  createAnswered?: Promise<void>;
  /** Held open until it resolves: every export. */
  exportAnswered?: Promise<void>;
  listsFail?: boolean;
  viewsFail?: boolean;
  /** Both catalog reads say they stopped at the server's cap. */
  truncated?: boolean;
  /** The most rows one `GET /lists` answers before it says there are more. */
  listsCap?: number;
  /** Held open until it resolves: a `GET /lists` asking for archived lists too. */
  archivedAnswered?: Promise<void>;
  /** Held open until it resolves: every preview count. */
  previewAnswered?: Promise<void>;
  preview?: Readonly<{
    match_count: number;
    columns?: readonly string[];
    rows?: readonly Record<string, unknown>[];
  }>;
  /**
   * What `POST /filters/propose` answers, one per ask in order; the last one
   * answers every ask after it. A status other than 200 is a problem, and an
   * answer is held open until its `answered` resolves.
   */
  proposals?: readonly Readonly<{
    status?: number;
    body: unknown;
    answered?: Promise<void>;
  }>[];
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
    // A write may carry no body at all, as a list visit does.
    const text = request
      ? await request.clone().text()
      : String(init?.body ?? "");
    body = text === "" ? undefined : JSON.parse(text);
  }
  return {
    method,
    url: String(request ? request.url : input),
    ifMatch: request?.headers.get("If-Match") ?? null,
    body,
  };
}

type Row = Readonly<Record<string, unknown>>;

/** The page size a preview request asked for. */
function limitOf(sent: Sent): number {
  const limit = isRow(sent.body) ? sent.body.limit : undefined;
  return typeof limit === "number" ? limit : Number.POSITIVE_INFINITY;
}

function isRow(value: unknown): value is Row {
  return typeof value === "object" && value !== null;
}

const problem = (
  refusal: Readonly<{ status: number; detail: string; code?: string }>,
) =>
  new Response(JSON.stringify({ title: "Refused", ...refusal }), {
    status: refusal.status,
    headers: { "Content-Type": "application/problem+json" },
  });

const notFound = () =>
  problem({ status: 404, code: "not_found", detail: "Not found." });

const searchOf = (sent: Sent) =>
  new URL(sent.url, "https://x.local").searchParams;

/** A stored view changed as a colleague would: a later read sees it. */
type ChangeView = (id: string, changes: Row) => void;

/**
 * The saved views, as a server holding them: writes change what reads see, a
 * PATCH whose If-Match names a version the row has moved past is refused, and
 * an archived row is read but no longer written, as the server does both.
 */
function viewsServer(server: FiltersServer) {
  let views: Row[] = [...(server.views ?? [])];
  const patch = async (sent: Sent, id: string | undefined, body: Row) => {
    await server.viewPatched;
    const was = views.find((row) => row.id === id);
    if (was === undefined || was.archived_at) {
      return notFound();
    }
    if (sent.ifMatch !== null && sent.ifMatch !== String(was.version)) {
      return problem({
        status: 409,
        code: "version_skew",
        detail: "The view changed since it was read.",
      });
    }
    const now = { ...was, ...body, version: Number(was.version ?? 0) + 1 };
    views = views.map((row) => (row.id === id ? now : row));
    return json(now);
  };
  const write = (
    sent: Sent,
    id: string | undefined,
  ): Response | Promise<Response> => {
    const body = isRow(sent.body) ? sent.body : {};
    if (sent.method === "POST" && server.createRefused) {
      return problem(server.createRefused);
    }
    if (sent.method === "POST") {
      const made = { id: "v-new", owner_id: "u-1", version: 1, ...body };
      views = [...views, made];
      return json(made, 201);
    }
    if (sent.method === "PATCH") {
      return patch(sent, id, body);
    }
    views = views.filter((row) => row.id !== id);
    return new Response(null, { status: 204 });
  };
  const change: ChangeView = (id, changes) => {
    views = views.map((row) => (row.id === id ? { ...row, ...changes } : row));
  };
  const read = async (sent: Sent, id: string | undefined) => {
    await server.viewsAnswered;
    if (server.viewsFail) {
      return refused();
    }
    if (id !== undefined) {
      const found = views.find((row) => row.id === id);
      return found ? json(found) : notFound();
    }
    const resource = searchOf(sent).get("resource");
    const rows = resource
      ? views.filter((row) => row.resource === resource)
      : views;
    return page(rows, server.truncated === true);
  };
  const answer = async (sent: Sent, path: string) => {
    const id = /^\/views\/([^/]+)$/.exec(path)?.[1];
    if (sent.method === "POST") {
      await server.createAnswered;
    }
    return sent.method === "GET" ? read(sent, id) : write(sent, id);
  };
  return { answer, change };
}

/** The lists catalog, honouring `q`, `include_archived` and the cap. */
async function listsCatalog(
  server: FiltersServer,
  lists: readonly List[],
  sent: Sent,
) {
  const query = searchOf(sent);
  const archived = query.get("include_archived") === "true";
  if (archived) {
    await server.archivedAnswered;
  }
  const q = (query.get("q") ?? "").toLowerCase();
  const rows = lists.filter(
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

/** A stored list changed as a colleague would: a later read sees it. */
type ChangeList = (id: string, changes: Partial<List>) => void;

/**
 * The lists: the catalog, one list by id, a create, and a filter written back,
 * refused as the server refuses it when the version it carries is not the
 * stored one.
 */
function listsServer(server: FiltersServer) {
  let lists: List[] = [...(server.lists ?? [])];
  const change: ChangeList = (id, changes) => {
    lists = lists.map((row) => (row.id === id ? { ...row, ...changes } : row));
  };
  const update = (id: string, sent: Sent) => {
    const body = isRow(sent.body) ? sent.body : {};
    const was = lists.find((row) => row.id === id);
    if (was === undefined) {
      return notFound();
    }
    if (body.version !== was.version) {
      return problem({
        status: 409,
        code: "version_skew",
        detail: "The list changed since it was read.",
      });
    }
    const now: List = {
      ...was,
      version: was.version + 1,
      definition: isRow(body.definition) ? body.definition : was.definition,
    };
    lists = lists.map((row) => (row.id === id ? now : row));
    return json(now);
  };
  const answer = async (sent: Sent, path: string) => {
    const id = /^\/lists\/([^/]+)$/.exec(path)?.[1];
    if (sent.method === "POST") {
      await server.createAnswered;
      const body = isRow(sent.body) ? sent.body : {};
      return server.createRefused
        ? problem(server.createRefused)
        : json({ id: "new-list", ...body, ...server.created }, 201);
    }
    if (server.listsFail) {
      return refused();
    }
    if (id !== undefined && sent.method === "PATCH") {
      return update(id, sent);
    }
    if (id !== undefined) {
      return oneList(id);
    }
    return listsCatalog(server, lists, sent);
  };
  async function oneList(id: string) {
    await server.listAnswered;
    const found = lists.find((row) => row.id === id);
    return found ? json(found) : notFound();
  }
  return { answer, change };
}

/** The model's answers, handed out in the order the reader asked. */
function proposeServer(server: FiltersServer) {
  let asked = 0;
  return async () => {
    const answers = server.proposals ?? [];
    const answer = answers[Math.min(asked, answers.length - 1)];
    asked += 1;
    if (answer === undefined) {
      return notFound();
    }
    await answer.answered;
    const status = answer.status ?? 200;
    return status === 200
      ? json(answer.body)
      : problem({
          status,
          detail: "refused",
          ...(isRow(answer.body) ? answer.body : {}),
        });
  };
}

/** Everything else the pages read, one answer per address. */
function readsServer(
  server: FiltersServer,
): Readonly<Record<string, (sent: Sent) => Promise<Response> | Response>> {
  return {
    "/filters/propose": proposeServer(server),
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
    // At most the page asked for, as the server sends: a case about asking
    // for more rows hands over more than one page and reads what arrives.
    "/filters/preview": async (sent) => {
      await server.previewAnswered;
      return json({
        resource: "contact",
        match_count: server.preview?.match_count ?? 0,
        columns: server.preview?.columns ?? ["id"],
        rows: (server.preview?.rows ?? []).slice(0, limitOf(sent)),
        truncated: false,
      });
    },
    "/exports": async () => {
      await server.exportAnswered;
      return exported();
    },
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
      return views.answer(sent, path);
    }
    if (path.startsWith("/lists")) {
      return lists.answer(sent, path);
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
  return {
    seen,
    written,
    wrapper,
    client,
    changeView: views.change,
    changeList: lists.change,
  };
}

/**
 * The shell's unsaved guard around a page, with the toasts a save shows.
 * `page` draws the addresses a suite opens; every other address reads as where
 * the reader went, so a way off the page that should not ask is seen not to.
 */
export function GuardedFilters({
  page,
}: Readonly<{ page: (address: string) => ReactNode }>) {
  const hash = useHash();
  return (
    <ToastProvider>
      <UnsavedGuard
        address={hash}
        onKeep={(kept) => {
          window.location.hash = kept;
        }}
      >
        {(shown) => page(shown) ?? <p>{`Arrived at ${shown}`}</p>}
      </UnsavedGuard>
      <ToastRegion />
    </ToastProvider>
  );
}

/** The first row's field, where a press that redrew the rows leaves focus. */
export function firstRowField(): Element | null {
  return document.querySelector('.filter-clause [role="combobox"]');
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

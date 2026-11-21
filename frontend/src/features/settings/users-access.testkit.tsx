/** Shared fixtures for the access preview and teams roster tests. */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { vi } from "vitest";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { LocaleProvider } from "../../i18n";

export type Preview = {
  row_scope: "own" | "team" | "all";
  teams?: { id: string; name: string }[];
  objects?: Record<
    string,
    { read?: boolean; create?: boolean; update?: boolean; delete?: boolean }
  >;
  field_masks?: { object: string; field: string; condition: string }[];
};

export type Team = {
  id: string;
  name: string;
  member_count?: number;
  parent_team_id?: string;
};
// The roster row the membership editor reads. `team_ids` is the admin-only
// field the server populates, and a fixture that omits it models a NON-admin
// read — which is a different case, not a lighter one.
export type RosterUser = {
  id: string;
  email: string;
  display_name: string;
  status: string;
  is_agent: boolean;
  team_ids?: string[];
};

// Held in a constant rather than written inline: the prop is the seat's ROLE,
// and a literal here reads to the a11y lint as an ARIA role on an element.

export type Call = {
  method: string;
  path: string;
  body: unknown;
  // The query, because a GET carries what a POST used to put in its body.
  query: Record<string, string>;
};

export function backend(
  opts: Readonly<{
    preview?: Preview;
    teams?: Team[];
    /** The user roster, which is where a team's membership is read from. */
    users?: RosterUser[];
    /** Answers one write with a problem document, to drive the refusal arms. */
    refuse?: (call: Call) => boolean;
    /**
     * The caller's own roles, off `/me` — admin by default, so the write
     * assertions in this suite exercise the same seat they always have. A
     * test naming `["ops"]` or `[]` here gets the read-only case, which is a
     * different render entirely rather than the same one with fewer writes
     * attempted.
     */
    me?: readonly string[];
    /**
     * The grants this principal holds, when a case needs something other than
     * the full pair. The card asks two DIFFERENT objects — `team_admin` for the
     * verbs and `user_admin:read` for the membership the roster carries — so a
     * case can now describe a reader who sees who is in a team and may not
     * change it, which one admin check could not express.
     */
    allow?: Record<string, Record<string, boolean>>;
    /** Held open until it settles, so a case can look at a write in flight. */
    holdWrites?: Promise<void>;
  }>,
) {
  const calls: Call[] = [];
  // Copied, so a membership write lands on this case's roster and the refetch
  // after it reads the team the way the server would.
  const users = (opts.users ?? []).map((user) => ({
    ...user,
    team_ids: user.team_ids && [...user.team_ids],
  }));
  const fetchMock = vi.fn(
    // biome-ignore lint/complexity/noExcessiveCognitiveComplexity: a stub of every read and write the access screens make; its branches are the route table
    async (input: RequestInfo | URL, init?: RequestInit) => {
      // Built as a Request rather than read off `init`: openapi-fetch may pass a
      // Request with no init at all, and a mock reading the method from init
      // would answer every write as if it were a read.
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(request.url, "http://localhost");
      const path = url.pathname;
      calls.push({
        method: request.method,
        path,
        query: Object.fromEntries(url.searchParams),
        // A membership write carries NO body — the ids are the path. Reading
        // one unconditionally throws, and the mock would then answer the
        // write as a network failure that looks exactly like a refusal.
        body:
          request.method === "GET" ||
          request.headers.get("content-type") === null
            ? undefined
            : await request.clone().json(),
      });
      if (path.endsWith("/me")) {
        // `user` is required on MeResponse — useMe() treats a payload
        // without it as an availability failure and never resolves `.data`,
        // which would silently read every seat here as non-admin.
        //
        // The GRANTS matter for the same reason, one level up: the card asks
        // `team_admin` for its verbs and `user_admin:read` for the membership
        // the roster carries, so a payload naming a role and no authorization
        // describes a principal the API cannot produce — and every case here
        // would pass with nothing rendered. `allow` lets a case withhold one
        // of the two, which is the reader the split made possible.
        return new Response(
          JSON.stringify({
            user: { email: "you@acme.test" },
            roles: opts.me ?? ["admin"],
            authorization: {
              objects: opts.allow ?? {
                team_admin: {
                  read: true,
                  create: true,
                  update: true,
                  delete: false,
                },
                user_admin: {
                  read: true,
                  create: true,
                  update: true,
                  delete: true,
                },
              },
              seat_type: "full",
              row_scope: "all",
            },
          }),
          { headers: { "Content-Type": "application/json" } },
        );
      }
      if (request.method !== "GET") {
        await opts.holdWrites;
      }
      if (opts.refuse?.(calls[calls.length - 1])) {
        return new Response(JSON.stringify({ detail: "the team was merged" }), {
          status: 409,
          headers: { "Content-Type": "application/problem+json" },
        });
      }
      applyMembership(users, request.method, path);
      // The teams list answers as the contract answers: a page, with the
      // cursor of the next one. Without `page` the roster walk has nothing to
      // read the end of the list from.
      // Three reads answer here, and the roster two of them serve are
      // DIFFERENT lists: a test that answered users with the team page would
      // let a membership assertion pass against rows that carry no membership.
      const body = path.endsWith("/users/access-preview")
        ? (opts.preview ?? { row_scope: "own" })
        : {
            data: path.endsWith("/users")
              ? // The server serves the active-only roster unless the wide
                // management view is asked for.
                users.filter(
                  (user) =>
                    url.searchParams.get("include_inactive") === "true" ||
                    user.status === "active",
                )
              : (opts.teams ?? []),
            page: { next_cursor: null, has_more: false },
          };
      return new Response(JSON.stringify(body), {
        headers: { "Content-Type": "application/json" },
      });
    },
  );
  return { fetchMock, calls };
}

export function applyMembership(
  users: RosterUser[],
  method: string,
  path: string,
) {
  const [, teamId, userId] =
    /\/teams\/([^/]+)\/members\/([^/]+)$/.exec(path) ?? [];
  const user = users.find((each) => each.id === userId);
  if (!(teamId && user && (method === "PUT" || method === "DELETE"))) {
    return;
  }
  const others = (user.team_ids ?? []).filter((id) => id !== teamId);
  user.team_ids = method === "PUT" ? [...others, teamId] : others;
}

export function Providers({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        {/* The region is the shell's in the running app (`main.tsx`), so a
            suite whose subject includes what an archive SAYS mounts it the
            same way — the Undo this card offers lives inside it. */}
        <ToastProvider>
          {children}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>
  );
}

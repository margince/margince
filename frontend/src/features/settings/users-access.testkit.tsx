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
// The roster row the membership editor reads. Only an admin read carries
// `team_ids`, so a fixture without it models a reader who is not an admin.
export type RosterUser = {
  id: string;
  email: string;
  display_name: string;
  status: string;
  is_agent: boolean;
  team_ids?: string[];
};

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
     * The caller's own roles, off `/me`. Admin by default, so the write cases
     * run as an admin. `["ops"]` or `[]` gives the read-only screen.
     */
    me?: readonly string[];
    /**
     * The grants this principal holds, when a case needs less than both. The
     * card asks `team_admin` for its verbs and `user_admin:read` for the
     * membership. So a case can describe a reader who sees a team's members and
     * may not change them.
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
      // openapi-fetch may pass a Request with no `init`. Read off `init`, every
      // write would answer as a read.
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const url = new URL(request.url, "http://localhost");
      const path = url.pathname;
      calls.push({
        method: request.method,
        path,
        query: Object.fromEntries(url.searchParams),
        // A membership write has no body, because the ids are in the path.
        // Reading one throws, and the write would look like a refusal.
        body:
          request.method === "GET" ||
          request.headers.get("content-type") === null
            ? undefined
            : await request.clone().json(),
      });
      if (path.endsWith("/me")) {
        // useMe() reads a payload without `user` as a failure, so every seat
        // would read as not an admin. Without the grants, every case would pass
        // with nothing drawn; `allow` withholds one of the two.
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
      const last = calls.at(-1);
      if (last && opts.refuse?.(last)) {
        return new Response(JSON.stringify({ detail: "the team was merged" }), {
          status: 409,
          headers: { "Content-Type": "application/problem+json" },
        });
      }
      applyMembership(users, request.method, path);
      // The teams list answers as a page with the next cursor, as the contract
      // does. The roster is a separate list, so a membership check reads rows
      // that carry a membership.
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
        {/* The running app mounts the region in the shell (`main.tsx`). It is
            mounted here too, because the card's Undo lives inside it. */}
        <ToastProvider>
          {children}
          <ToastRegion />
        </ToastProvider>
      </LocaleProvider>
    </QueryClientProvider>
  );
}

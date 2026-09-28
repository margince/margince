import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  type RenderResult,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import type userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { SEEDED_ASSIGNABLE_ROLES } from "./roles.testkit";

// The roster fixture, the routed backend and the row helpers the member card's
// suites share: users-admin.test.tsx for the roster and its verbs,
// users-admin.roles.test.tsx for the role picker.

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// `roles` rides the roster only for an admin caller, which this card always is.
// Nora holds none — an unassigned seat is reachable and has no current role to
// show. `allowed_actions` is what the server offers an admin on each member;
// Ada is not the caller's only fellow admin, so she can be demoted and switched
// off, and the agent seat takes no role and signs in with no password.
//
// Typed as the contract's members, so `allowed_actions` stays the enum.
type Member = components["schemas"]["User"];
export const ROSTER: {
  data: Omit<Member, "timezone">[];
  page: { next_cursor: null; has_more: boolean };
} = {
  data: [
    {
      id: "u-active",
      email: "ada@acme.test",
      display_name: "Ada Active",
      status: "active",
      is_agent: false,
      roles: ["admin"],
      allowed_actions: ["change_role", "issue_password_link", "deactivate"],
    },
    {
      id: "u-off",
      email: "otto@acme.test",
      display_name: "Otto Off",
      status: "deactivated",
      is_agent: false,
      roles: ["read_only"],
      allowed_actions: ["change_role", "reactivate"],
    },
    // An invitation nobody has redeemed: it holds a licensed seat and appears
    // in the roster, but signs in nowhere until the link is used.
    {
      id: "u-invited",
      email: "ivy@acme.test",
      display_name: "Ivy Invited",
      status: "invited",
      is_agent: false,
      roles: ["rep"],
      allowed_actions: ["change_role", "issue_password_link", "deactivate"],
    },
    {
      id: "u-none",
      email: "nora@acme.test",
      display_name: "Nora None",
      status: "active",
      is_agent: false,
      roles: [],
      allowed_actions: ["change_role", "issue_password_link", "deactivate"],
    },
    // An agent identity. Bootstrap no longer seeds one, so a fresh installation
    // shows a contacts-only roster — but the roster still LISTS such a row where
    // one exists (an installation that has not run the retirement migration, or
    // a resident runner later), and this screen's agent-specific branches are
    // what that row renders. The fixture keeps it so those branches stay tested.
    {
      id: "u-agent",
      email: "agent@acme.gradion.local",
      display_name: "Margince Agent",
      status: "active",
      is_agent: true,
      roles: [],
      allowed_actions: ["deactivate"],
    },
  ],
  page: { next_cursor: null, has_more: false },
};

// Both helpers narrow by instance rather than asserting: a cast would let the
// suite read `.disabled` off whatever the query happened to return, so a control
// that stopped being the Select trigger would surface as a confusing undefined
// instead of a named failure.
export function roleSelect(row: HTMLElement, name: string) {
  const control = within(row).getByRole("combobox", {
    name: new RegExp(`set role for ${name}`, "i"),
  });
  if (!(control instanceof HTMLButtonElement)) {
    throw new Error(`the role control for ${name} is not a select trigger`);
  }
  return control;
}

// What the closed control reads. The trigger's only text is its face — the
// chevron is aria-hidden and carries none — so this is the role an operator
// sees on the row without opening anything.
export function roleShown(row: HTMLElement, name: string): string {
  return roleSelect(row, name).textContent ?? "";
}

// A member is one SettingRow inside a wrapper carrying the row's refusal and
// the focus target its deactivate confirm hands back to. The wrapper's testid is
// what identifies it: SettingRow is a <div>, so there is no <li> to climb to.
export function rowFor(name: string) {
  const row = screen.getByText(name).closest('[data-testid^="member-"]');
  if (!(row instanceof HTMLElement)) {
    throw new Error(`no member row rendered for ${name}`);
  }
  return row;
}

// The row's verbs — the set-password link and the status change — live behind
// its OverflowMenu, and the menu's panel is portalled to the body: a query
// scoped to the row cannot see them, and the children are not even rendered
// until the menu is first opened. So open it and scope to the panel the trigger
// names.
export async function rowMenu(
  user: ReturnType<typeof userEvent.setup>,
  name: string,
) {
  const trigger = within(rowFor(name)).getByRole("button", {
    name: new RegExp(`actions for ${name}`, "i"),
  });
  // The trigger TOGGLES, so a helper that always clicks would shut a menu a
  // previous step left open — and the assertion after it would then be about a
  // panel with `hidden` on it rather than about what the row offers.
  if (trigger.getAttribute("aria-expanded") !== "true") {
    await user.click(trigger);
  }
  const panelId = trigger.getAttribute("aria-controls");
  const panel = panelId === null ? null : document.getElementById(panelId);
  if (!(panel instanceof HTMLElement)) {
    throw new Error(`no actions menu rendered for ${name}`);
  }
  return within(panel);
}

// `roles` defaults to the admin this suite is mostly about; a caller naming
// another role gets the same routed backend, so a non-admin case differs from an
// admin one by the principal alone and not by a second hand-rolled stub.
//
// GRANTS, not the role name. The card asks `user_admin` verb by verb now, and a
// /me carrying a role and no `authorization` describes a principal the API
// cannot produce — every gate would read false and every case would pass by
// construction. The admin default holds all four verbs because the seeded admin
// role does; a case wanting less passes `allow` explicitly.
const ADMIN_USER_ADMIN: Record<string, string[]> = {
  user_admin: ["read", "create", "update", "delete"],
};
export function backend(
  calls: { method: string; url: string; body?: unknown }[],
  me: { roles: string[]; allow?: Record<string, string[]> } = {
    roles: ["admin"],
    allow: ADMIN_USER_ADMIN,
  },
) {
  const allow = me.allow ?? {};
  const objects: Record<string, Record<string, boolean>> = {};
  for (const [object, verbs] of Object.entries(allow)) {
    objects[object] = {
      read: verbs.includes("read"),
      create: verbs.includes("create"),
      update: verbs.includes("update"),
      delete: verbs.includes("delete"),
    };
  }
  // openapi-fetch calls fetch(request) with a Request object, so read the
  // method + body off it rather than a separate init.
  return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const req =
      input instanceof Request ? input : new Request(String(input), init);
    if (req.url.endsWith("/v1/me")) {
      return jsonResponse({
        user: { email: "admin@acme.test" },
        roles: me.roles,
        teams: [],
        // The seat rides here too: `useCanWrite` folds the ceiling that
        // identity/admission.go enforces ABOVE RBAC, so a snapshot without a
        // seat refuses every write however wide its grants.
        // `row_scope` is REQUIRED on Authorization (crm.yaml), so a stub without
        // it describes a /me the server cannot send.
        authorization: { objects, seat_type: "full", row_scope: "all" },
        // The installation CAN mint set-password links. Without this the card
        // withholds that action from every row, and any assertion that one
        // particular row lacks it passes without the row having anything to do
        // with it.
        admin_password_link: true,
      });
    }
    const read = readRoute(req, allow);
    if (read !== undefined) {
      return read;
    }
    let body: unknown;
    try {
      body = await req.clone().json();
    } catch {
      body = undefined;
    }
    calls.push({ method: req.method, url: req.url, body });
    // The link mint answers its own shape. With admin_password_link on, the
    // invite flow opens the link dialog itself, and a generic user row handed
    // to it renders an expiry from `undefined` — an unhandled error rather
    // than a failed assertion, which fails the run without naming a test.
    if (req.url.includes("/password-link")) {
      return jsonResponse(
        {
          set_password_url: "https://crm.example.test/set-password?t=fixture",
          expires_at: "2026-08-18T09:00:00Z",
        },
        201,
      );
    }
    return jsonResponse({ ...ROSTER.data[0], id: "u-new" }, 201);
  });
}

// The reads the card makes besides /me, answered the way the server would for
// a caller holding `allow`; undefined for a write, which the caller records.
function readRoute(
  req: Request,
  allow: Record<string, string[]>,
): Response | undefined {
  if (req.url.includes("/teams") && req.method === "GET") {
    return jsonResponse({ data: [], page: { has_more: false } });
  }
  // The access preview is the server's own sentence about the role; this
  // suite is about the invite and the roster, so it answers a neutral rep.
  if (req.url.includes("/users/access-preview")) {
    return jsonResponse({
      role: "rep",
      row_scope: "team",
      objects: {},
      field_masks: [],
      teams: [],
    });
  }
  if (req.url.includes("/users/assignable-roles")) {
    return jsonResponse({ roles: SEEDED_ASSIGNABLE_ROLES });
  }
  if (req.url.includes("/users") && req.method === "GET") {
    // The SAME projection the server makes: `roles` and the deactivated
    // members ride the privileged view, which handlers_roster.go sends only
    // to a caller passing `user_admin:read`. A fixture answering the full
    // roster to everyone describes a response the API cannot produce — and
    // a case asserting a rep sees "Read-only" would then be leaning on data
    // the rep would never have received.
    //
    // The allowed actions follow the caller's verbs: a role change and a link
    // take `update`, switching a seat off or on takes `delete`.
    const verbs = allow.user_admin ?? [];
    if (verbs.includes("read")) {
      return jsonResponse({
        ...ROSTER,
        data: ROSTER.data.map((u) => ({
          ...u,
          allowed_actions: (u.allowed_actions ?? []).filter((action) =>
            verbs.includes(
              action === "deactivate" || action === "reactivate"
                ? "delete"
                : "update",
            ),
          ),
        })),
      });
    }
    return jsonResponse({
      ...ROSTER,
      data: ROSTER.data
        .filter((u) => u.status !== "deactivated")
        .map(
          ({ roles: _withheld, allowed_actions: _computed, ...rest }) => rest,
        ),
    });
  }
  return undefined;
}

export const render = (ui: ReactNode): RenderResult => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

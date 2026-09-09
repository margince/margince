import { render } from "@testing-library/react";
import type { components } from "../api/schema";
import { PersonPageV2 } from "./personpage";
import type { PERSON_TABS } from "./persontab";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The fixture and the mount every `personpage*.test.tsx` suite shares, in one
// place: a second suite reading its own copy of this fixture is the shape
// that drifts the two apart.

type Person360 = components["schemas"]["Person360"];
export type PersonConsentGuardEntry =
  components["schemas"]["PersonConsentGuardEntry"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

export const view: Person360 = {
  as_of: "2026-08-13T09:00:00Z",
  // With an address on file, so the header's lead verb is the Email one. The
  // verb names the transport the composer would pick, so a contact with no way
  // to be reached carries a different button — which is its own suite below.
  person: {
    id: "p-1",
    full_name: "Dana Buyer",
    // The server's own per-row answer, which every edit affordance on this
    // page asks for. Absent it the fixture describes a contact this reader
    // may not write, and the controls correctly disappear.
    writable: true,
    emails: [
      {
        id: "pe-1",
        person_id: "p-1",
        email: "dana@brandt.example",
        email_type: "work",
        is_primary: true,
        position: 0,
        ...CAPTURED,
      },
    ],
    ...CAPTURED,
  },
  sections_omitted: [],
  activities: {
    data: [
      {
        id: "a-1",
        kind: "email",
        subject: "Fleet renewal",
        occurred_at: "2026-08-11T12:00:00Z",
        is_done: false,
        ...CAPTURED,
      },
    ],
    page: { has_more: false },
  },
  deal_roles: { data: [], page: { has_more: false } },
  profile_fields: [],
};

export function mount(
  tab: (typeof PERSON_TABS)[number],
  page: Person360 = view,
  guardEntries: readonly PersonConsentGuardEntry[] = [],
  // Routes a test adds for the write it makes; the reads every mount needs
  // stay here.
  extraRoutes: RouteMap = {},
  // What the caller may do. Logging needs `activity.create`, which the store
  // behind the form requires, so a spec that logs must hold it here or it
  // passes under an authorization production refuses.
  allow: Parameters<typeof meRoute>[0] = {
    person: ["read", "update"],
    activity: ["create"],
  },
) {
  installFetchStub({
    "GET /me": meRoute(allow, { seat: "full" }),
    "GET /people/p-1/360": () => jsonResponse(page),
    "GET /people/p-1/brief": () =>
      jsonResponse({ person_id: "p-1", sentences: [], generated_by: "rules" }),
    "GET /people/p-1/consent/guard": () =>
      jsonResponse({ person_id: "p-1", entries: guardEntries }),
    // Which messaging providers exist is a deployment fact, so a channel has
    // no name until the directory supplies one.
    "GET /channel-providers": () =>
      jsonResponse({
        data: [
          {
            provider: "zalo_oa",
            label: "Zalo OA",
            credential_model: "workspace_bot",
            supplies_transport: true,
          },
        ],
      }),
    // The reader's own connected mailbox, which is what makes an address on
    // the page the composer's rather than their mail client's.
    "GET /connectors": () =>
      jsonResponse({
        data: [
          { id: "g1", provider: "gmail", status: "connected", scopes: [] },
        ],
      }),
    // Last, so a spec that needs to override a read above (e.g. a /me that
    // never resolves, for a pending-grant spec) can.
    ...extraRoutes,
  });
  render(
    <StoryProviders>
      <PersonPageV2 id="p-1" tab={tab} />
    </StoryProviders>,
  );
}

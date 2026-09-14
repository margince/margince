import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { ContactPageV2 } from "./contactpage";
import type { CONTACT_TABS } from "./contacttab";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "./story-utils";

// The fixture and the mount every `contactpage*.test.tsx` suite shares, in one
// place: a second suite reading its own copy of this fixture is the shape
// that drifts the two apart.

type Contact360 = components["schemas"]["Contact360"];
export type ContactConsentGuardEntry =
  components["schemas"]["ContactConsentGuardEntry"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

export const view: Contact360 = {
  as_of: "2026-08-13T09:00:00Z",
  // With an address on file, so the header's lead verb is the Email one. The
  // verb names the transport the composer would pick, so a contact with no way
  // to be reached carries a different button — which is its own suite below.
  contact: {
    id: "p-1",
    full_name: "Dana Buyer",
    // The server's own per-row answer, which every edit affordance on this
    // page asks for. Absent it the fixture describes a contact this reader
    // may not write, and the controls correctly disappear.
    writable: true,
    emails: [
      {
        id: "pe-1",
        contact_id: "p-1",
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

// What the caller may do unless a spec says otherwise. Logging needs
// `activity.create`, which the store behind the form requires, so a spec that
// logs must hold it here or it passes under an authorization production
// refuses.
//
// One fixed grant set, named at module scope beside the view above rather than
// rebuilt inline on every mount.
const callerGrants: Parameters<typeof meRoute>[0] = {
  contact: ["read", "update"],
  activity: ["create"],
};

export function mount(
  tab: (typeof CONTACT_TABS)[number],
  page: Contact360 = view,
  guardEntries: readonly ContactConsentGuardEntry[] = [],
  // Routes a test adds for the write it makes; the reads every mount needs
  // stay here.
  extraRoutes: RouteMap = {},
  allow: Parameters<typeof meRoute>[0] = callerGrants,
) {
  installFetchStub({
    "GET /me": meRoute(allow, { seat: "full" }),
    "GET /contacts/p-1/360": () => jsonResponse(page),
    "GET /contacts/p-1/brief": () =>
      jsonResponse({ contact_id: "p-1", sentences: [], generated_by: "rules" }),
    "GET /contacts/p-1/consent/guard": () =>
      jsonResponse({ contact_id: "p-1", entries: guardEntries }),
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
      <ContactPageV2 id="p-1" tab={tab} />
    </StoryProviders>,
  );
}

// Edit, merge, share, full history, research and archive are ROWS of the
// header's overflow menu, and the menu does not mount its rows until it has
// been opened once — so a spec reaching for one opens it first.
//
// Both contact surfaces draw that menu (ContactPageV2 through contactactions.tsx,
// ContactScreen through contacts.tsx), so both suites reach for these: a copy
// per suite would be two answers to where those verbs live, and the suite
// holding the stale one would go on passing against a header nobody ships.
export async function openRecordMenu(): Promise<void> {
  await userEvent.click(
    await screen.findByRole("button", { name: en["record.moreActions"] }),
  );
}

// Open the menu and press one of its rows — the two steps every spec that
// exercises one of those verbs starts with.
export async function pressRecordVerb(testId: string): Promise<void> {
  await openRecordMenu();
  await userEvent.click(await screen.findByTestId(testId));
}

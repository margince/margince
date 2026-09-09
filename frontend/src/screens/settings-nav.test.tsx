/** @vitest-environment jsdom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RbacObject } from "../app/capability";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { translate } from "../i18n";
import {
  expectNavSettlesTo,
  floorPlus,
  labelOf,
  offeredPages,
  pagesNamed,
  readOn,
  renderHome,
  settingsNavBackend,
  UNGATED_IDS,
} from "./settings.testkit";
import {
  SETTINGS_GROUPS,
  SETTINGS_PAGES,
  type SettingsGroupId,
  type SettingsPageId,
  visibleSettingsPages,
} from "./settingscatalog";
import { SETTINGS_HOME_ID } from "./settingsnav";

// WHICH pages the settings level offers a principal, as a whole list.
//
// The level is composed from the SETTINGS_PAGES catalog and the grant map /me
// carries, so every expectation here is DERIVED from that catalog rather than
// restated beside it: a list of labels written out by hand is a second source
// of truth, and nothing updates it.
//
// What ONE page's own requirement opens is `settings-grants.test.tsx`; what the
// chrome around a page renders is `settings-chrome.test.tsx`; what happens when
// a reader addresses a page the rail did not offer is `settings-reach.test.tsx`.
// What a page then RENDERS is `settings.test.tsx` and its siblings’ subject;
// the shared fixtures are in `settings.testkit.tsx`.

// No shared fetch stub: the backend a claim needs is installed beside the claim,
// so what answered it is readable where it is asserted.
beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

// The rows under ONE group heading. Each group renders its heading and its own
// links inside a single container, so the heading's parent is what says which
// rows belong to which group — the flat list above cannot tell a mis-grouped
// page from a correctly grouped one.
function navGroupPages(heading: HTMLElement): string[] {
  const container = heading.parentElement;
  if (!container) {
    throw new Error(`the group heading "${heading.textContent}" stands alone`);
  }
  return within(container)
    .getAllByRole("link")
    .map((link) => link.textContent ?? "");
}

const groupLabelOf = (group: SettingsGroupId) =>
  translate("en", `settings.group.${group}`);

// Without the home row: a claim about ONE subject group's rows is not a claim
// about the row above all of them, which sits in the group carrying the level's
// own name.
const pagesIn = (group: SettingsGroupId) =>
  SETTINGS_PAGES.filter((page) => page.group === group).map((page) =>
    labelOf(page.id),
  );

// Every page open at once: one grant apiece for the pages that follow an
// object, plus the reset page's `delete` — emptying an installation is not a
// thing you read.
//
// Every term is the object its page's own cards ask for. `system_reset:delete`
// opens the danger zone and nothing else now: it used to stand in for "is
// admin" on Extensions and Audit log, whose cards asked `useHoldsAdminRole`,
// and those cards ask `extension_access:read` and `audit_log:read` instead.
// `ai_diagnostics:read` is what `AiUsageCard` and `AiCallsCard` ask, where they
// used to ask `automation:update`.
// Four of these are WRITES, and that is the change rather than a slip: a page
// whose subject is the installation's own configuration asks for the verb that
// changes it, because the read is one every seat holds. Granting the read here
// would leave this "every page" fixture four pages short.
const EVERY_PAGE_GRANTED: GrantSpec = {
  // Writing voice, whose card asks the WRITE. It needed no entry while the page
  // was ungated; now that the rail carries what a reader can act on, an admin
  // without this grant would consult their own voice profile rather than own it.
  voice_profile: ["read", "create", "update"],
  installation_settings: ["read", "update"],
  // Sign-in & apps, whose card reads the narrow projection.
  authentication_policy: ["read"],
  license: ["read"],
  // The sales vocabulary, with its WRITES. Reads alone opened every one of these
  // pages and still do — but the rail carries what a reader can act on, and a
  // fixture standing for "every page at once" has to be able to act on them or
  // the whole Sales heading is legitimately absent.
  pipeline: ["read", "create", "update"],
  custom_field: ["read", "create", "update"],
  tag: ["read", "create", "update"],
  product: ["read", "create", "update"],
  capture_settings: ["read", "update"],
  webhook_subscription: ["read", "create", "update"],
  knowledge_corpus: ["read", "create"],
  import_run: ["read", "create", "update"],
  ai_routing: ["read", "update"],
  automation: ["read", "create", "update"],
  ai_diagnostics: ["read"],
  // What ModelCostsCard writes. AI usage opens on `ai_diagnostics` and is ACTED
  // on through the rate table beside it, so a fixture meaning "every page" needs
  // both.
  ai_model_rate: ["read", "create", "update"],
  person: ["read"],
  // What opens Privacy & retention now that `person:read` does not.
  retention_policy: ["read", "create", "update"],
  audit_log: ["read"],
  job_health: ["read"],
  embedding_reindex: ["read", "update"],
  extension_access: ["read"],
  // Extensions is TWO reads: the unit inventory on `extension_access` and every
  // role's grant on every object on `role_admin` (identity/roles.go ListRoles).
  // The card fires both behind one flag, so the page needs both.
  role_admin: ["read", "update"],
  // The roster and team pages, which follow their own objects now rather than
  // opening for every authenticated reader.
  user_admin: ["read", "create", "update", "delete"],
  team_admin: ["read", "create", "update"],
  system_reset: ["delete"],
};

// Home leads this list too, for the same reason it leads pagesNamed: it is a
// row of the nav, and this is the nav's every row.
const EVERY_PAGE = [
  translate("en", "settings.home"),
  ...SETTINGS_PAGES.map((page) => labelOf(page.id)),
];

// The five reads the old Data model entry unioned, now spread across five pages
// of their own. Each still has to open its page ALONE: a page wired to one
// object with four decorative terms passes any fixture that grants all five.
// `custom_field` names TWO pages: the field editor and the lead vocabulary are
// both stored as custom fields and the server gates both on that object, so one
// revoked grant closes both — which a case naming only the editor would not
// say. `pipeline` names the stage designer alone.
//
// A record rather than a tuple, so the object and the pages keep their own
// types: a tuple with a variadic tail widens both halves to their union, and
// `readOn` would then accept a page id.
const SALES_READS: readonly {
  readonly object: RbacObject;
  readonly opens: readonly SettingsPageId[];
}[] = [
  { object: "custom_field", opens: ["leads", "fields"] },
  { object: "pipeline", opens: ["pipelines", "stageautomation"] },
  { object: "product", opens: ["products"] },
  { object: "offer_template", opens: ["products"] },
  { object: "tag", opens: ["tags"] },
];

// The seeded grant matrix's READ verbs. manager, read_only and rep hold the
// identical ten reads and differ only in the writes on top.
//
// Most pages open on a read, and for those this fixture is the whole story.
// Three exceptions ask a WRITE — company, integrations and automations — because
// the read that used to open them is one every seat holds: the base currency,
// whether capture is working, what an automation ran.
//
// Privacy is a fourth page that moved but NOT to a write. It dropped its
// `person:read` arm and kept two reads nobody below admin and ops holds at all,
// plus a `person` AND `consent_config` pair for the management role, which is
// seeded the consent vocabulary and nothing else on that page.
const SEEDED_READS: GrantSpec = {
  automation: ["read"],
  person: ["read"],
  capture_settings: ["read"],
  custom_field: ["read"],
  installation_settings: ["read"],
  knowledge_corpus: ["read"],
  offer_template: ["read"],
  // The write, because the seeded roles really hold it: rep carries
  // `company` create+update and manager carries all four. It is what keeps
  // Company profile open for them — the company profile the AI reads is a thing
  // a rep legitimately edits, which is why that page did not follow the other
  // three out of her rail.
  company: ["read", "create", "update"],
  overlay_connection: ["read"],
  pipeline: ["read"],
  product: ["read"],
  webhook_subscription: ["read"],
};

// What the matrix adds for ops: the objects it shares with admin alone.
// `embedding_reindex` is what opens System health, and `license` — which core
// migration 0261 grants to admin and ops and to nobody else — is what opens
// Seats & license.
//
// `ai_model_rate` opens AI usage and NOT Model calls, which is the two entries
// differing rather than a gap in this fixture: AI usage unions the model prices
// with `ai_diagnostics:read` because a rate-sheet author belongs on the page
// carrying the table, and Model calls asks `ai_diagnostics:read` alone.
const SEEDED_OPS_READS: GrantSpec = {
  ...SEEDED_READS,
  ai_model_rate: ["read"],
  embedding_reindex: ["read"],
  fx_rate: ["read"],
  retention_policy: ["read"],
  license: ["read"],
  // The writes ops actually holds in the seeded matrix, and the reason it keeps
  // the three configuration pages a rep no longer sees. Spelled here rather than
  // left to the reads above because those pages ask the write: a fixture that
  // gave ops only the reads would model a role the product does not seed, and
  // would then "prove" ops loses a page it does not lose.
  automation: ["read", "create", "update", "delete"],
  installation_settings: ["read", "update"],
  overlay_connection: ["read", "create", "update", "delete"],
  webhook_subscription: ["read", "create", "update", "delete"],
};

// `authentication` is NOT on this list, and that is the fix rather than an
// omission: SignInMethodsCard reads GET /installation/authentication-policy now,
// so the page asks for `authentication_policy` — which management and above
// hold — instead of the installation read every seeded role holds for the base
// currency. A sales rep does not see the installation's sign-in policy.
const SEEDED_READ_PAGES = pagesNamed(
  "account",
  "voice",
  "agents",
  "connections",
  "capture-activity",
  // `company` is NOT here: its requirement ANDs the company write with the
  // `company_context` deployment flag, and this fixture leaves that flag off.
  // The page's own availability cases are the ones that turn it on.
  //
  // Nor `members`/`teams`: only the `admin` role is seeded `user_admin` or
  // `team_admin`, so no seeded role below it reaches either page.
  "pipelines",
  // The same `pipeline:read` that opens Pipelines. The report is read-only, so
  // a seat that may see the stages may see what their transitions have earned.
  "stageautomation",
  "leads",
  "fields",
  "products",
  "capture",
  "knowledge",
);

const SEEDED_OPS_PAGES = pagesNamed(
  "account",
  "voice",
  "agents",
  "connections",
  "capture-activity",
  "company",
  // Not `members`/`teams`: ops holds neither `user_admin` nor `team_admin` in
  // the seeded matrix. Administering colleagues is the admin's, and the roster
  // ops reads for pickers is answered by the endpoint, not by this page.
  "seats",
  "pipelines",
  // The same `pipeline:read` that opens Pipelines. The report is read-only, so
  // a seat that may see the stages may see what their transitions have earned.
  "stageautomation",
  "leads",
  "fields",
  "products",
  "capture",
  "integrations",
  "knowledge",
  "automations",
  "usage",
  "privacy",
  "system-health",
);

describe("SettingsScreen page visibility", () => {
  // ONE gate, and every case below names it: the READ grant the page's own
  // cards ask for. Opening a page is reading it, so every requirement in the
  // catalog is a read apart from the reset page's delete; the write affordances
  // inside gate themselves, and no case here reaches a page by granting one.
  //
  // There is no seat or role gate above those requirements any more. One used to
  // sit over the whole admin half — admin-or-ops — and it answered false for
  // every page underneath whatever the page had decided, so a seat holding
  // `pipeline:read`, which every seeded role holds, was shown nothing while the
  // API answered it 200. The disagreement was invisible, because the page was
  // ABSENT rather than refused.

  it("renders every page in its declared order, under the group that claims it", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: EVERY_PAGE_GRANTED,
        // Armed, or the reset page is legitimately absent and EVERY_PAGE — which
        // is derived from the catalog — would be one row longer than the nav.
        dataResetAvailable: true,
      }),
    );
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(EVERY_PAGE));
    // And each page is under the heading that claims it: the flat order above
    // would read the same if a page were declared in the wrong group.
    const nav = screen.getByRole("navigation", { name: /primary navigation/i });
    // The level's own name leads them — it heads the group the Overview row is
    // in — so the subject headings start one along.
    const headings = within(nav).getAllByRole("heading", { level: 2 }).slice(1);
    // Asserted before any heading is read, so a level that lost a group fails
    // on the missing heading rather than on a lookup inside it.
    expect(headings.map((heading) => heading.textContent)).toEqual(
      SETTINGS_GROUPS.map(groupLabelOf),
    );
    for (const [index, group] of SETTINGS_GROUPS.entries()) {
      const heading = headings[index];
      if (!heading) {
        throw new Error(`the level published no heading for ${group}`);
      }
      expect(navGroupPages(heading)).toEqual(pagesIn(group));
    }
  });

  // The floor, and the reason removing the seat gate is not "everyone gets
  // everything". A principal holding NOTHING reaches the five personal pages and
  // no others: members and teams follow `user_admin`/`team_admin` now, and every
  // remaining page is absent because its requirement says so.
  //
  // ASSERTED THROUGH THE EVALUATOR, NOT THE RAIL, and that is not a shortcut.
  // For a grantless snapshot the rendered rail is byte-identical before and
  // after `/me` resolves — measured, not assumed — so no wait on the DOM can
  // tell the two apart, and any `waitFor` here passes on its first tick whatever
  // the requirements say. Granting every `user_admin` verb to this case and
  // keeping the expectation still passed while it went through the rail.
  //
  // `visibleSettingsPages` takes the snapshot as an argument, so there is no
  // in-flight state to race. The rail's own wiring is held by every other case
  // in this file, each of which asserts a page only a resolved snapshot draws.
  it("gives a principal holding no grant at all the pages that ask for none", () => {
    expect(
      visibleSettingsPages(meFixture({ roles: ["rep"], allow: {} })).map(
        (page) => page.id,
      ),
    ).toEqual(UNGATED_IDS);
  });

  it("gives an admin holding no grant at all exactly the same floor", () => {
    // The role is not a gate in either direction. It used to open three pages on
    // its own — extensions, the job report and the danger zone — and each of
    // those now follows a grant an edited role can hold or lose, so an admin
    // stripped of every grant reaches what anyone else stripped of every grant
    // reaches. A role that could not lose a page is a role that cannot be
    // edited.
    expect(
      visibleSettingsPages(meFixture({ roles: ["admin"], allow: {} })).map(
        (page) => page.id,
      ),
    ).toEqual(UNGATED_IDS);
  });

  // The grant, not the role name. These principals hold every read the seeded
  // matrix carries and are neither admin nor ops — and they reach every page
  // those reads open, because what the server answers 200 is what the product
  // offers.
  it.each(["manager", "rep"] as const)(
    "offers a seeded %s every page the reads it holds open",
    async (role) => {
      vi.stubGlobal(
        "fetch",
        settingsNavBackend({ roles: [role], allow: SEEDED_READS }),
      );
      renderHome();
      await waitFor(() => expect(offeredPages()).toEqual(SEEDED_READ_PAGES));
    },
  );

  // A write is still not what opens a page: this principal may AUTHOR custom
  // fields and holds no read anywhere, and the Fields row stays shut. The
  // affordance the write buys is on the page, and the page is reached by
  // reading it.
  it("opens no page for a principal holding writes and no read", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        // The witness rides along: `pipeline:read` opens Pipelines and Stage
        // automation and nothing else, so those rows prove /me resolved before
        // this asserts that the WRITES bought no page.
        allow: { custom_field: ["create", "update"], pipeline: ["read"] },
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
  });

  it.each(SALES_READS)(
    "opens the page $object opens, for that read alone",
    async ({ object, opens }) => {
      const allow = readOn(object);
      vi.stubGlobal("fetch", settingsNavBackend({ roles: ["ops"], allow }));
      renderHome();
      // `readOn` carries `person:read` with it as a floor, so a case about ONE
      // object stays about one object. It no longer opens Privacy: that page
      // asks the two governance objects nobody below admin and ops holds.
      await waitFor(() => expect(offeredPages()).toEqual(floorPlus(...opens)));
    },
  );

  it.each(["webhook_subscription", "overlay_connection"] as const)(
    "opens Integrations for a lone %s write, and not for its read",
    async (object) => {
      // The installation's outside wiring: every seeded role READS both objects,
      // because whether capture is working shows up on the records they already
      // open. Connecting a mirror or pointing a webhook somewhere is the work
      // the page exists for.
      //
      // The system-of-record chip asks this same question before it offers a
      // link, so the two cannot disagree about where that chip goes.
      vi.stubGlobal(
        "fetch",
        settingsNavBackend({
          roles: ["ops"],
          // The witness alongside the object under test: `pipeline` opens
          // Pipelines and Stage automation and nothing else, so waiting for
          // those rows proves the snapshot resolved before this asserts what
          // is NOT there.
          allow: { ...readOn(object), pipeline: ["read"] },
        }),
      );
      const { unmount } = renderHome();
      await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
      unmount();

      vi.stubGlobal(
        "fetch",
        settingsNavBackend({
          roles: ["ops"],
          allow: { ...readOn(object), [object]: ["read", "update"] },
        }),
      );
      renderHome();
      await waitFor(() =>
        expect(offeredPages()).toEqual(floorPlus("integrations")),
      );
    },
  );

  it("narrows nothing for a read seat", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        seat: "read",
        allow: SEEDED_OPS_READS,
      }),
    );
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(SEEDED_OPS_PAGES));
  });

  it("withholds the seats and system-health pages from a principal holding only the shared reads", async () => {
    // The grant still decides: this principal holds exactly the reads every
    // seeded role holds, so the pages whose grants belong to admin and ops alone
    // — the reindex read, `license:read` — are the ones it loses, and nothing
    // else moves.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: SEEDED_READS }),
    );
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(SEEDED_READ_PAGES));
    for (const page of ["seats", "system-health"] as const) {
      expect(screen.queryByRole("link", { name: labelOf(page) })).toBeNull();
    }
  });

  it("reaches those two for a seeded ops, whose reindex and licence reads open them", async () => {
    // The two pages that genuinely narrow, and they narrow to admin/ops rather
    // than to admin: ops holds both the reindex read and `license:read`, so each
    // opens on its grant and not on a role name — which is what lets an edited
    // role holding the same read reach them too.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: SEEDED_OPS_READS }),
    );
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(SEEDED_OPS_PAGES));
  });

  // An EDITED role, which is the case a seat gate would swallow. This admin
  // holds every grant the catalog asks for except one — and loses exactly the
  // one page that asks for it. A level gated on the seat alone would hand them
  // the licensing page their role no longer reads.
  it("loses only Seats & license for an admin whose role dropped that one read", async () => {
    const { license: _revoked, ...withoutSeats } = EVERY_PAGE_GRANTED;
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: withoutSeats,
        // Armed, so the ONE page this case is about losing is the one it names.
        dataResetAvailable: true,
      }),
    );
    renderHome();
    await waitFor(() =>
      expect(offeredPages()).toEqual(
        EVERY_PAGE.filter((label) => label !== labelOf("seats")),
      ),
    );
  });
});

// The home ROW's id is not a page id, and must never become one. If a page
// were ever added called `home`, its row and the home row would collide on
// `activeId` and one of them would go current on the other's address — and
// `#/settings/home` would stop being an unknown address and start being that
// page, silently.
//
// Derived from the catalog rather than restated: a list that named the ids by
// hand would agree with itself while the catalog moved underneath it.
it("keeps the home row's id out of the page vocabulary", () => {
  expect(SETTINGS_PAGES.map((page) => page.id)).not.toContain(SETTINGS_HOME_ID);
});

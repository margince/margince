/** @vitest-environment jsdom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { RbacObject } from "../app/capability";
import { type GrantSpec, meFixture } from "../app/mefixture";
import type { Route } from "../app/router";
import * as router from "../app/router";
import { SettingsRail } from "../app/shell";
import { translate } from "../i18n";
import { SettingsScreen } from "./settings";
import {
  jsonResponse,
  readOn,
  render,
  renderNav,
  settingsBackend,
} from "./settings.testkit";
import {
  SETTINGS_GROUPS,
  SETTINGS_PAGES,
  type SettingsGroupId,
  type SettingsPageId,
} from "./settingscatalog";
import { SETTINGS_HOME_ID } from "./settingsnav";
import { settingsHref } from "./settingsrouting";

// WHICH settings pages a principal is offered at all, and which group holds each
// one. The level is composed from the SETTINGS_PAGES catalog and the grant map
// /me carries, so every expectation here is DERIVED from that catalog rather
// than restated beside it: a list of labels written out by hand is a second
// source of truth, and nothing updates it.
//
// What a page then RENDERS is `settings.test.tsx` and its siblings' subject;
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

describe("SettingsScreen page layout", () => {
  // These layout assertions run as an admin holding the catalog grants the
  // testkit's fixture carries, so every page under test is present. Which
  // principal sees which page is the visibility suite's subject, not this one's.
  beforeEach(() => {
    vi.stubGlobal("fetch", settingsBackend());
  });

  it("groups the nav by subject, Account current by default", async () => {
    renderNav();
    // ONE navigation landmark in the chrome: the level names itself with a
    // heading rather than opening a second `nav` beside the sidebar's own.
    const nav = screen.getByRole("navigation", { name: /primary navigation/i });
    expect(
      within(nav).getByRole("heading", { level: 2, name: "Settings" }),
    ).toBeTruthy();
    // The granted pages appear once the /me probe resolves the grant map.
    await waitFor(() =>
      expect(screen.getByRole("link", { name: "Fields" })).toBeTruthy(),
    );
    // The headings the level carries, under its own title rather than beside
    // it. A group with no visible member is dropped rather than printed empty,
    // so this fixture's grants decide which of the seven appear — and the
    // subjects it does open are named in catalog order.
    expect(
      within(nav)
        .getAllByRole("heading", { level: 3 })
        .map((heading) => heading.textContent),
    ).toEqual(["You", "People", "Sales", "Governance"]);
    for (const label of [
      "Account",
      "Writing voice",
      "Agents",
      "Connections",
      "Members",
      "Fields",
      "Pipelines",
      "Privacy & audit",
    ]) {
      expect(screen.getByRole("link", { name: label })).toBeTruthy();
    }
    const account = screen.getByRole("link", { name: "Account" });
    expect(account.getAttribute("aria-current")).toBe("page");
    // Every row addresses the level's own depth. The group a page sits under is
    // no longer a segment in its address, so a page that changed group would
    // keep the link a reader bookmarked.
    expect(account.getAttribute("href")).toBe("#/settings/account");
    expect(
      screen.getByRole("link", { name: "Fields" }).getAttribute("href"),
    ).toBe("#/settings/fields");
  });

  it("renders only the active page's cards — the passport is off the Account page", async () => {
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());
    // Scout lives on Agents; the default Account page must not render it.
    expect(screen.queryByText("Scout")).toBeNull();
  });

  it("renders the custom-field editor itself on the Fields page, never a door to it", async () => {
    render(<SettingsScreen route={settingsHref("fields")} />);
    // Visible once /me resolves the custom_field read grant.
    expect(
      await screen.findByRole("heading", { name: "Custom fields" }),
    ).toBeTruthy();
    // The editor IS the content now, so nothing on the page navigates to it.
    expect(screen.queryByRole("link", { name: /custom fields/i })).toBeNull();
  });

  it("renders the pipeline designer inline on its own page, never a door to it", async () => {
    render(<SettingsScreen route={settingsHref("pipelines")} />);
    expect(
      await screen.findByRole("heading", { name: "Pipelines" }),
    ).toBeTruthy();
    // A former standalone screen is inline content: the door-card that stood in
    // for it is gone rather than relabelled.
    const hrefs = screen
      .queryAllByRole("link")
      .map((link) => link.getAttribute("href"));
    expect(hrefs).not.toContain("#/pipelines");
  });

  it("renders the product and offer-template surfaces on one page, never doors to them", async () => {
    // The two priced surfaces share a page, so the claim spans both: each was a
    // standalone screen behind a door-card before, and both doors are gone.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("product") }),
    );
    render(<SettingsScreen route={settingsHref("products")} />);
    expect(
      await screen.findByRole("heading", { name: "Products" }),
    ).toBeTruthy();
    expect(
      screen.getByRole("heading", { name: "Offer templates" }),
    ).toBeTruthy();
    const hrefs = screen
      .queryAllByRole("link")
      .map((link) => link.getAttribute("href"));
    expect(hrefs).not.toContain("#/products");
    expect(hrefs).not.toContain("#/offer-templates");
  });
});

// The nav, driven by exactly the two things the catalog composes: the grant map
// /me carries, and the company-context rollout flag beside it. Every other
// endpoint answers empty, so a failure here can only be about visibility.
function settingsNavBackend(opts: {
  roles: string[];
  allow?: GrantSpec;
  // The licensing seat, which the catalog deliberately leaves out: a read seat
  // still READS every page behind these entries, so a case can name the seat and
  // expect the nav not to narrow.
  seat?: "full" | "read";
  companyReadEnabled?: boolean;
  // A server that predates `settings_availability` answers /me without it.
  // The catalog has to read that as "the surface does not exist" rather than
  // as permission, so a case can ask for the field to be absent entirely.
  omitAvailability?: true;
  // Whether this DEPLOYMENT permits a data reset. The compiled default is false
  // everywhere, so the reset page is absent unless a case arms it — which is
  // the behaviour, not a fixture convenience: the page needs the grant AND the
  // deployment's own consent.
  dataResetAvailable?: true;
}) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      const me = meFixture({
        roles: opts.roles,
        seat: opts.seat ?? "full",
        allow: opts.allow ?? {},
        settingsAvailability: opts.omitAvailability
          ? null
          : { company_context: opts.companyReadEnabled ?? false },
      });
      return jsonResponse({
        ...me,
        data_reset_available: opts.dataResetAvailable ?? false,
      });
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

// The settings pages currently in the nav, in render order. Asserting the WHOLE
// list rather than one membership is the point: a requirement wired to the wrong
// object shows up as an extra or a missing row, where a single getBy would pass
// regardless.
function navPages(): string[] {
  return screen
    .getAllByRole("link")
    .filter((link) => {
      const href = link.getAttribute("href") ?? "";
      // `#/settings` exactly is Settings home — a row of this level whose
      // address is the level itself. The trailing slash alone would drop it,
      // which is how a helper quietly stops seeing the row it was meant to
      // prove is there.
      return href === "#/settings" || href.startsWith("#/settings/");
    })
    .map((link) => link.textContent ?? "");
}

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

// The expected labels, DERIVED from the catalog rather than restated beside it.
// A list of labels beside a list of pages is a second source of truth, and
// nothing updates it: the restated lists this replaces omitted `license` — a
// fully wired entry with a predicate, content, labels in two locales and a deep
// link from the sidebar's seat meter — so every assertion in this file,
// including the ones claiming to walk the whole level, passed while checking one
// entry fewer than existed.
const labelOf = (id: SettingsPageId) => translate("en", `settings.tab.${id}`);
const groupLabelOf = (group: SettingsGroupId) =>
  translate("en", `settings.group.${group}`);

// The pages a caller reaches by naming a set of page ids, in CATALOG order —
// so a case states which pages a grant opens and the order comes from the one
// table that decides it, never from the order the case happened to list them.
// Settings home leads every list, for every reader. It is not a page and no
// grant reaches it — it is the address with no page segment — so it belongs in
// the shared expectation rather than in each case that would otherwise have to
// remember it.
const pagesNamed = (...ids: readonly SettingsPageId[]) => [
  translate("en", "settings.home"),
  ...SETTINGS_PAGES.filter((page) => ids.includes(page.id)).map((page) =>
    labelOf(page.id),
  ),
];

// Without the home row: a claim about ONE group's rows is not a claim about the
// headingless row above all of them.
const pagesIn = (group: SettingsGroupId) =>
  SETTINGS_PAGES.filter((page) => page.group === group).map((page) =>
    labelOf(page.id),
  );

// What every reader gets: the personal pages, which carry no requirement at
// all, and the two People pages that ask for none.
//
// `agents` and `connections` are personal because what they carry is the
// PERSON's: gating `agents` would regress passport minting for every seat that
// is not an admin, and a mailbox and a LinkedIn network nobody else can see are
// not the installation's configuration.
//
// `members` and `teams` are the floor beside them for a different reason:
// `GET /users` answers 200 to any authenticated principal and "who is on my
// team" is not an admin's private question, so the pages open for everyone and
// their controls withhold themselves.
const UNGATED_IDS = [
  ...SETTINGS_PAGES.filter((page) => page.group === "me").map(
    (page) => page.id,
  ),
  "members",
  "teams",
] as const satisfies readonly SettingsPageId[];
const UNGATED_PAGES = pagesNamed(...UNGATED_IDS);

// The floor plus the pages a grant opens, in CATALOG order. Concatenating the
// two lists instead would assert an order the catalog does not have: `company`
// and `authentication` are declared BEFORE `members`, so a page's position
// comes from the table rather than from which half of the fixture named it.
const floorPlus = (...ids: readonly SettingsPageId[]) =>
  pagesNamed(...UNGATED_IDS, ...ids);

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
const EVERY_PAGE_GRANTED: GrantSpec = {
  installation_settings: ["read"],
  // Sign-in & apps, whose card reads the narrow projection.
  authentication_policy: ["read"],
  license: ["read"],
  pipeline: ["read"],
  custom_field: ["read"],
  tag: ["read"],
  product: ["read"],
  capture_settings: ["read"],
  webhook_subscription: ["read"],
  knowledge_corpus: ["read"],
  import_run: ["read"],
  ai_routing: ["read"],
  automation: ["read"],
  ai_diagnostics: ["read"],
  person: ["read"],
  audit_log: ["read"],
  job_health: ["read"],
  embedding_reindex: ["read"],
  extension_access: ["read"],
  // Extensions is TWO reads: the unit inventory on `extension_access` and every
  // role's grant on every object on `role_admin` (identity/roles.go ListRoles).
  // The card fires both behind one flag, so the page needs both.
  role_admin: ["read"],
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
  { object: "pipeline", opens: ["pipelines"] },
  { object: "product", opens: ["products"] },
  { object: "offer_template", opens: ["products"] },
  { object: "tag", opens: ["tags"] },
];

// The seeded grant matrix, READ verbs only — the only verb a page requirement
// asks for, apart from the reset page's delete. manager, read_only and rep hold
// the identical ten reads and differ only in the writes on top, which is exactly
// why write-shaped predicates hid pages the server serves: the differentiation
// the matrix carries lives in the writes, and a write is not what opens a page.
const SEEDED_READS: GrantSpec = {
  automation: ["read"],
  person: ["read"],
  capture_settings: ["read"],
  custom_field: ["read"],
  installation_settings: ["read"],
  knowledge_corpus: ["read"],
  offer_template: ["read"],
  organization: ["read"],
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
  "company",
  "members",
  "teams",
  "pipelines",
  "leads",
  "fields",
  "products",
  "capture",
  "integrations",
  "knowledge",
  "automations",
  "privacy",
);

const SEEDED_OPS_PAGES = pagesNamed(
  "account",
  "voice",
  "agents",
  "connections",
  "capture-activity",
  "company",
  "members",
  "teams",
  "seats",
  "pipelines",
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
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(EVERY_PAGE));
    // And each page is under the heading that claims it: the flat order above
    // would read the same if a page were declared in the wrong group.
    const nav = screen.getByRole("navigation", { name: /primary navigation/i });
    const headings = within(nav).getAllByRole("heading", { level: 3 });
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

  it("gives a principal holding no grant at all the pages that ask for none", async () => {
    // The floor, and the reason removing the seat gate is not "everyone gets
    // everything". A principal holding NOTHING reaches the five personal pages
    // plus the member roster and the team list, which `GET /users` serves to
    // anyone signed in and which the same handler narrows by role — a rep's page
    // carries no role keys and no inactive members. Every other page is absent
    // because its requirement says so, which is the same sentence for this
    // principal as for an admin.
    //
    // Privacy is NOT on the floor with them: the consent registry's server gate
    // is `person:read`, and a principal holding nothing does not hold it.
    vi.stubGlobal("fetch", settingsNavBackend({ roles: ["rep"], allow: {} }));
    renderNav();
    // /me has to have SETTLED before this claim means anything: a nav read
    // mid-flight is empty for every principal. Waiting on the rows themselves is
    // what proves it settled — the sidebar no longer prints the signed-in
    // address, which is what this used to wait for, because the account block
    // moved to the top bar.
    await waitFor(() => expect(navPages()).toEqual(UNGATED_PAGES));
  });

  it("gives an admin holding no grant at all exactly the same floor", async () => {
    // The role is not a gate in either direction. It used to open three pages on
    // its own — extensions, the job report and the danger zone — and each of
    // those now follows a grant an edited role can hold or lose, so an admin
    // stripped of every grant reaches what anyone else stripped of every grant
    // reaches. A role that could not lose a page is a role that cannot be
    // edited.
    vi.stubGlobal("fetch", settingsNavBackend({ roles: ["admin"], allow: {} }));
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(UNGATED_PAGES));
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
      renderNav();
      await waitFor(() => expect(navPages()).toEqual(SEEDED_READ_PAGES));
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
        allow: { custom_field: ["create", "update"] },
      }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(UNGATED_PAGES));
  });

  it.each(SALES_READS)(
    "opens the page $object opens, for that read alone",
    async ({ object, opens }) => {
      const allow = readOn(object);
      vi.stubGlobal("fetch", settingsNavBackend({ roles: ["ops"], allow }));
      renderNav();
      // `readOn` carries `person:read` with it, which is what opens Privacy —
      // the floor a case about ONE object stands on rather than part of what it
      // proves.
      await waitFor(() =>
        expect(navPages()).toEqual(floorPlus(...opens, "privacy")),
      );
    },
  );

  it.each(["webhook_subscription", "overlay_connection"] as const)(
    "opens Integrations for a lone %s read",
    async (object) => {
      // The installation's outside wiring, and the system-of-record chip in the
      // topbar points every seat at it — so either read has to open the page on
      // its own, or whoever follows that chip lands on the Account fallback.
      const allow = readOn(object);
      vi.stubGlobal("fetch", settingsNavBackend({ roles: ["ops"], allow }));
      renderNav();
      await waitFor(() =>
        expect(navPages()).toEqual(floorPlus("integrations", "privacy")),
      );
    },
  );

  it("opens Sign-in & apps on authentication_policy, its card's own grant", async () => {
    // SignInMethodsCard reads GET /installation/authentication-policy now, so
    // the page asks for the grant that endpoint takes.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["management"],
        allow: readOn("authentication_policy"),
      }),
    );
    renderNav();
    // `privacy` rides the floor here: meFixture grants `person:read`, which is
    // one of that page's union terms — the consent registry's own server gate.
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("authentication", "privacy")),
    );
  });

  it("does NOT open it on the read every seeded role holds", async () => {
    // The disclosure the backend split closed, asserted from the client side.
    // `installation_settings:read` is held by every role — a rep reads it for
    // the base currency — so a page opening on it would put the installation's
    // sign-in policy in front of the whole workspace.
    //
    // It still opens Company profile, which is one of that page's three union
    // terms and is meant to be readable by everyone.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["rep"],
        allow: readOn("installation_settings"),
      }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("company", "privacy")),
    );
  });

  it("opens Seats & license for a lone license read", async () => {
    // `LicenseCard` calls `/installation/license` and nothing else, so this is
    // the grant that actually reaches content. Core migration 0261 grants
    // `license` to admin and ops and to nobody else, so it is still a grant an
    // edited role can hold rather than a role name.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("license") }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("seats", "privacy")),
    );
  });

  it("opens Seats & license for a lone seat_usage read", async () => {
    // The capacity half of the split, and the reader it shipped for:
    // management, which may see headcount without commercial standing.
    // `LicenseCard` reads `/installation/license` for a `license` holder and
    // falls back to `/installation/seat-usage` for this one, so the page it
    // opens has content rather than a failed entitlement read.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("seat_usage") }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("seats", "privacy")),
    );
    cleanup();

    // The other half, or the assertion above would pass against a page that
    // opened for everybody: a seat holding neither read does not reach it.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("person") }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(floorPlus("privacy")));
  });

  it("opens Capture for a lone capture_settings read", async () => {
    // Two surfaces both called "Capture" became one page, and this is the read
    // the merged page asks for. Granted alone so a Capture wired to a
    // neighbouring object, or a neighbour wired to this one, shows up as a row
    // the whole-list assertion does not expect.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("capture_settings") }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("capture", "privacy")),
    );
  });

  it("opens System health for a lone embedding_reindex read, for a principal who is no admin", async () => {
    // The reindex and the job report share a page, and the reindex read is the
    // half the cards honour. Taking the page away from a principal who could
    // reach the reindex before would be a regression dressed as a tidy-up — and
    // asking the grant rather than the admin role is what lets an edited role
    // reach it, which a role check could never express.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        allow: readOn("embedding_reindex"),
      }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("privacy", "system-health")),
    );
  });

  it("opens System health for a lone job_health read", async () => {
    // The job report's own object, which `GET /v1/jobs/health` asks for and
    // which `JobHealthCard` now asks for too — where it used to ask whether the
    // reader WAS an admin, and so refused an ops seat the server answers 200.
    //
    // The page unions this with the reindex read, and each term has to open it
    // alone or the union is one object with a decorative second term.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("job_health") }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("privacy", "system-health")),
    );
    cleanup();

    // And a seat holding neither term still does not reach it, or the case
    // above would pass against a page that opened for everybody.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("person") }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(floorPlus("privacy")));
  });

  it("opens Extensions for a lone extension_access read", async () => {
    // `GET /v1/extensions` asks for `extension_access:read`, and
    // `ExtensionAccessCard` asks the same object — where it used to ask whether
    // the reader WAS an admin, which refused an ops seat the endpoint answers.
    //
    // The read is the WHOLE gate now: the entry used to AND it with
    // `system_reset:delete` as a stand-in for "is admin", and dropping that
    // term is what lets an edited role reach the page.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        // `person:read` is the floor `readOn` holds steady for every other case
        // here — without it this stops being a case about Extensions and also
        // becomes one about losing Privacy.
        allow: {
          person: ["read"],
          extension_access: ["read"],
          role_admin: ["read"],
        },
      }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("privacy", "extensions")),
    );
    cleanup();

    // The inventory read ALONE is not the page. The card makes a second request
    // for every role's grants, which asks `role_admin:read` — so on this grant
    // the page opened and the card 403'd inside it, which is an unreadable page
    // rather than a narrower one.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("extension_access") }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(floorPlus("privacy")));
    cleanup();

    // And the read is load-bearing rather than decorative: an ADMIN who lost it
    // does not reach the page, which is what says the entry stopped asking for
    // the role.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: { person: ["read"], system_reset: ["delete"] },
      }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(floorPlus("privacy")));
  });

  it("opens Reset data on the delete verb, and never on a read of the same object", async () => {
    // Two conditions of different kinds, and the page needs BOTH — the only
    // requirement in the catalog shaped that way.
    //
    // The verb first: emptying an installation is not a thing you read, so a
    // `system_reset:read` must not reach it even on an armed deployment.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: readOn("system_reset"),
        dataResetAvailable: true,
      }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(UNGATED_PAGES));
    cleanup();

    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: { person: ["read"], system_reset: ["delete"] },
        dataResetAvailable: true,
      }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("privacy", "reset")),
    );
    cleanup();

    // And the deployment's own consent, which is not a permission: the same
    // holder on an installation that never opted in has no such destination.
    // The compiled default is false everywhere, so this is the ordinary case
    // rather than the exotic one.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: { person: ["read"], system_reset: ["delete"] },
      }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(floorPlus("privacy")));
  });

  it("opens Company profile for a lone fx_rate read, and no other page with it", async () => {
    // The currency table joined the base currency it converts to, so fx_rate is
    // one of the three terms Company profile's requirement unions — this read
    // alone has to open it, and the neighbouring pages have to stay shut.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("fx_rate") }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("company", "privacy")),
    );
  });

  it("opens AI usage for a lone ai_model_rate read", async () => {
    // Model prices joined the usage figures they price, and either term of that
    // page's requirement opens it on its own — so the union has to be read as a
    // union and not as one object with a decorative second term.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("ai_model_rate") }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(floorPlus("usage", "privacy")),
    );
  });

  it("opens both AI diagnostics pages for a lone ai_diagnostics read", async () => {
    // `ai_diagnostics` is the object the server moved these reads onto, and
    // `AiUsageCard`, `AiCallsCard` and `AiHealthCard` all ask for it now. They
    // used to ask `automation:update` — a write verb guarding a GET, from when
    // the runtime's spend was operator information.
    //
    // One grant opens two pages, which is what makes granting it alone worth
    // asserting: a Model calls wired to some other object would be invisible
    // here and everywhere else.
    //
    // THREE pages, not two: `AiHealthCard` reads on this object too and Models
    // is the only page that renders it, so a Models shut on `ai_routing` alone
    // put that card behind a door its own reader could not open. Management is
    // seeded diagnostics WITHOUT routing, which is exactly that reader.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("ai_diagnostics") }),
    );
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(
        floorPlus("models", "usage", "model-calls", "privacy"),
      ),
    );
  });

  it("keeps both AI diagnostics pages shut for the automation write the cards used to check", async () => {
    // The other half of the move, and the reason it is a move rather than a
    // widening: `automation:update` no longer reaches either page. An
    // automation editor is not thereby entitled to the installation's model
    // spend, and a case asserting only the positive above would pass whether or
    // not the old term was dropped.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        allow: { person: ["read"], automation: ["update"] },
      }),
    );
    renderNav();
    // `automations` stays shut too: it asks for `automation:read`, and the
    // write on the same object is not it. A page is reached by reading it.
    await waitFor(() => expect(navPages()).toEqual(floorPlus("privacy")));
  });

  it.each(["retention_policy", "privacy_request", "person"] as const)(
    "opens Privacy for a lone %s read",
    async (object) => {
      // Three terms, each opening the page alone — the retention ladder, the DSR
      // queue, and `person`, which is the gate the registry endpoint actually
      // applies (consent/store.go's ListPurposes calls auth.Require on
      // person:read). A union read as one object with two decorative terms would
      // pass any fixture granting all three.
      //
      // `consent_config` is deliberately NOT a term. It is what the purposes card
      // ADMINISTERS, but it buys no read: only the writes moved to it. On that
      // grant alone the page opened with every card inside it withheld.
      const allow: GrantSpec = {};
      allow[object] = ["read"];
      vi.stubGlobal("fetch", settingsNavBackend({ roles: ["rep"], allow }));
      renderNav();
      await waitFor(() => expect(navPages()).toEqual(floorPlus("privacy")));
    },
  );

  // The term that was dropped, asserted as an absence so nobody adds it back
  // without meeting the card that would have to read on it.
  it("does not open Privacy for a lone consent_config read", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["custom"],
        allow: { consent_config: ["read", "create"] },
      }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(floorPlus()));
  });

  it("opens Audit log without opening Privacy, for an admin holding the trail read", async () => {
    // The trail was split off the privacy page because it answers to a
    // DIFFERENT grant: a reader could hold `audit_log:read` and be refused the
    // page carrying it. Granting the trail read and NOT `person:read` is what
    // proves the split — the two pages move independently.
    //
    // The trail read is the WHOLE gate now: the entry used to AND it with
    // `system_reset:delete` as a stand-in for "is admin", because `AuditLogCard`
    // asked whether the reader WAS one. Both card and entry ask `audit_log:read`
    // — what `GET /v1/audit-log` asks for — so nothing rides along.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: { audit_log: ["read"] },
      }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(floorPlus("audit")));
  });

  it("opens Audit log for a delegated audit_log holder who is no admin", async () => {
    // The reader the split shipped for: a management seat holding the trail
    // read and no admin role. Both the entry and `AuditLogCard` ask
    // `audit_log:read`, so this holder reaches the page and the trail on it —
    // where the role check refused them a page the server answers 200.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["management"],
        allow: { audit_log: ["read"] },
      }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(floorPlus("audit")));
    cleanup();

    // And a management seat WITHOUT the read still does not reach it, or the
    // assertion above would pass against a page that opened for everybody.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["management"], allow: {} }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(UNGATED_PAGES));
  });

  // THE LICENSING SEAT, which is a THIRD axis and gates none of this: the server
  // clamps a read seat on the HTTP method, so it still READS every page behind
  // these rows. A principal on a read seat therefore reaches the level
  // undiminished, and the withheld things inside are the write controls.
  //
  // Named as its own case because folding it into the requirements is the
  // regression this rule exists to prevent: measured against the live API, the
  // write-shaped predicates hid a read seat from eight of the eleven entries the
  // server answers 200 on — three of which (products, offer templates, custom
  // fields) were ungated routes of their own before the merge.
  it("narrows nothing for a read seat", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        seat: "read",
        allow: SEEDED_OPS_READS,
      }),
    );
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(SEEDED_OPS_PAGES));
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
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(SEEDED_READ_PAGES));
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
    renderNav();
    await waitFor(() => expect(navPages()).toEqual(SEEDED_OPS_PAGES));
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
    renderNav();
    await waitFor(() =>
      expect(navPages()).toEqual(
        EVERY_PAGE.filter((label) => label !== labelOf("seats")),
      ),
    );
  });

  it("shows Company profile to an admin holding the organization read once the company rollout flag is on", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: readOn("organization"),
        companyReadEnabled: true,
      }),
    );
    renderNav();
    expect(
      await screen.findByRole("link", { name: labelOf("company") }),
    ).toBeTruthy();
  });

  it("withholds Company profile from that same admin while the rollout flag is off", async () => {
    // The flag is a deployment posture, not a permission, so it ANDs with the
    // grant beside it: the company profile may simply not exist on this
    // installation.
    //
    // This used to assert two moments — the nav composed while the flag was
    // still in flight, then again once it answered — because the fact arrived
    // over its own request and a row could appear and then vanish. It rides /me
    // now, so there is no in-flight window to hold open: the nav cannot render
    // before the snapshot it reads. The race is gone rather than untested, which
    // is why the second moment went with it.
    //
    // The organization read is the ONLY term of Company profile's requirement
    // this fixture grants, which is what leaves the flag decisive. On a seeded
    // installation every role also holds `installation_settings:read` and the
    // page opens on that regardless — so this is about the flag's contribution
    // to the union, not a claim that the page is ever unreachable in practice.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: readOn("organization"),
        companyReadEnabled: false,
      }),
    );
    renderNav();

    await waitFor(() => expect(navPages()).toEqual(floorPlus("privacy")));
    expect(screen.queryByRole("link", { name: labelOf("company") })).toBeNull();
  });

  it("withholds Company profile when /me carries no availability at all", async () => {
    // The absent case, which is a DIFFERENT fact from the flag reading false: a
    // server older than `settings_availability` answers /me without the object,
    // and a browser holding a cached snapshot from before the field shipped does
    // the same. Both are states a running deployment reaches during a rollout,
    // and neither says the company profile exists.
    //
    // Without this case the catalog's `?? false` is unheld — flipping it to
    // `?? true` passes every other test in this file, because they all supply
    // the field. What that flip ships is a page offered on an installation that
    // may not have the surface, which is the one direction a deployment fact
    // must not fail.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: readOn("organization"),
        omitAvailability: true,
      }),
    );
    renderNav();

    await waitFor(() => expect(navPages()).toEqual(floorPlus("privacy")));
    expect(screen.queryByRole("link", { name: labelOf("company") })).toBeNull();
  });
});

// The access boundary, which replaced a silent fallback.
//
// The fallback was wrong in a way the reader could not see: a denied address
// rendered Account AND rewrote the URL to say Account, so somebody following a
// colleague's link had no evidence the link had gone anywhere else. They would
// report it broken; the sender would open it and find it worked.
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

// The search box in the rail searches the READER'S pages, not the catalog.
//
// The box's own tests hand it a page list directly, so they cannot see which
// list the rail passes — a wiring that handed it SETTINGS_PAGES would offer a
// rep the audit log, and every one of those tests would still pass. This is the
// case that fails when that happens.
describe("the settings search in the rail", () => {
  it("offers a rep no page their own sidebar does not draw", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", settingsNavBackend({ roles: ["rep"], allow: {} }));
    renderNav();

    // Waited on a row a resolved snapshot draws, so the empty result below is
    // about the grants rather than about a rail that has not loaded.
    await screen.findByRole("link", { name: labelOf("account") });
    await user.type(screen.getByRole("combobox"), "audit");

    expect(screen.queryAllByRole("option")).toHaveLength(0);
  });

  // The control: the same word, one grant apart. Without it the case above
  // would pass against a search that finds nothing for anybody.
  it("offers the page to a reader who holds its grant", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["admin"], allow: readOn("audit_log") }),
    );
    renderNav();

    await screen.findByRole("link", { name: labelOf("audit") });
    await user.type(screen.getByRole("combobox"), "audit");

    expect(
      screen
        .queryAllByRole("option")
        .some((option) => option.textContent?.includes(labelOf("audit"))),
    ).toBe(true);
  });
});

describe("the settings access boundary", () => {
  it("tells a reader the page is not theirs, and leaves the address alone", async () => {
    const replaced: Route[] = [];
    vi.spyOn(router, "navigateReplacing").mockImplementation((route) => {
      replaced.push(route);
    });
    vi.stubGlobal("fetch", settingsNavBackend({ roles: ["rep"], allow: {} }));
    // Both halves, because the claim spans them: the SCREEN says the page is
    // not theirs, and the RAIL must not go on marking a page current beside it.
    render(
      <>
        <SettingsRail route={settingsHref("audit")} />
        <SettingsScreen route={settingsHref("audit")} />
      </>,
    );

    // The boundary is ALSO what renders while /me is in flight — every
    // capability predicate reads false until the snapshot lands — so finding it
    // proves nothing on its own. Waited on a row only a RESOLVED snapshot can
    // draw, and only then asserted the denial, which is what makes it a claim
    // about the grant rather than about the load.
    expect(
      await screen.findByRole("link", { name: labelOf("account") }),
    ).toBeTruthy();
    expect(
      screen.getByText(/this settings page is not yours to open/i),
    ).toBeTruthy();
    // Account's own content is what the fallback used to show here.
    expect(screen.queryByText("test@example.test")).toBeNull();
    // And the address is untouched, which is the whole affordance: the reader
    // can read what they asked for and quote it to somebody who holds it.
    expect(replaced).toEqual([]);
    // Nor does the CHROME claim a page. The sidebar used to mark Account
    // current beside a body saying "not yours" — half the false fallback,
    // living on in the rail.
    expect(
      screen
        .getByRole("link", { name: labelOf("account") })
        .getAttribute("aria-current"),
    ).toBeNull();
  });

  it("tells a reader an address names no page, which is a different fact", async () => {
    vi.stubGlobal("fetch", settingsNavBackend({ roles: ["admin"], allow: {} }));
    render(
      <SettingsScreen route={{ screen: "settings", id: "no-such-page" }} />,
    );

    expect(
      await screen.findByText(/no settings page has this address/i),
    ).toBeTruthy();
    // Not the denial: an admin holding every grant is refused nothing, and
    // telling them the page is "not theirs" would send them asking for a grant
    // that would not help.
    expect(screen.queryByText(/not yours to open/i)).toBeNull();
  });

  it("opens the page for a reader who does hold the grant", async () => {
    // The control: the same address, one grant apart. Without it the two cases
    // above would pass against a page nobody can ever open.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["admin"], allow: readOn("audit_log") }),
    );
    render(<SettingsScreen route={settingsHref("audit")} />);

    await waitFor(() =>
      expect(screen.queryByText(/not yours to open/i)).toBeNull(),
    );
    expect(
      screen.getByText(translate("en", "settings.tab.audit")),
    ).toBeTruthy();
  });
});

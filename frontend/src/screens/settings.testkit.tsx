import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  type RenderResult,
  render as rtlRender,
  screen,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { expect, vi } from "vitest";
import type { RbacObject } from "../app/capability";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { SettingsRail } from "../app/shell";
import { LocaleProvider, translate } from "../i18n";
import { SettingsScreen, settingsAddress } from "./settings";
import { SETTINGS_PAGES, type SettingsPageId } from "./settingscatalog";

// The render helpers and grant fixtures every `settings*.test.tsx` suite needs,
// in one place. Settings is ONE route carrying fourteen entries, so its coverage
// is split by subject across several files — and each of them wants the same
// three things: a fetch mock that answers per endpoint, a render that carries
// the query client and the locale the screen reads, and the grant fixtures that
// decide what a principal is shown.
//
// It is NOT a *.test.* file, on purpose: the design-system and lint gates skip
// test files, and a helper that renders the real screen should answer to the
// same rules the screen does.

// The content type is part of the answer, not decoration: the API client reads
// it before it parses, so a mock that omits it is not the response the product
// receives.
export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// What a settings render hands back, written out rather than inferred, and the
// annotation is load-bearing: this module is compiled by the app project, which
// emits declarations, and an inferred return type here reaches a transitive
// dependency of the testing library that tsc cannot name from outside it.
export type SettingsRender = RenderResult & { client: QueryClient };

// The client comes back with the render so a test can read a query's settled
// state, not just the DOM: "the answer is in the cache" is the fact a nav
// assertion about an absent tab has to stand on.
export const render = (ui: ReactNode): SettingsRender => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return {
    ...rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">{ui}</LocaleProvider>
      </QueryClientProvider>,
    ),
    client,
  };
};

// A settings route renders in two halves, and the sidebar owns one of them: the
// tabs are the shell's SECOND NAVIGATION LEVEL, fed by the section this screen
// publishes (useSettingsSection). So a claim about which tabs a principal is
// offered renders the real rail — the production wiring, not a copy of it — and
// a claim about a tab's content renders the screen.
// The bare settings address is Settings HOME now, not the first page — so a
// suite that means "the default page" has to say which page that is. `account`
// is what the fallback used to land on, and every case here that omitted a tab
// meant exactly that.
const DEFAULT_TAB = "account";

const railFor = (tab?: string) => (
  <SettingsRail route={settingsAddress(tab ?? DEFAULT_TAB)} />
);

export const renderNav = (tab?: string): SettingsRender => render(railFor(tab));

// Settings home, which no `tab` can name: it is the address with NO page
// segment. Its own helper rather than a magic argument, so a case about home
// reads as one.
export const renderHome = (): SettingsRender =>
  render(
    <>
      <SettingsRail route={settingsAddress()} />
      <SettingsScreen route={settingsAddress()} />
    </>,
  );

// Both halves, for a claim that spans them: the tab is in the nav AND its cards
// are on the page.
export const renderSettings = (tab?: string): SettingsRender =>
  render(
    <>
      {railFor(tab)}
      <SettingsScreen route={settingsAddress(tab ?? DEFAULT_TAB)} />
    </>,
  );

// The Admin settings tab group is composed from its MEMBERS, and OPENING AN ENTRY
// IS A READ: every predicate asks for a read grant on something the entry shows,
// while the write affordances inside it gate themselves. So a fixture that wants
// an entry in the nav has to name the READ, and one that also wants the authoring
// controls names the write on top — two separate claims, and a grant list holding
// writes alone reaches no entry at all.
export const PIPELINE_ADMIN: GrantSpec = {
  pipeline: ["read", "create", "update"],
};
const ADMIN_GRANTS: GrantSpec = {
  ...PIPELINE_ADMIN,
  custom_field: ["read", "create", "update"],
  // A seeded admin holds all four verbs on their own voice profile. It was
  // absent while nothing asked — Writing voice opened for everybody — and its
  // absence made this fixture describe an account the product never issues.
  voice_profile: ["read", "create", "update", "delete"],
  // The consent registry's own gate (consent/store.go demands person:read),
  // which every seeded role holds. It is the floor a fixture standing in for a
  // real principal carries — but it no longer OPENS Privacy: that page asks
  // `retention_policy` or `privacy_request`, neither of which anybody below
  // admin and ops holds.
  person: ["read"],
  // What actually opens Privacy & retention for this admin fixture. Named here
  // rather than left to `person`, because the page moved off the read every
  // seat holds and a fixture that did not follow would quietly stop rendering
  // the page its cases are about.
  retention_policy: ["read", "create", "update"],
  // The roster and team pages. They used to open for every authenticated
  // reader; they follow `user_admin` and `team_admin` now, and the layout cases
  // that render this fixture expect both present.
  user_admin: ["read", "create", "update", "delete"],
  team_admin: ["read", "create", "update"],
};

// The read grant on ONE object, as a GrantSpec.
//
// Built by assignment rather than as a literal, because a computed key whose own
// type is a union widens the object to `{ [x: string]: string[] }` — which does
// not satisfy GrantSpec, and only fails in `tsc -b`, where test files are
// typechecked, rather than under the app project alone.
export function readOn(object: RbacObject): GrantSpec {
  // `person:read` rides along because every seeded role holds it and the consent
  // registry's own endpoint demands it — so a case about ONE object's entry is
  // not also a case about losing that read. Isolating the object under test
  // means holding the floor steady, not stripping it.
  //
  // The floor no longer reaches a PAGE. Privacy used to open on it, which made
  // every `readOn` case also a case about Privacy; the page asks the two
  // governance objects now, and the expectations lost their trailing "privacy".
  const spec: GrantSpec = { person: ["read"] };
  spec[object] = ["read"];
  return spec;
}

// The endpoints on this screen that answer with a KEYED envelope rather than
// the paged `{data, page}` one every fake falls back to. An unrouted keyed
// endpoint does not read as an empty card: the consumer indexes the key it was
// promised, gets undefined, and throws mid-render — which takes the whole entry
// down and surfaces as its OTHER cards being absent, nowhere near the cause.
//
// Shared because this screen has several fetch fakes — `settingsBackend` here,
// `mergedEntryBackend` parameterizing the seat, `settingsNavBackend` the grant
// map — and a keyed endpoint added to one of them alone leaves the others
// failing exactly that way. Every fake routes through here for that reason;
// counting them in this comment is how the sentence goes stale, so it does not.
export function keyedEnvelope(url: string) {
  // `providers` is required in the contract, so a card is right to index it
  // directly; an empty list is the honest answer for an installation that has
  // bound no cloud provider.
  if (url.includes("/ai/provider-keys")) {
    return jsonResponse({ providers: [] });
  }
  // `rungs` and `window_hours` are both required, and the health card indexes
  // them directly. No rung is the honest answer for an installation that called
  // no model in the window — which is what a test fixture is.
  if (url.includes("/ai/health")) {
    return jsonResponse({ window_hours: 1, rungs: [] });
  }
  // `budget` is required too, and the agent at the foot of the settings rail
  // indexes it for the currency its spend figure is in. A month with no call is
  // the honest answer for a fixture, and the figure it draws is then absent
  // rather than a confident zero (app/agentrail.tsx).
  if (url.includes("/ai/usage")) {
    return jsonResponse({
      days: [],
      budget: { monthly_tokens: 0, spent_tokens: 0, band: "normal" },
    });
  }
  return null;
}

// Routed by URL so every card on the screen gets an honest per-endpoint
// answer; the cards not under test render their empty states.
export function settingsBackend() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      const me = meFixture({
        roles: ["admin", "field_marketing"],
        allow: ADMIN_GRANTS,
      });
      return jsonResponse({
        ...me,
        user: { ...me.user, email: "ada@acme.test" },
      });
    }
    // When this reader is bookable. The Account tab carries the card, and a
    // page whose card cannot load its own answer renders nothing around it —
    // which is what every case on this tab would then be measuring.
    if (url.includes("/me/working-hours")) {
      return jsonResponse({
        chosen: false,
        working_hours: {
          start_time: "09:00",
          end_time: "17:00",
          days: [1, 2, 3, 4, 5],
          timezone: "Europe/Berlin",
        },
      });
    }
    if (url.includes("/passports")) {
      return jsonResponse({
        data: [
          {
            id: "pp-1",
            label: "Scout",
            scopes: ["read"],
            created_at: "2026-07-01T08:00:00Z",
            expires_at: null,
            revoked_at: null,
          },
        ],
        page: { next_cursor: null, has_more: false },
      });
    }
    const keyed = keyedEnvelope(url);
    if (keyed) {
      return keyed;
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

// One audit-log entry carrying a full attribution trail (before/after diff,
// agent passport, on-behalf-of human, authorization rule, and evidence) so
// the expand panel has every field to render honestly.
export const auditEntry = {
  id: "al-1",
  actor_type: "agent",
  actor_id: "agent:sdr",
  passport_id: "pp-9",
  on_behalf_of: "u-1",
  action: "update",
  entity_type: "person",
  entity_id: "p-1",
  before: { stage: "new" },
  after: { stage: "qualified" },
  authorization_rule: "role:admin",
  evidence: { snippet: "Reply confirmed budget", source: "email:msg-1" },
  occurred_at: "2026-07-10T09:00:00Z",
};

// A background system with nothing queued and nothing failed — GET
// /admin/job-health's honest quiet answer.
export const IDLE_JOB_HEALTH = {
  generated_at: "2026-08-13T09:30:00Z",
  kinds: [],
  recent_failures: [],
};

// The nav, driven by exactly the two things the catalog composes: the grant map
// /me carries, and the company-context rollout flag beside it. Every other
// endpoint answers empty, so a failure here can only be about visibility.
export function settingsNavBackend(opts: {
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
    // The rail carries the agent at its foot, and the agent reads endpoints that
    // answer with a KEYED envelope rather than the paged one below — an unrouted
    // one throws mid-render, which surfaces here as the nav being empty.
    const keyed = keyedEnvelope(url);
    if (keyed) {
      return keyed;
    }
    return jsonResponse({
      data: [],
      page: { next_cursor: null, has_more: false },
    });
  });
}

/**
 * Every page this render OFFERS, rail and home together, de-duplicated and in
 * catalog order.
 *
 * The rail alone stopped being the list of pages a reader may open: it carries
 * `changes` now — the pages they can act on — and a page they may read and
 * cannot change is deliberately absent from it while still answering its
 * address, appearing in search, and being listed on the settings home under a
 * heading that says which it is.
 *
 * So a case whose claim is "this grant opens that page" reads BOTH surfaces.
 * Reading only the rail would turn every such case into a claim about
 * prominence, which is a different question and one most of them never meant
 * to ask.
 */

export function offeredPages(): string[] {
  const seen = new Set(
    screen
      .getAllByRole("link")
      .map((link) => link.getAttribute("href") ?? "")
      .filter((href) => href.startsWith("#/settings/"))
      .map((href) => href.slice("#/settings/".length)),
  );
  return [
    translate("en", "settings.home"),
    ...SETTINGS_PAGES.filter((page) => seen.has(page.id)).map((page) =>
      labelOf(page.id),
    ),
  ];
}

// The expected labels, DERIVED from the catalog rather than restated beside it.
// A list of labels beside a list of pages is a second source of truth, and
// nothing updates it: the restated lists this replaces omitted `license` — a
// fully wired entry with a predicate, content, labels in two locales and a deep
// link from the sidebar's seat meter — so every assertion in this file,
// including the ones claiming to walk the whole level, passed while checking one
// entry fewer than existed.
export const labelOf = (id: SettingsPageId) =>
  translate("en", `settings.tab.${id}`);

// The pages a caller reaches by naming a set of page ids, in CATALOG order —
// so a case states which pages a grant opens and the order comes from the one
// table that decides it, never from the order the case happened to list them.
// Settings home leads every list, for every reader. It is not a page and no
// grant reaches it — it is the address with no page segment — so it belongs in
// the shared expectation rather than in each case that would otherwise have to
// remember it.
export const pagesNamed = (...ids: readonly SettingsPageId[]) => [
  translate("en", "settings.home"),
  ...SETTINGS_PAGES.filter((page) => ids.includes(page.id)).map((page) =>
    labelOf(page.id),
  ),
];

// What every reader gets: the personal pages, which carry no requirement at
// all, and the two People pages that ask for none.
//
// `agents` and `connections` are personal because what they carry is the
// PERSON's: gating `agents` would regress passport minting for every seat that
// is not an admin, and a mailbox and a LinkedIn network nobody else can see are
// not the installation's configuration.
// The pages that ask for nothing: a reader's own five, and nothing else.
//
// `members` and `teams` used to sit here. The roster endpoint still answers any
// authenticated caller — the share and assignee pickers depend on it — but a
// directory is not an administration page, so the settings ENTRY follows the
// `user_admin` and `team_admin` verbs while the safe roster stays open
// underneath it.
export const UNGATED_IDS = SETTINGS_PAGES.filter(
  (page) => page.group === "me",
).map((page) => page.id) as readonly SettingsPageId[];

// The floor plus the pages a grant opens, in CATALOG order. Concatenating the
// two lists instead would assert an order the catalog does not have: `company`
// and `authentication` are declared BEFORE `members`, so a page's position
// comes from the table rather than from which half of the fixture named it.
export const floorPlus = (...ids: readonly SettingsPageId[]) =>
  pagesNamed(...UNGATED_IDS, ...ids);

/**
 * Assert the nav settles to exactly these rows, once `/me` has actually
 * answered.
 *
 * A bare `waitFor(() => expect(offeredPages()).toEqual(floorPlus()))` is VACUOUS
 * for a case about an absence. Every capability predicate reads false while the
 * snapshot is in flight, so the loading nav renders precisely the ungated floor
 * — the same rows a resolved snapshot granting nothing renders. `waitFor`
 * succeeds on its first tick, before the page under test could have appeared,
 * and the case passes whatever the requirement says. Granting the very page a
 * case says is withheld still passed, which is how this was found.
 *
 * So every such case needs a POSITIVE CONTROL: one page that only a resolved
 * snapshot can draw. The caller grants a witness object alongside whatever it
 * is really testing, this waits for that witness row to appear — which cannot
 * happen until `/me` has answered AND rendered — and only then asserts the
 * whole list. `pipeline` is the witness: it opens exactly one page, on a plain
 * read, and is unrelated to every requirement these cases move.
 */

export async function expectNavSettlesTo(expected: readonly string[]) {
  await expectSnapshotResolved();
  await expect.poll(() => offeredPages()).toEqual(expected);
}

/**
 * Wait until `/me` has actually answered AND rendered.
 *
 * The witness is the settings home's seat row, which draws only when
 * `authorization` is present on the snapshot — it is absent while the request
 * is in flight, and the panel renders nothing rather than guessing a seat.
 *
 * A page row cannot do this job any more. The old witness was Pipelines, which
 * worked only for a fixture that granted `pipeline:read`; a case about a lone
 * `seat_usage` reader has no such row to wait for, and waiting for one that
 * never comes fails a case whose subject is somewhere else entirely. The seat
 * row is on every resolved snapshot regardless of grants, which is what a
 * positive control has to be.
 */

export async function expectSnapshotResolved() {
  await screen.findByText(translate("en", "settings.home.seat.full"));
}

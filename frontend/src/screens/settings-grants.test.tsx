/** @vitest-environment jsdom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { GrantSpec } from "../app/mefixture";
import {
  expectNavSettlesTo,
  floorPlus,
  labelOf,
  offeredPages,
  readOn,
  renderHome,
  settingsNavBackend,
} from "./settings.testkit";

// ONE page, ONE grant: which requirement opens each settings entry.
//
// Every case here names the READ grant the page's own cards ask for and asserts
// the level with that grant against the level without it. The floor those
// additions sit on, and the whole-list claims, are `settings-nav.test.tsx`.

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

describe("the grant that opens one settings page", () => {
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
    renderHome();
    // `privacy` rides the floor here: meFixture grants `contact:read`, which is
    // one of that page's union terms — the consent registry's own server gate.
    await waitFor(() =>
      expect(offeredPages()).toEqual(floorPlus("authentication")),
    );
  });

  it("does NOT open it on the read every seeded role holds", async () => {
    // The disclosure the backend split closed, asserted from the client side.
    // `installation_settings:read` is held by every role — a rep reads it for
    // the base currency — so a page opening on it would put the installation's
    // sign-in policy in front of the whole workspace.
    //
    // It no longer opens Company profile either, and that is the same fix one
    // page further on: the installation's own facts are admin and ops work, and
    // the read was only ever there so a rep could resolve the base currency.
    // The page asks the UPDATE now, which the second half of this case proves
    // still lands.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["rep"],
        allow: { ...readOn("installation_settings"), pipeline: ["read"] },
      }),
    );
    const { unmount } = renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
    unmount();

    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        allow: {
          ...readOn("installation_settings"),
          installation_settings: ["read", "update"],
        },
      }),
    );
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(floorPlus("company")));
  });

  it("opens Seats & license for a lone license read", async () => {
    // `LicenseCard` calls `/installation/license` and nothing else, so this is
    // the grant that actually reaches content. The role defaults give `license`
    // read to admin and ops and to nobody else (identity's policy defaults), so
    // it is still a grant an edited role can hold rather than a role name.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("license") }),
    );
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(floorPlus("seats")));
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
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(floorPlus("seats")));
    cleanup();

    // The other half, or the assertion above would pass against a page that
    // opened for everybody: a seat holding neither read does not reach it.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        allow: { ...readOn("contact"), pipeline: ["read"] },
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
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
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(floorPlus("capture")));
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
    renderHome();
    await waitFor(() =>
      expect(offeredPages()).toEqual(floorPlus("system-health")),
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
    renderHome();
    await waitFor(() =>
      expect(offeredPages()).toEqual(floorPlus("system-health")),
    );
    cleanup();

    // And a seat holding neither term still does not reach it, or the case
    // above would pass against a page that opened for everybody.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        allow: { ...readOn("contact"), pipeline: ["read"] },
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
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
        // `contact:read` is the floor `readOn` holds steady for every other case
        // here — without it this stops being a case about Extensions and also
        // becomes one about losing Privacy.
        allow: {
          contact: ["read"],
          extension_access: ["read"],
          role_admin: ["read"],
        },
      }),
    );
    renderHome();
    await waitFor(() =>
      expect(offeredPages()).toEqual(floorPlus("extensions")),
    );
    cleanup();

    // The inventory read ALONE is not the page. The card makes a second request
    // for every role's grants, which asks `role_admin:read` — so on this grant
    // the page opened and the card 403'd inside it, which is an unreadable page
    // rather than a narrower one.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["ops"],
        allow: { ...readOn("extension_access"), pipeline: ["read"] },
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
    cleanup();

    // And the read is load-bearing rather than decorative: an ADMIN who lost it
    // does not reach the page, which is what says the entry stopped asking for
    // the role.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: {
          contact: ["read"],
          system_reset: ["delete"],
          // The witness: `pipeline` opens Pipelines and Stage automation and
          // nothing else, so
          // waiting for that row proves /me resolved before this asserts
          // what is NOT there.
          pipeline: ["read"],
        },
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
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
        allow: { ...readOn("system_reset"), pipeline: ["read"] },
        dataResetAvailable: true,
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
    cleanup();

    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: { contact: ["read"], system_reset: ["delete"] },
        dataResetAvailable: true,
      }),
    );
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(floorPlus("reset")));
    cleanup();

    // And the deployment's own consent, which is not a permission: the same
    // holder on an installation that never opted in has no such destination.
    // The compiled default is false everywhere, so this is the ordinary case
    // rather than the exotic one.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: {
          contact: ["read"],
          system_reset: ["delete"],
          // The witness: `pipeline` opens Pipelines and Stage automation and
          // nothing else, so
          // waiting for that row proves /me resolved before this asserts
          // what is NOT there.
          pipeline: ["read"],
        },
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
  });

  it("opens Company profile for a lone fx_rate read, and no other page with it", async () => {
    // The currency table joined the base currency it converts to, so fx_rate is
    // one of the three terms Company profile's requirement unions — this read
    // alone has to open it, and the neighbouring pages have to stay shut.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("fx_rate") }),
    );
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(floorPlus("company")));
  });

  it("opens AI usage for a lone ai_model_rate read", async () => {
    // Model prices joined the usage figures they price, and either term of that
    // page's requirement opens it on its own — so the union has to be read as a
    // union and not as one object with a decorative second term.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({ roles: ["ops"], allow: readOn("ai_model_rate") }),
    );
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(floorPlus("usage")));
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
    renderHome();
    await waitFor(() =>
      expect(offeredPages()).toEqual(
        floorPlus("models", "usage", "model-calls"),
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
        allow: { contact: ["read"], automation: ["update"] },
      }),
    );
    renderHome();
    // `automations` is OPEN here, and naming it is the point: this fixture holds
    // `automation:update`, which is exactly what that page asks now. The claim
    // under test is about the three AI diagnostics pages staying shut, and
    // asserting the whole row keeps the two facts from being confused.
    await waitFor(() =>
      expect(offeredPages()).toEqual(floorPlus("automations")),
    );
  });

  it.each(["retention_policy", "privacy_request"] as const)(
    "opens Privacy for a lone %s read",
    async (object) => {
      // Two terms, each opening the page alone — the retention ladder and the
      // DSR queue. A union read as one object with a decorative second term
      // would pass any fixture granting both.
      //
      // `consent_config` is deliberately NOT a term. It is what the purposes
      // card ADMINISTERS, but it buys no read: only the writes moved to it. On
      // that grant alone the page opened with every card inside it withheld.
      const allow: GrantSpec = {};
      allow[object] = ["read"];
      vi.stubGlobal("fetch", settingsNavBackend({ roles: ["rep"], allow }));
      renderHome();
      await waitFor(() => expect(offeredPages()).toEqual(floorPlus("privacy")));
    },
  );

  // `contact` was the third term and is now the case that must NOT open it.
  // Every seeded role holds this read — it is the gate the registry endpoint
  // applies (consent/store.go's ListPurposes) and the Contact 360 needs it — so
  // a page opening on it was the whole workspace's governance page. The card
  // still reads through `contact`; a card narrower than its page withholds
  // itself, which is the safe direction.
  it("does not open Privacy for the contact read every seeded role holds", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["rep"],
        allow: { contact: ["read"], pipeline: ["read"] },
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
  });

  // The term that was dropped, asserted as an absence so nobody adds it back
  // without meeting the card that would have to read on it.
  it("does not open Privacy for a lone consent_config read", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["custom"],
        allow: { consent_config: ["read", "create"], pipeline: ["read"] },
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
  });

  it("opens Audit log without opening Privacy, for an admin holding the trail read", async () => {
    // The trail was split off the privacy page because it answers to a
    // DIFFERENT grant: a reader could hold `audit_log:read` and be refused the
    // page carrying it. Granting the trail read and NOT `contact:read` is what
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
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(floorPlus("audit")));
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
    renderHome();
    await waitFor(() => expect(offeredPages()).toEqual(floorPlus("audit")));
    cleanup();

    // And a management seat WITHOUT the read still does not reach it, or the
    // assertion above would pass against a page that opened for everybody.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["management"],
        // The witness: this case asserts audit is ABSENT, and a grantless
        // fixture would assert it against the loading render.
        allow: { pipeline: ["read"] },
      }),
    );
    renderHome();
    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
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

  it("shows Company profile to an admin holding the company read once the company rollout flag is on", async () => {
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: { ...readOn("company"), company: ["read", "update"] },
        companyReadEnabled: true,
      }),
    );
    renderHome();
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
    // The company WRITE is the only term of Company profile's requirement
    // this fixture grants, which is what leaves the flag decisive. Granting the
    // read alone would hide the page whatever the flag said, and the case would
    // pass while proving nothing about the flag.
    vi.stubGlobal(
      "fetch",
      settingsNavBackend({
        roles: ["admin"],
        allow: {
          ...readOn("company"),
          company: ["read", "update"],
          // The witness, so the absence below is asserted against a
          // RESOLVED snapshot rather than the loading render.
          pipeline: ["read"],
        },
        companyReadEnabled: false,
      }),
    );
    renderHome();

    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
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
        // The write, for the same reason the case above takes it: on the read
        // alone the page is shut anyway and the absent-availability arm this
        // case exists to hold would never be reached.
        allow: {
          ...readOn("company"),
          company: ["read", "update"],
          // The witness, so the absence below is asserted against a
          // RESOLVED snapshot rather than the loading render.
          pipeline: ["read"],
        },
        omitAvailability: true,
      }),
    );
    renderHome();

    await expectNavSettlesTo(floorPlus("pipelines", "stageautomation"));
    expect(screen.queryByRole("link", { name: labelOf("company") })).toBeNull();
  });
});

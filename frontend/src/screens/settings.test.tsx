/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { isValidElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AccountMenu } from "../app/account";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { pickOption } from "../design-system/select-testing";
import { LOCALES, localeNameKey, translate } from "../i18n";
import { AuditLogCard, SettingsScreen, tabContent } from "./settings";
import {
  auditEntry,
  IDLE_JOB_HEALTH,
  jsonResponse,
  keyedEnvelope,
  render,
  renderSettings,
  settingsBackend,
} from "./settings.testkit";
import { SETTINGS_PAGES, type SettingsPageId } from "./settingscatalog";
import { settingsHref } from "./settingsrouting";

// What a principal is SERVED once an entry opens: the identity and locale
// surfaces on Account, the rows that read as WITHHELD rather than absent, and
// the merged entries carrying the surfaces their parts brought with them.
//
// The rest of this screen's coverage sits beside it, one subject per file —
// `settings-nav` (which entries a principal is offered at all),
// `settings-agents`, `settings-integrations`, `settings-pipelines`,
// `settings-maintenance` and `settings-audit`. The shared fixtures are in
// `settings.testkit.tsx`.

// The settings identity + passport surfaces through the RBAC primitives:
// roles render as localized RoleBadges (a workspace-defined key stays raw),
// and the passport list's token slot reads as WITHHELD (FieldGuard mask) —
// the wire schema carries no token, and the row says so instead of omitting
// the field as if none existed.

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  vi.stubGlobal("fetch", settingsBackend());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

// A principal holding the model-price grants and NOTHING else: enough to reach
// the two AI diagnostics pages and author the price table, and not enough for
// the cards whose endpoints ask for the automation grant. Shared by the two
// cases below, which are one claim asserted on the two addresses it now spans.
function aiRateReaderBackend() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({
          roles: ["ops"],
          allow: {
            // What opens both pages. The price grant authors the table on one
            // of them but reaches neither on its own, so a fixture without this
            // would be testing the fallback to Account.
            //
            // NOT `ai_diagnostics:read`, which is what the cards check — that
            // is the whole fixture: reach the page, be refused the card. The
            // cards asked `automation:update` before the runtime's spend got an
            // object of its own.
            //
            // Usage opens on `ai_model_rate` alone, so this reader gets there
            // and finds the spend withheld. Model calls asks for the same grant
            // its card does, so there is no such state on that page any more:
            // a reader who reaches it can read it. The case below therefore
            // asserts the page is UNREACHABLE rather than reachable-and-empty.
            ai_model_rate: ["read", "update"],
          },
        }),
      );
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

describe("SettingsScreen RBAC surfaces", () => {
  it("renders the session roles as localized badges on the default Account tab; a custom key stays its raw self", async () => {
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());
    expect(screen.getByText("Admin")).toBeTruthy();
    expect(screen.getByText("field_marketing")).toBeTruthy();
    // the seeded key never leaks raw once a label exists
    expect(screen.queryByText("admin")).toBeNull();
  });

  // Appearance is chosen from Settings AND from the account menu. The menu keeps
  // its shortcut — it is the setting a reader changes most often and from
  // wherever they are standing — and this is where somebody who came looking for
  // it in Settings finds it.
  it("offers the appearance choice on the Account tab", async () => {
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    expect(screen.getByRole("heading", { name: "Your account" })).toBeTruthy();
    expect(screen.getByRole("combobox", { name: "Language" })).toBeTruthy();
    // Opened, because a Margince `Select` renders its options in a listbox on
    // click rather than as children of the control.
    const user = userEvent.setup();
    await user.click(screen.getByRole("combobox", { name: "Appearance" }));
    expect(
      within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["Light", "Dark", "System"]);
  });

  // ONE state, two faces, and BOTH of them are on screen for this.
  //
  // Asserting localStorage alone would pass for a row that writes the key and
  // never notifies anybody — the menu would sit there stale, which is the exact
  // failure "one state" is supposed to rule out. So the real account menu is
  // mounted beside the settings screen, and the assertion is that it followed.
  it("shares its answer with the account menu", async () => {
    const user = userEvent.setup();
    render(
      <>
        <SettingsScreen route={settingsHref("account")} />
        <AccountMenu />
      </>,
    );
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Appearance" }),
      "Dark",
    );

    // The menu's own radio, read through the menu rather than through the store
    // underneath it. `setThemeChoice` publishes to `useSyncExternalStore`
    // subscribers, and this is one of them.
    await user.click(screen.getByRole("button", { name: "Account" }));
    await user.click(screen.getByRole("menuitem", { name: "Theme" }));
    expect(
      screen
        .getByRole("menuitemradio", { name: "Dark" })
        .getAttribute("aria-checked"),
    ).toBe("true");
  });

  // Identity, credential, sign-off and language are ONE card, not four: a
  // reader auditing their own account reads one title and finds three answers
  // at one x. The claim is about the three ROWS being there, in the one card —
  // a page that grew a second panel back would still pass a query for any one
  // of them on its own.
  //
  // The tab holds a second card beside it — which kinds of proposal answer
  // themselves — and that is a different subject rather than a fragment of this
  // one, so the count below asks how many headings this card carries rather
  // than how many the tab does.
  it("carries the identity, the password, the signature and the language in ONE card", async () => {
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    const card = screen
      .getByRole("heading", { name: "Your account" })
      .closest("section");
    if (!(card instanceof HTMLElement)) {
      throw new Error("the account card is not a section");
    }
    // The identity block, and the three verbs/answers that belong to it.
    expect(within(card).getByText("ada@acme.test")).toBeTruthy();
    expect(
      within(card).getByRole("button", { name: "Change password" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("button", { name: "Edit signature" }),
    ).toBeTruthy();
    expect(
      within(card).getByRole("combobox", { name: "Language" }),
    ).toBeTruthy();
    // And the four are not four cards: this one has a single title over all of
    // them, which is what fragmenting it again would break.
    expect(within(card).getAllByRole("heading", { level: 2 })).toHaveLength(1);
  });

  // A member correcting the name their colleagues see them by. Until this row
  // existed there was no way to: `display_name` was written by the invite and
  // by nothing else, so a name typed wrong stayed wrong on every record that
  // contact touched.
  it("saves a corrected name to the caller's own seat", async () => {
    const user = userEvent.setup();
    const sent: { path: string; body: unknown }[] = [];
    const backend = settingsBackend();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : undefined;
        const url = String(request ? request.url : input);
        const method = request?.method ?? init?.method ?? "GET";
        if (url.includes("/me/display-name") && method === "PUT") {
          // openapi-fetch passes a Request, so the body is read off it rather
          // than off `init` — `init.method` is undefined for this client.
          const raw = request ? await request.text() : String(init?.body ?? "");
          sent.push({ path: "/me/display-name", body: JSON.parse(raw) });
          return new Response(JSON.stringify({}), {
            status: 200,
            headers: { "content-type": "application/json" },
          });
        }
        return backend(input);
      }),
    );

    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    const field = screen.getByRole("textbox", { name: "Your name" });
    await user.clear(field);
    await user.type(field, "  Ada Lovelace  ");
    await user.click(screen.getByRole("button", { name: "Save" }));

    // Trimmed on the way out, so a trailing space is not a change the server
    // has to refuse. Asserted through the WIRE, because a row that renders the
    // right value and sends the wrong field reads as "the feature does nothing".
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual({
      path: "/me/display-name",
      body: { display_name: "Ada Lovelace" },
    });
  });

  // The control for the case above: Save is withheld until the name actually
  // moves. Without it, a row that always enabled Save would pass the case above
  // and quietly write on every render.
  it("offers no save until the name changes", async () => {
    const user = userEvent.setup();
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    const save = screen.getByRole("button", { name: "Save" });
    expect(save).toHaveProperty("disabled", true);

    // A name of only whitespace is not a name, so it does not enable it either.
    const field = screen.getByRole("textbox", { name: "Your name" });
    await user.clear(field);
    await user.type(field, "   ");
    expect(screen.getByRole("button", { name: "Save" })).toHaveProperty(
      "disabled",
      true,
    );

    await user.type(field, "Ada");
    expect(screen.getByRole("button", { name: "Save" })).toHaveProperty(
      "disabled",
      false,
    );
  });

  it("switches the language from the Account tab, through the design-system select", async () => {
    const user = userEvent.setup();
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Language" }),
      "Deutsch",
    );
    // The choice reaches the chrome around the control, not just the control's
    // own face — which is the whole point of changing a language here.
    expect(screen.getByRole("combobox", { name: "Sprache" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Ihr Konto" })).toBeTruthy();
  });

  // WCAG 2.2 AA 3.1.2. This is the one picker in the product where every option
  // is deliberately in a language other than the page's, so a screen reader has
  // to be told which. Derived from LOCALES rather than listed, the way the login
  // footer's own coverage is: a hardcoded pair keeps passing after a fourth
  // language is added without one.
  it("declares each language name's own language, on the options and on the face", async () => {
    const user = userEvent.setup();
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());
    const trigger = screen.getByRole("combobox", { name: "Language" });

    expect(trigger.querySelector(".select-face")?.getAttribute("lang")).toBe(
      "en",
    );
    await user.click(trigger);

    for (const locale of LOCALES) {
      const name = translate("en", localeNameKey(locale));
      const option = screen.getByRole("option", { name });
      expect(
        option.querySelector(".select-option-label")?.getAttribute("lang"),
      ).toBe(locale);
    }
  });

  it("the passport row's token reads as withheld — masked, never re-disclosed — on the Agents tab", async () => {
    render(<SettingsScreen route={settingsHref("agents")} />);
    await waitFor(() => expect(screen.getByText("Scout")).toBeTruthy());
    expect(screen.getByRole("img", { name: "Masked value" })).toBeTruthy();
    expect(screen.queryByText(/mgp_/)).toBeNull();
  });

  // The spend cards follow `automation:update` rather than any AI-named object,
  // so the principal here holds the model-price grants that open AI usage and
  // nothing else: the read reaches the page, the write authors the price table on
  // it, and the card whose endpoint would 403 keeps its place and says why.
  //
  // The five-tab strip that used to carry these is gone — each card is its own
  // address now — so the two withheld readings are asserted on the two pages
  // that hold them rather than one page's tabs.
  it("withholds the AI spend from a principal without the diagnostics read", async () => {
    vi.stubGlobal("fetch", aiRateReaderBackend());
    render(<SettingsScreen route={settingsHref("usage")} />);

    // The model prices this grant authors are on screen...
    await waitFor(() =>
      expect(screen.getByText("AI model costs")).toBeTruthy(),
    );
    // ...and the card whose endpoint requires `ai_diagnostics:read` KEEPS its
    // place and says it is withheld. Absent, it would claim the installation had
    // spent nothing — a statement about the data, where the truth is only about
    // who may read it. No request is made for it, so a rep never hits a 403
    // error box (GET /ai/usage).
    expect(await screen.findByText("AI usage & budget")).toBeTruthy();
    expect(
      await screen.findByText(
        /only an operator can see what the AI runtime spent/i,
      ),
    ).toBeTruthy();
  });

  it("does not offer the AI call trace to that same principal at all", async () => {
    // The trace split onto an address of its own, and the page asks for exactly
    // what its card asks for — so unlike Usage beside it, there is no state
    // where a reader reaches this page and finds the card withheld. Reaching it
    // and reading it are one grant.
    //
    // So the honest assertion is absence rather than a withheld card: the
    // address reaches the access BOUNDARY, and nothing about the trace appears.
    // If the page and the card ever diverge again this fails, which is the
    // right alarm.
    vi.stubGlobal("fetch", aiRateReaderBackend());
    render(<SettingsScreen route={settingsHref("model-calls")} />);
    // Waited on the BOUNDARY's own words rather than on an absence: the trace
    // is missing before /me resolves too, so an absence alone would pass
    // against a page that goes on to render it.
    //
    // It used to wait on Account's content, because a denied address silently
    // landed there. It does not any more — the reader is told, and the address
    // they were given is left in the bar for them to quote.
    // Waited on content only a RESOLVED /me draws — the AI rate card this
    // principal DOES hold — before asserting the denial beside it. The boundary
    // is also what renders while the snapshot is in flight, so asserting it
    // alone would pass against a page that goes on to render the trace.
    await waitFor(() =>
      expect(
        screen.getByText(/this settings page is not yours to open/i),
      ).toBeTruthy(),
    );
    // The chrome agrees: no page is current, where the fallback used to mark
    // Account.
    expect(screen.queryByText("AI call trace")).toBeNull();
  });

  // The way off the boundary leads to the settings home, which is the address
  // the sidebar's first row leads to — so it wears that row's word rather than
  // a second name for one place. Read from the catalog, so renaming the row
  // renames this link and cannot leave the two saying different things.
  it("names the way off the boundary with the sidebar's own word for it", async () => {
    vi.stubGlobal("fetch", aiRateReaderBackend());
    render(<SettingsScreen route={settingsHref("model-calls")} />);
    const back = await screen.findByRole("link", {
      name: translate("en", "settings.home"),
    });
    expect(back.getAttribute("href")).toBe("#/settings");
  });
});

// A reindex marker with work waiting, so the search-index card has a state to
// report rather than a shapeless payload to guess at.
const REINDEX_STATUS = {
  configured_identity: "anthropic/voyage-3@1024",
  populated_identity: "anthropic/voyage-2@1024",
  status: "idle",
  updated_at: "2026-07-21T12:00:00Z",
  reindex_needed: true,
  entities_pending: 42,
  per_workspace: [{ entities_pending: 42 }],
};

// Every read the restructured pages make, answered honestly in one place: the
// passports Agents lists, the consent registry on Privacy, the trail on the
// audit page beside it, and the two operational reports on System health.
function mergedEntryBackend(opts: {
  roles: string[];
  seat?: "full" | "read";
  allow?: GrantSpec;
  dataResetAvailable?: boolean;
}) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      const me = meFixture({
        roles: opts.roles,
        seat: opts.seat ?? "full",
        allow: opts.allow ?? {},
      });
      return jsonResponse({
        ...me,
        workspace_name: "Acme Inc",
        data_reset_available: opts.dataResetAvailable ?? false,
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
    if (url.includes("/audit-log")) {
      return jsonResponse({
        data: [auditEntry],
        page: { next_cursor: null, has_more: false },
      });
    }
    if (url.includes("/admin/job-health")) {
      return jsonResponse(IDLE_JOB_HEALTH);
    }
    if (url.includes("/embeddings/reindex/status")) {
      return jsonResponse(REINDEX_STATUS);
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

// The pages the restructure created or merged into, read as CONTENT: a merged
// page has to carry the surfaces its parts brought, a split one has to answer to
// its own grant, and the personal one has to open for a seat no grant would have
// admitted.
describe("SettingsScreen restructured pages", () => {
  it("opens Agents for a read-only seat, passports and all", async () => {
    vi.stubGlobal(
      "fetch",
      mergedEntryBackend({ roles: ["rep"], seat: "read" }),
    );
    renderSettings("agents");
    await waitFor(() =>
      expect(
        screen
          .getByRole("link", { name: "Agents" })
          .getAttribute("aria-current"),
      ).toBe("page"),
    );
    // A passport is minted by the HUMAN who holds it, so the surface that mints
    // and lists one opens for a seat holding no company grant and no writing
    // licence at all — gating it behind the company group would have meant only
    // admins could mint one.
    expect(
      await screen.findByRole("heading", { name: "Agent passports" }),
    ).toBeTruthy();
    expect(screen.getByText("Scout")).toBeTruthy();
    // And the autonomy table the passports sit under, which came off the
    // company's AI entry with them.
    expect(
      screen.getByRole("heading", { name: "Autonomy tiers" }),
    ).toBeTruthy();
  });

  it("renders the consent registry on Privacy, and the trail on its own page", async () => {
    // The two used to share one page. They are split now because they answer to
    // DIFFERENT grants — the registry to the consent gate, the trail to
    // `audit_log` — so this reader holds both and each page carries its own
    // half. A page still holding the other's card would show up here as a
    // heading on the wrong address.
    vi.stubGlobal(
      "fetch",
      mergedEntryBackend({
        roles: ["admin"],
        allow: {
          contact: ["read"],
          // What opens Privacy now. `contact:read` still reaches the purposes
          // list — that endpoint's gate is unchanged — but it no longer opens
          // the page, because every seeded role holds it.
          //
          // Every verb, because this case reaches the page through its rail row
          // and the rail carries what a reader can act on. A seeded admin holds
          // all four here — RetentionCard offers the delete too — so anything
          // short of them described an account the product does not issue.
          retention_policy: ["read", "create", "update", "delete"],
          audit_log: ["read"],
        },
      }),
    );
    renderSettings("privacy");
    await waitFor(() =>
      expect(
        screen
          .getByRole("link", { name: "Privacy & retention" })
          .getAttribute("aria-current"),
      ).toBe("page"),
    );
    expect(
      await screen.findByRole("heading", { name: "Consent purposes" }),
    ).toBeTruthy();
    expect(screen.queryByRole("heading", { name: "Audit log" })).toBeNull();
    cleanup();

    // ...and the trail, on the address it moved to: its filters — in a
    // disclosure now, closed on arrival — and an entry answering them.
    vi.stubGlobal(
      "fetch",
      mergedEntryBackend({
        roles: ["admin"],
        allow: {
          contact: ["read"],
          audit_log: ["read"],
        },
      }),
    );
    renderSettings("audit");
    expect(
      await screen.findByRole("heading", { name: "Audit log" }),
    ).toBeTruthy();
    expect(screen.getByLabelText("Actor").closest("details")).not.toBeNull();
    expect(await screen.findByText("update")).toBeTruthy();
  });

  // The WRITE opens it, editor included. The page absorbed a route that nothing
  // gated, and for a while it asked the read — which every seeded role holds,
  // so the page that DEFINES automations stood in a rep's rail with nothing on
  // it she could change. Reading what an automation did is answered on the
  // records it touched, not here.
  it("opens Automations for an operator who may change one, editor and all", async () => {
    // The write, not the read: management and manager read `automation` to see
    // what ran, and the page that DEFINES automations is not theirs.
    vi.stubGlobal(
      "fetch",
      mergedEntryBackend({
        roles: ["ops"],
        allow: { automation: ["read", "create", "update"] },
      }),
    );
    renderSettings("automations");
    await waitFor(() =>
      expect(
        screen
          .getByRole("link", { name: "Automations" })
          .getAttribute("aria-current"),
      ).toBe("page"),
    );
    // The surface they came for, which is now the whole page rather than one tab
    // of five sharing an address.
    expect(
      await screen.findByRole("heading", { name: "Automations" }),
    ).toBeTruthy();
  });

  // The trail answers to `audit_log:read`, which is what
  // `GET /v1/audit-log` asks for and what `AuditLogCard` asks for now — it
  // asked whether the reader WAS an admin before. The page opens on the same
  // grant, so the two agree: a holder reaches the page AND the trail on it.
  it("serves the audit trail to an ops seat holding the trail read", async () => {
    const backend = mergedEntryBackend({
      roles: ["ops"],
      allow: { audit_log: ["read"] },
    });
    vi.stubGlobal("fetch", backend);
    renderSettings("audit");
    expect(
      await screen.findByRole("heading", { name: "Audit log" }),
    ).toBeTruthy();
    // Reached the wire rather than merely rendering a shell: the card's fetch is
    // `enabled` on the same grant, so an entry it served is what says the read
    // was honoured end to end.
    expect(await screen.findByText("update")).toBeTruthy();
  });

  // The other half, and the reason opening the page is not the whole answer: a
  // reader who reaches this page some other way still may not see the trail, so
  // the card carries its own gate and says so rather than rendering empty.
  it("withholds the trail from a reader without the read, and asks the server for nothing", async () => {
    const backend = mergedEntryBackend({
      roles: ["ops"],
      allow: { contact: ["read"] },
    });
    vi.stubGlobal("fetch", backend);
    // Rendered directly: without the grant the catalog gives this reader no
    // Audit log row at all, so the card is the only way to reach the branch.
    render(<AuditLogCard />);
    expect(
      await screen.findByText(translate("en", "settings.auditAdminOnly")),
    ).toBeTruthy();
    // WITHHELD rather than absent, and the request is never issued: it could
    // only ever come back 403, and a red failure with a futile Retry is what
    // the withheld body replaces.
    const asked = backend.mock.calls.map((call) =>
      String(call[0] instanceof Request ? call[0].url : call[0]),
    );
    expect(asked.some((url) => url.includes("/audit-log"))).toBe(false);
  });

  // The admin-ONLY surface inside System health, from an ops seat that reaches
  // the page on the reindex read. The page's own gate is not the card's, so this
  // is what proves opening the page did not also hand over what the server
  // spells with RequireAdmin.
  it("renders the reindex on System health for an operator holding only that grant, and withholds job health", async () => {
    vi.stubGlobal(
      "fetch",
      mergedEntryBackend({
        roles: ["ops"],
        allow: { embedding_reindex: ["read", "update"] },
      }),
    );
    renderSettings("system-health");
    await waitFor(() =>
      expect(
        screen
          .getByRole("link", { name: "System health" })
          .getAttribute("aria-current"),
      ).toBe("page"),
    );
    // The verb this grant buys, which used to hide beside the field editor.
    expect(
      await screen.findByRole("heading", { name: "Search index" }),
    ).toBeTruthy();
    // The job report keeps its place and withholds its content: the endpoint is
    // the admin's, and an absent card here would read as "nothing is queued".
    expect(
      screen.getByRole("heading", { name: "Background jobs" }),
    ).toBeTruthy();
    expect(
      screen.getByText(/background-job health needs permission/i),
    ).toBeTruthy();
  });

  // The danger zone moved OFF this page onto an address of its own, and the
  // catalog gates that address on `system_reset:delete` — the one requirement in
  // the table that is not a read. So an operator holding the reindex read no
  // longer reaches it by being on the same page: the nav never offers it, and
  // the address falls back rather than rendering the verb.
  it("keeps the danger zone off System health, and off the nav, for that operator", async () => {
    vi.stubGlobal(
      "fetch",
      mergedEntryBackend({
        roles: ["ops"],
        allow: { embedding_reindex: ["read", "update"] },
        // The installation switch the reset page's second gate asks for, so the
        // missing GRANT is the only thing holding it back below.
        dataResetAvailable: true,
      }),
    );
    renderSettings("system-health");
    expect(
      await screen.findByRole("heading", { name: "Search index" }),
    ).toBeTruthy();
    expect(screen.queryByText(/reset data/i)).toBeNull();
    expect(screen.queryByRole("link", { name: "Reset data" })).toBeNull();
  });
});

// Where a settings card LIVES is a claim about WHOSE setting it is, and the
// catalog says that in two fields: the group names the subject, and `scope` says
// whose state the page changes — `self` for a credential or connection the
// reader personally holds, `workspace` or `installation` for the company's
// posture.
//
// The Google app is one app per installation, supplied by whoever operates it,
// and every rep's mailbox is connected through it. It shipped on `connections`
// — a `self` page — which put installation configuration on a page holding a
// contact's own mailbox and their own LinkedIn network. The server gates the read
// on capture_settings, so a rep saw a refused card rather than the operator's
// client id; the defect was that the page offered them a setting that was never
// theirs.
//
// Walked from the catalog rather than asserted against a hard-coded page id: the
// rule is "not a personal page", so a future move to any other installation page
// passes and a move back onto a personal one fails.
describe("installation-wide cards live off the personal pages", () => {
  // The card names itself; searching the returned tree for that name avoids
  // rendering twenty-nine pages and their API calls.
  function pageRenders(id: SettingsPageId, componentName: string): boolean {
    const seen = new Set<unknown>();
    const walk = (node: ReactNode): boolean => {
      if (!node || typeof node !== "object") {
        return false;
      }
      if (Array.isArray(node)) {
        return node.some(walk);
      }
      if (seen.has(node)) {
        return false;
      }
      seen.add(node);
      // The props generic is what lets `children` be read without asserting a
      // shape nothing checked: isValidElement narrows both halves at once.
      if (!isValidElement<{ children?: ReactNode }>(node)) {
        return false;
      }
      // A function component's `name` is what the catalog renders it under.
      if (typeof node.type === "function" && node.type.name === componentName) {
        return true;
      }
      return walk(node.props.children);
    };
    return walk(tabContent(id));
  }

  it("puts the vendor OAuth apps on an installation page, not beside a contact's own connections", () => {
    const hosts = SETTINGS_PAGES.filter((page) =>
      pageRenders(page.id, "OAuthAppCard"),
    );
    expect(hosts).toHaveLength(1);
    expect(hosts[0]?.scope).toBe("installation");
    // And the group with it: `scope` alone would be satisfied by any page the
    // company owns, while the claim is that this card belongs beside the
    // sign-in policy the same OAuth client now serves.
    expect(hosts[0]?.group).toBe("company");
  });
});

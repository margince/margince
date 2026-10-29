/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { isValidElement, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { AccountMenu } from "../../app/account";
import { pickOption } from "../../design-system/select-testing";
import { LOCALES, localeNameKey, translate } from "../../i18n";
import { SETTINGS_PAGES, type SettingsPageId } from "../../lib/settingscatalog";
import { settingsHref } from "../../lib/settingsrouting";
import { allowance } from "../../testing/ai-admin.testkit";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import {
  auditEntry,
  IDLE_JOB_HEALTH,
  jsonResponse,
  keyedEnvelope,
  render,
  renderSettings,
  settingsBackend,
} from "../../testing/settings.testkit";
import { SIGN_OFF_QUERY } from "../mail/composesignoff";
import { SettingsScreen, tabContent } from "./settings";
import { AuditLogCard } from "./settings-audit";

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

type MeResponse = components["schemas"]["MeResponse"];

// The page's save bar Save, which exists only while a name differs from what
// was saved.
function profileSave(): HTMLElement | null {
  const bar = screen.queryByRole("region", { name: "Unsaved changes" });
  return bar && within(bar).queryByRole("button", { name: "Save" });
}

function pressProfileSave(): HTMLElement {
  const save = profileSave();
  if (save === null) throw new Error("the page offers no Save");
  return save;
}

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
  vi.stubGlobal("fetch", settingsBackend());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

// A principal holding the allowance read and the model-price grants, without
// `ai_diagnostics:read`. Shared by the cases below, one claim on two addresses.
function allowanceReaderBackend() {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    if (url.endsWith("/v1/me")) {
      return jsonResponse(
        meFixture({
          roles: ["ops"],
          allow: {
            // Not `ai_diagnostics:read`, which is what the spend and trace
            // cards check: reach the page, be refused the card.
            //
            // Usage opens on the allowance read, so this reader gets there and
            // finds the spend withheld. Model calls asks for the same grant its
            // card does, so the case below asserts that page is unreachable.
            ai_model_rate: ["read", "update"],
            ai_budget: ["read"],
          },
        }),
      );
    }
    if (url.endsWith("/v1/ai/budget")) {
      return jsonResponse(allowance);
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

    expect(screen.getByRole("combobox", { name: "Language" })).toBeTruthy();
    // A segmented choice, so every option is on screen without opening it.
    expect(
      within(screen.getByRole("group", { name: "Appearance" }))
        .getAllByRole("button")
        .map((option) => option.textContent),
    ).toEqual(["Light", "Dark", "System"]);
  });

  // ONE state, two faces, and BOTH of them are on screen for this.
  //
  // Asserting localStorage alone would pass for a row that writes the key and
  // never notifies anybody — the menu would sit there stale, which is the exact
  // failure "one state" is supposed to rule out. So the real account menu is
  // mounted beside the settings screen, and the assertion is that it followed.
  it("shares its saved answer with the account menu", async () => {
    const user = userEvent.setup();
    render(
      <>
        <SettingsScreen route={settingsHref("account")} />
        <AccountMenu />
      </>,
    );
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    await user.click(
      within(screen.getByRole("group", { name: "Appearance" })).getByRole(
        "button",
        { name: "Dark" },
      ),
    );
    await user.click(pressProfileSave());
    await waitFor(() => expect(profileSave()).toBeNull());

    // The menu's own radio, read through the menu rather than through the store
    // underneath it. `setThemeChoice` publishes to `useSyncExternalStore`
    // subscribers, and this is one of them.
    await user.click(screen.getByRole("button", { name: "Account" }));
    await user.click(screen.getByRole("menuitem", { name: "Theme" }));
    expect(screen.getByRole("radio", { name: "Dark" })).toBeChecked();
  });

  // One card per job: who colleagues see, how you sign in, the language that
  // follows the account, and what only this browser remembers. Each row is
  // asserted inside its own card, because a row that slid into a neighbour would
  // still pass a page-wide query.
  it("puts each account setting in the card for its job", async () => {
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    function card(title: string): HTMLElement {
      const section = screen
        .getByRole("heading", { name: title })
        .closest("section");
      if (!(section instanceof HTMLElement)) {
        throw new Error(`the ${title} card is not a section`);
      }
      return section;
    }
    const profile = card("Profile");
    expect(within(profile).getByText("ada@acme.test")).toBeTruthy();
    expect(
      within(profile).getByRole("textbox", { name: "Display name" }),
    ).toBeTruthy();
    expect(
      within(profile).getByRole("button", { name: /signature/ }),
    ).toBeTruthy();
    const signIn = card("Sign-in");
    expect(
      within(signIn).getByRole("button", { name: "Change password" }),
    ).toBeTruthy();
    expect(
      within(signIn).getByRole("button", { name: "Sign out" }),
    ).toBeTruthy();
    expect(
      within(card("Language")).getByRole("combobox", { name: "Language" }),
    ).toBeTruthy();
    expect(
      within(card("This browser")).getByRole("group", { name: "Appearance" }),
    ).toBeTruthy();
  });

  // THE DRAFT IS DISCARDED ON THE NEXT OPENING, not on the way out.
  //
  // The dialog outlives its own close so it can animate out, and a draft
  // cleared at close snapped the box back to the stored sign-off in front of a
  // reader still watching the dialog leave. Moving the discard to the opening
  // keeps that frame honest and has to keep this promise too: a sign-off
  // half-typed and walked away from is not an edit anybody is coming back to.
  it("reopens the sign-off dialog on a clean form, not on an abandoned draft", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", settingsBackend());
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    await user.click(screen.getByRole("button", { name: /signature/ }));
    const draft = await screen.findByRole("textbox", { name: "Sign-off" });
    await user.type(draft, "half a sign-off nobody meant to keep");
    await user.click(screen.getByRole("button", { name: "Cancel" }));

    await user.click(screen.getByRole("button", { name: /signature/ }));
    const reopened = await screen.findByRole("textbox", {
      name: "Sign-off",
    });
    if (!(reopened instanceof HTMLTextAreaElement)) {
      throw new Error("the sign-off box is not a textarea");
    }
    expect(reopened.value).not.toContain("nobody meant to keep");
  });

  // Every open composer previews the sign-off a send appends, and that answer
  // is read from this signature. Saving it must send them all back to ask.
  it("saving a signature sends every composer's sign-off preview back to ask", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", settingsBackend());
    const { client } = render(
      <SettingsScreen route={settingsHref("account")} />,
    );
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());
    const preview = [SIGN_OFF_QUERY, "user-a", "Body", "Subject"];
    client.setQueryData(preview, {
      text: "Best regards,\nAda",
      kind: "closing",
    });

    await user.click(screen.getByRole("button", { name: "Edit signature" }));
    await user.type(
      await screen.findByRole("textbox", { name: "Sign-off" }),
      "Ada Lovelace",
    );
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", { name: "Save" }),
    );

    await waitFor(() =>
      expect(client.getQueryState(preview)?.isInvalidated).toBe(true),
    );
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

    const field = screen.getByRole("textbox", { name: "Display name" });
    await user.clear(field);
    await user.type(field, "  Ada Lovelace  ");
    await user.click(pressProfileSave());

    // Trimmed on the way out, so a trailing space is not a change the server
    // has to refuse. Asserted through the WIRE, because a row that renders the
    // right value and sends the wrong field reads as "the feature does nothing".
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual({
      path: "/me/display-name",
      body: { display_name: "Ada Lovelace" },
    });
  });

  it("shows a refused name on the save bar and under its field, keeping the draft", async () => {
    const user = userEvent.setup();
    const backend = settingsBackend();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const request = input instanceof Request ? input : undefined;
        const url = String(request ? request.url : input);
        const method = request?.method ?? init?.method ?? "GET";
        if (url.includes("/me/display-name") && method === "PUT") {
          return new Response(
            JSON.stringify({
              status: 422,
              code: "validation_error",
              detail: "Display name is too long.",
            }),
            {
              status: 422,
              headers: { "content-type": "application/problem+json" },
            },
          );
        }
        return backend(input);
      }),
    );

    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());
    const field = screen.getByRole("textbox", { name: "Display name" });
    await user.clear(field);
    await user.type(field, "Ada Lovelace");
    await user.click(pressProfileSave());

    // The bar carries the summary; the refused field names its own refusal.
    const lines = await screen.findAllByText("Display name is too long.");
    expect(lines).toHaveLength(2);
    const named = (field.getAttribute("aria-describedby") ?? "").split(" ");
    expect(lines.some((line) => named.includes(line.id))).toBe(true);
    expect(field).toHaveValue("Ada Lovelace");
    expect(profileSave()).not.toBeNull();
  });

  // Save and Discard exist only while a name differs from what was saved, and
  // Discard puts the saved name back.
  it("offers Save and Discard only while a name changed", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    vi.stubGlobal("fetch", greetingBackend(null, sent));
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    const field = screen.getByRole("textbox", { name: "Display name" });
    if (!(field instanceof HTMLInputElement)) throw new Error("not an input");
    const stored = field.value;
    expect(profileSave()).toBeNull();

    await user.type(field, "x");
    expect(profileSave()).not.toBeNull();
    await user.click(screen.getByRole("button", { name: "Discard" }));
    expect(field).toHaveValue(stored);
    expect(profileSave()).toBeNull();
  });

  // A name of only whitespace is not a name: the field says so, and the bar's
  // Save is held, so nothing is sent.
  it("refuses an empty display name on the field, sending nothing", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    vi.stubGlobal("fetch", greetingBackend(null, sent));
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    const field = screen.getByRole("textbox", { name: "Display name" });
    await user.clear(field);
    await user.type(field, "   ");
    expect(field).toHaveAttribute("aria-invalid", "true");
    await user.click(pressProfileSave());
    expect(sent).toHaveLength(0);
  });

  // The greeting name: what a colleague's drafted greeting opens with. Empty
  // is an answer too — it clears the choice, and is sent as null.
  function greetingBackend(stored: string | null, sent: unknown[]) {
    const backend = settingsBackend();
    return vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : undefined;
      const url = String(request ? request.url : input);
      const method = request?.method ?? init?.method ?? "GET";
      if (url.includes("/me/greeting-name") && method === "PUT") {
        const raw = request ? await request.text() : String(init?.body ?? "");
        const body: unknown = JSON.parse(raw);
        sent.push(body);
        return jsonResponse(body);
      }
      const answer = await backend(input);
      if (!url.endsWith("/v1/me")) {
        return answer;
      }
      const served: MeResponse = await answer.json();
      return jsonResponse({
        ...served,
        user: { ...served.user, greeting_name: stored },
      });
    });
  }

  it("saves a greeting name trimmed, offering no save until it changes", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    vi.stubGlobal("fetch", greetingBackend(null, sent));
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    const field = screen.getByRole("textbox", { name: "Greeting name" });
    expect(profileSave()).toBeNull();
    await user.type(field, "  Ada  ");
    await user.click(pressProfileSave());
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual({ greeting_name: "Ada" });
  });

  // Enter commits a changed name, as a rename does everywhere else.
  it("saves a changed name on Enter", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    vi.stubGlobal("fetch", greetingBackend(null, sent));
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    await user.type(
      screen.getByRole("textbox", { name: "Greeting name" }),
      "Ada{Enter}",
    );
    await waitFor(() => expect(sent).toEqual([{ greeting_name: "Ada" }]));
  });

  it("sends an emptied greeting name as null, which clears it", async () => {
    const user = userEvent.setup();
    const sent: unknown[] = [];
    vi.stubGlobal("fetch", greetingBackend("Sofia", sent));
    render(<SettingsScreen route={settingsHref("account")} />);
    const field = await screen.findByDisplayValue("Sofia");

    await user.clear(field);
    await user.click(pressProfileSave());
    await waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual({ greeting_name: null });
  });

  it("switches the language from the Account tab once the page is saved", async () => {
    const user = userEvent.setup();
    render(<SettingsScreen route={settingsHref("account")} />);
    await waitFor(() => expect(screen.getByText("ada@acme.test")).toBeTruthy());

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Language" }),
      "Deutsch",
    );
    // A pick is a draft: the page stays in its language until Save.
    expect(screen.getByRole("heading", { name: "Sign-in" })).toBeTruthy();
    await user.click(pressProfileSave());
    // The choice reaches the chrome around the control, not just the control's
    // own face — which is the whole point of changing a language here.
    expect(
      await screen.findByRole("combobox", { name: "Sprache" }),
    ).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Anmeldung" })).toBeTruthy();
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

  it("never re-discloses a passport's token on the Agents tab", async () => {
    render(<SettingsScreen route={settingsHref("agents")} />);
    await waitFor(() => expect(screen.getByText("Scout")).toBeTruthy());
    expect(screen.queryByText(/mgp_/)).toBeNull();
  });

  // The allowance opens AI usage for this principal; the spend card beside it
  // asks `ai_diagnostics:read`, so it keeps its place and says why.
  it("withholds the AI spend from a principal without the diagnostics read", async () => {
    vi.stubGlobal("fetch", allowanceReaderBackend());
    render(<SettingsScreen route={settingsHref("usage")} />);

    // The allowance this grant reads is on screen...
    expect(await screen.findByText("Monthly AI allowance")).toBeTruthy();
    // ...and the card whose endpoint requires `ai_diagnostics:read` KEEPS its
    // place and says it is withheld. Absent, it would claim the installation had
    // spent nothing. No request is made for it (GET /ai/usage).
    expect(screen.getByRole("heading", { name: "AI spend" })).toBeTruthy();
    expect(
      await screen.findByText(
        /only an administrator or operations user can see AI spend/i,
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
    vi.stubGlobal("fetch", allowanceReaderBackend());
    render(<SettingsScreen route={settingsHref("model-calls")} />);
    // The boundary's own words come only once /me has resolved, so an absence
    // asserted after them is not the absence of a page still loading.
    await waitFor(() =>
      expect(screen.getByText(/no access to this settings page/i)).toBeTruthy(),
    );
    // A trace card mounted for this reader would say it is withheld.
    expect(screen.queryByText(translate("en", "aicalls.withheld"))).toBeNull();
  });

  // The way off the boundary leads to the settings home, which is the address
  // the sidebar's first row leads to — so it wears that row's word rather than
  // a second name for one place. Read from the catalog, so renaming the row
  // renames this link and cannot leave the two saying different things.
  it("names the way off the boundary with the sidebar's own word for it", async () => {
    vi.stubGlobal("fetch", allowanceReaderBackend());
    render(<SettingsScreen route={settingsHref("model-calls")} />);
    const back = await screen.findByRole("link", {
      name: translate("en", "settings.home"),
    });
    expect(back.getAttribute("href")).toBe("/#/settings");
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
  it("opens AI access for a read-only seat, passports and all", async () => {
    vi.stubGlobal(
      "fetch",
      mergedEntryBackend({ roles: ["rep"], seat: "read" }),
    );
    renderSettings("agents");
    await waitFor(() =>
      expect(
        screen
          .getByRole("link", { name: "AI access" })
          .getAttribute("aria-current"),
      ).toBe("page"),
    );
    // A passport is minted by the HUMAN who holds it, so the surface that mints
    // and lists one opens for a seat holding no company grant and no writing
    // licence at all — gating it behind the company group would have meant only
    // admins could mint one.
    const developers = await screen.findByText("For developers");
    const user = userEvent.setup();
    await user.click(developers);
    expect(
      await screen.findByRole("heading", { name: "Passports" }),
    ).toBeTruthy();
    expect(screen.getByText("Scout")).toBeTruthy();
    // And the autonomy table, one tab over under Agent rules.
    await user.click(screen.getByRole("button", { name: "Agent rules" }));
    expect(
      screen.getByRole("heading", { name: "Autonomy tiers" }),
    ).toBeTruthy();
  });

  it("leads AI access with connecting a tool, and keeps passports behind For developers", async () => {
    vi.stubGlobal("fetch", mergedEntryBackend({ roles: ["rep"] }));
    renderSettings("agents");
    const headings = await screen.findAllByRole("heading", { level: 2 });
    expect(headings[0].textContent).toBe("Connect your AI tool");
    const developers = screen.getByText("For developers").closest("details");
    if (!(developers instanceof HTMLDetailsElement)) {
      throw new Error("For developers is not a disclosure");
    }
    expect(developers.open).toBe(false);
    expect(developers.textContent).toContain("Passports");
    await userEvent.setup().click(screen.getByText("For developers"));
    expect(developers.open).toBe(true);
    // API keys open first: the passports, not the coding agents.
    expect(
      screen
        .getByRole("button", { name: "API keys" })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(screen.queryByRole("heading", { name: "Coding agents" })).toBeNull();
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
          .getByRole("link", { name: "Privacy and retention" })
          .getAttribute("aria-current"),
      ).toBe("page"),
    );
    expect(
      await screen.findByRole("heading", { name: "Consent purposes" }),
    ).toBeTruthy();
    expect(screen.queryByLabelText("Actor")).toBeNull();
    cleanup();

    // ...and the trail, on the address it moved to: its filters, open above
    // the table, and an entry answering them.
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
    const filters = await screen.findByRole("group", { name: "Filters" });
    expect(within(filters).getByLabelText("Actor")).toBeVisible();
    expect(await screen.findByText("Updated")).toBeTruthy();
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
      await screen.findByRole("heading", { name: "Configured automations" }),
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
    expect(await screen.findByRole("group", { name: "Filters" })).toBeTruthy();
    // Reached the wire rather than merely rendering a shell: the card's fetch is
    // `enabled` on the same grant, so an entry it served is what says the read
    // was honoured end to end.
    expect(await screen.findByText("Updated")).toBeTruthy();
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

  // The job health inside System health, from a seat that reaches the page on
  // the reindex read alone. The page's own gate is not the card's, so this
  // proves opening the page did not also hand over what `job_health` guards.
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
    // The job report keeps its place and withholds its content. The endpoint is
    // gated by `job_health`, and an absent card would read as "nothing is queued".
    expect(
      screen.getByRole("heading", { name: "Background jobs" }),
    ).toBeTruthy();
    expect(
      screen.getByText(/background job health covers the whole installation/i),
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
      pageRenders(page.id, "OAuthAppsCard"),
    );
    expect(hosts).toHaveLength(1);
    expect(hosts[0]?.scope).toBe("installation");
    // And the group with it: `scope` alone would be satisfied by any page the
    // company owns, while the claim is that this card belongs beside the
    // sign-in policy the same OAuth client now serves.
    expect(hosts[0]?.group).toBe("company");
  });
});

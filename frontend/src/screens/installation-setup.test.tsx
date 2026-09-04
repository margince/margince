/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { pickOption, pickSuggestion } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "./company.fixtures";
import { InstallationSetup, outstandingStep } from "./installation-setup";

afterEach(() => {
  // Every case starts with the platform question unanswered by this account.
  window.localStorage.clear();
  cleanup();
  vi.unstubAllGlobals();
});

// The wire row, spelled from the generated contract rather than as a loose
// `string`: a fixture naming a step the server cannot report would be a test of
// a shape nothing serves.
type Step = components["schemas"]["InstallationSetupStep"];

// The seeded price sheet a fresh installation is provisioned with, which is
// also the catalogue both model fields offer from. The full wire row, prices
// included — the picker renders them.
function sheetRow(
  provider: string,
  model_id: string,
  lane: "chat" | "embeddings",
  input_per_mtok: string,
  output_per_mtok: string,
) {
  return {
    provider,
    model_id,
    lane,
    input_per_mtok,
    output_per_mtok,
    cache_read_per_mtok: "0",
    cache_write_per_mtok: "0",
    effective_date: "2026-08-01",
  };
}

const SEEDED_SHEET = [
  sheetRow("gemini", "gemini-3.1-flash-lite", "chat", "0.25", "1.50"),
  sheetRow("gemini", "gemini-3.5-flash", "chat", "1.50", "9.00"),
  sheetRow("gemini", "gemini-embedding-001", "embeddings", "0.15", "0"),
  sheetRow(
    "openai_compatible",
    "mistralai/mistral-large-2512",
    "chat",
    "0.50",
    "1.50",
  ),
  // The OpenRouter preset's own two, which the sheet always carries — a preset
  // naming a model SeedModelRates does not price fails a backend gate.
  sheetRow(
    "openai_compatible",
    "mistralai/mistral-small-3.2-24b-instruct",
    "chat",
    "0.10",
    "0.30",
  ),
  sheetRow(
    "openai_compatible",
    "openai/text-embedding-3-small",
    "embeddings",
    "0.02",
    "0",
  ),
];

/**
 * The server's answer, in the order and with the policy the server gives it:
 * the model binding blocks, the Google app is reported and does not.
 *
 * `complete` follows from the blocking steps alone, because that is what the
 * contract says it means — a fixture that waited on the Google app too would be
 * describing an installation this server never reports, and the screen under
 * test would be passing against a shape that does not exist.
 */
function setupReport(
  ai: boolean,
  google: boolean,
): { complete: boolean; steps: Step[] } {
  return {
    complete: ai,
    steps: [
      { step: "ai_models", configured: ai, blocking: true },
      { step: "oauth_app", configured: google, blocking: false },
    ],
  };
}

function mount(
  report: ReturnType<typeof setupReport>,
  /** Paths whose write should fail, so a half-finished run can be exercised. */
  refuse: readonly string[] = [],
) {
  const writes: { url: string; body: unknown }[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    const url = new URL(request.url).pathname;
    if (request.method !== "GET") {
      writes.push({ url, body: JSON.parse(await request.text()) });
      if (refuse.some((p) => url.endsWith(p))) {
        return jsonResponse({ title: "refused" }, 500);
      }
      return new Response(null, { status: 204 });
    }
    if (url.endsWith("/ai-model-rates")) {
      return jsonResponse({ data: SEEDED_SHEET });
    }
    // The platform question's "Not now" is remembered against the account that
    // gave it, so the gate reads the session before it can hide the step.
    if (url.endsWith("/me")) {
      return jsonResponse(meFixture({ allow: {} }));
    }
    if (url.includes("/installation/oauth-apps/")) {
      return jsonResponse({
        source: "none",
        client_id: "",
        redirect_uris: [
          { purpose: "sign_in", url: "https://crm.example/v1/auth/oidc/x" },
        ],
      });
    }
    return jsonResponse(report);
  });
  vi.stubGlobal("fetch", fetchMock);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const Wrap = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={qc}>
      <LocaleProvider>{children}</LocaleProvider>
    </QueryClientProvider>
  );
  const { container } = render(<InstallationSetup />, { wrapper: Wrap });
  return { writes, container, qc };
}

/**
 * Waits until the report has actually ARRIVED.
 *
 * Every "draws nothing" assertion below needs this, and none of them can get it
 * from the DOM: the screen renders nothing while the query is in flight for
 * exactly the same reason it renders nothing when the step is done, so a
 * `waitFor` over an absence is satisfied by the first pending frame and the test
 * passes without the answer ever landing. That is not a slow assertion, it is a
 * vacuous one — it passed against a build that rendered the model form for a
 * step it was never given.
 */
async function reportArrived(qc: QueryClient) {
  await waitFor(() =>
    expect(qc.getQueryState(["installation-setup"])?.status).toBe("success"),
  );
}

describe("the first-run setup gate", () => {
  // The model binding is the one thing a cold start cannot proceed without, so
  // it is the one thing asked for here.
  it("asks for the model binding", async () => {
    mount(setupReport(false, false));
    expect(await screen.findByText("Choose a model provider")).toBeTruthy();
  });

  // The models bound and no app yet: the platform question, asked once of
  // the person running the cold start. Google is the answer it opens on, with
  // the two fields an OAuth client has.
  it("asks what the organization runs on once the models are bound", async () => {
    mount(setupReport(true, false));
    expect(
      await screen.findByText("What does your organization run on?"),
    ).toBeTruthy();
    expect(
      screen.getByRole("radio", { name: /Google Workspace/ }),
    ).toHaveProperty("checked", true);
    expect(screen.getByLabelText("Client ID")).toBeTruthy();
    expect(screen.getByLabelText("Client secret")).toBeTruthy();
    // The redirect URIs are registered in the vendor's console, which is the
    // one part of this step done elsewhere: they stand in the open with their
    // copy buttons, not behind the fold the rest of the help sits in.
    expect(
      await screen.findByRole("button", { name: "Copy Sign-in URI" }),
    ).toBeTruthy();
  });

  it("stores the vendor's app through the same route settings uses", async () => {
    const { writes } = mount(setupReport(true, false));
    const user = userEvent.setup();
    await user.type(
      await screen.findByLabelText("Client ID"),
      "123.apps.googleusercontent.com",
    );
    await user.type(screen.getByLabelText("Client secret"), "GOCSPX-x");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(writes.length).toBe(1));
    expect(writes[0]?.url.endsWith("/installation/oauth-apps/google")).toBe(
      true,
    );
    expect(writes[0]?.body).toEqual({
      client_id: "123.apps.googleusercontent.com",
      client_secret: "GOCSPX-x",
    });
  });

  // The directory is REQUIRED here, unlike Settings: it is what puts Microsoft
  // on the login page, and an admin who registered Microsoft and then found no
  // Microsoft button would have nothing on either screen to tell them why.
  it("asks Microsoft for its directory, and will not store an app without one", async () => {
    const { writes } = mount(setupReport(true, false));
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("radio", { name: /Microsoft 365/ }),
    );
    // The redirect URIs are the one thing done in the vendor's console, so
    // the step says so in the open rather than in a fold; and the pin is
    // the directory sign-in runs on, which nothing else would explain.
    expect(
      screen.getByText("Register these redirect URIs on the app"),
    ).toBeTruthy();
    expect(
      screen.getByText(/directory is what puts Microsoft on the login page/),
    ).toBeTruthy();
    await user.type(screen.getByLabelText("Client ID"), "entra-app");
    await user.type(screen.getByLabelText("Client secret"), "s3cret");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    expect(
      screen.getByText("Still needed: Directory (tenant) ID"),
    ).toBeTruthy();
    expect(writes.length).toBe(0);

    await user.type(
      screen.getByLabelText("Directory (tenant) ID"),
      "00000000-0000-0000-0000-000000000000",
    );
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(writes.length).toBe(1));
    expect(writes[0]?.url.endsWith("/installation/oauth-apps/microsoft")).toBe(
      true,
    );
    expect(writes[0]?.body).toEqual({
      client_id: "entra-app",
      client_secret: "s3cret",
      tenant: "00000000-0000-0000-0000-000000000000",
    });
  });

  // Google has no directory concept, so asking for one there would be a field
  // that does nothing — and a blocker nobody could clear.
  it("asks Google for no directory", async () => {
    mount(setupReport(true, false));
    const user = userEvent.setup();
    await screen.findByRole("radio", { name: /Google Workspace/ });
    expect(screen.queryByLabelText("Directory (tenant) ID")).toBeNull();
    await user.type(screen.getByLabelText("Client ID"), "google-app");
    await user.type(screen.getByLabelText("Client secret"), "s3cret");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    expect(screen.queryByText(/Still needed/)).toBeNull();
  });

  // The regression this screen was once built wrong for: an installation with
  // no app must still reach the company form. The step does not block, so
  // "not now" — or IMAP, which stores nothing installation-wide — lets the
  // reader through, and the answer is remembered so the question is asked once.
  it("lets a reader through on IMAP with Not now, and remembers it", async () => {
    const { container, writes } = mount(setupReport(true, false));
    const user = userEvent.setup();
    await user.click(await screen.findByRole("radio", { name: /IMAP/ }));
    await user.click(screen.getByRole("button", { name: "Not now" }));
    await waitFor(() => expect(container.innerHTML).toBe(""));
    expect(writes.length).toBe(0);
    expect(outstandingStep(setupReport(true, false), true)).toBeUndefined();
  });

  // The decline is the ACCOUNT's answer, not the browser's. Keyed on the
  // browser alone it outlived the installation it was about: a machine that
  // had run one cold start carried the answer into the next, and the second
  // installation's setup skipped the platform step entirely.
  it("keeps asking when the remembered decline belongs to another account", async () => {
    window.localStorage.setItem(
      "margince.first-run.platform-declined:00000000-0000-4000-8000-000000000009",
      "1",
    );
    mount(setupReport(true, false));
    expect(
      await screen.findByRole("radio", { name: /Google Workspace/ }),
    ).toBeTruthy();
  });

  it("stays answered for the account that gave it", async () => {
    window.localStorage.setItem(
      "margince.first-run.platform-declined:00000000-0000-4000-8000-000000000001",
      "1",
    );
    const { container } = mount(setupReport(true, false));
    await waitFor(() => expect(container.innerHTML).toBe(""));
  });

  // IMAP has no installation-wide app, so the one thing the answer can do is
  // connect the mailbox of the person on screen: the same standing connect
  // Settings makes, and the step is done once the server confirms it.
  it("connects the reader's own mailbox on IMAP, through the standing connect", async () => {
    const { container, writes } = mount(setupReport(true, false));
    const user = userEvent.setup();
    await user.click(await screen.findByRole("radio", { name: /IMAP/ }));
    await user.type(screen.getByLabelText("IMAP server *"), "mail.example.org");
    await user.type(
      screen.getByLabelText("Email address *"),
      "lars@example.org",
    );
    await user.type(screen.getByLabelText("App password *"), "app-password");
    await user.click(screen.getByRole("button", { name: "Connect" }));
    await waitFor(() => expect(container.innerHTML).toBe(""));
    expect(
      writes.map((w) => w.url.endsWith("/connectors/imap/connect")),
    ).toEqual([true]);
    expect(writes[0]?.body).toEqual({
      imap: {
        host: "mail.example.org",
        port: 993,
        username: "lars@example.org",
        secret: "app-password",
        mailbox: "INBOX",
        max_messages: 50,
      },
    });
  });

  it("lets a reader past the app step with Not now", async () => {
    const { container, writes } = mount(setupReport(true, false));
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Not now" }));
    await waitFor(() => expect(container.innerHTML).toBe(""));
    expect(writes.length).toBe(0);
  });

  // Nothing at all when there is nothing outstanding, so a caller can put this
  // in front of the next screen without asking the same question twice.
  it("draws nothing once every blocking step is done", async () => {
    const { container, qc } = mount(setupReport(true, true));
    await reportArrived(qc);
    expect(container.innerHTML).toBe("");
  });

  // A decline only counts for a step that does not block. Should a server
  // ever report the app step as blocking, a remembered "not now" must not let
  // a reader past a gate the server holds shut.
  it("keeps asking a blocking app step whatever was declined", () => {
    const report: { complete: boolean; steps: Step[] } = {
      complete: false,
      steps: [
        { step: "ai_models", configured: true, blocking: true },
        { step: "oauth_app", configured: false, blocking: true },
      ],
    };
    expect(outstandingStep(report, true)?.step).toBe("oauth_app");
  });

  // The key BEFORE the binding: a binding whose vendor has no key reads as
  // configured and fails on the first real call, where a key with no binding is
  // simply a key and the next attempt completes it.
  it("stores the key before it binds the models", async () => {
    const user = userEvent.setup();
    const { writes } = mount(setupReport(false, false));
    await screen.findByText("Choose a model provider");
    await user.type(screen.getByLabelText("API key"), "AIza-secret");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(writes.length).toBe(2));
    expect(writes[0].url).toBe("/v1/ai/provider-keys/gemini");
    expect(writes[1].url).toBe("/v1/ai/routing");
  });

  // Every chat tier, not just one. A half-bound installation answers for one
  // task and refuses another, with nothing on screen saying which was configured.
  it("binds every chat tier and the embedding lane", async () => {
    const user = userEvent.setup();
    const { writes } = mount(setupReport(false, false));
    await screen.findByText("Choose a model provider");
    await user.type(screen.getByLabelText("API key"), "AIza-secret");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(writes.length).toBe(2));
    const routing = writes[1].body as {
      tiers: Record<string, { provider: string; model: string }>;
      embeddings: { provider: string; model: string };
    };
    expect(Object.keys(routing.tiers).sort()).toEqual([
      "cheap_cloud",
      "frontier",
      "local_small",
      "premium",
    ]);
    for (const bound of Object.values(routing.tiers)) {
      expect(bound.provider).toBe("gemini");
      expect(bound.model).toBe("gemini-3.1-flash-lite");
    }
    expect(routing.embeddings.model).toBe("gemini-embedding-001");
  });

  // OpenRouter is a preset, and openai_compatible fails closed without a
  // base_url — so choosing it must carry the endpoint on every lane rather than
  // leaving the reader to discover an adapter name.
  it("carries the broker endpoint when OpenRouter is chosen", async () => {
    const user = userEvent.setup();
    const { writes } = mount(setupReport(false, false));
    await screen.findByText("Choose a model provider");
    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Provider" }),
      "OpenRouter",
    );
    await user.type(screen.getByLabelText("API key"), "sk-or-secret");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(writes.length).toBe(2));
    expect(writes[0].url).toBe("/v1/ai/provider-keys/openai_compatible");
    const routing = writes[1].body as {
      tiers: Record<string, { base_url?: string }>;
      embeddings: { base_url?: string };
    };
    for (const bound of Object.values(routing.tiers)) {
      expect(bound.base_url).toBe("https://openrouter.ai/api");
    }
    expect(routing.embeddings.base_url).toBe("https://openrouter.ai/api");
  });

  // Continue is always pressable. Pressing it early is how a reader learns
  // what is missing: the field turns red and the rail names it, and nothing is
  // written — a grey button would have said only that something is wrong.
  it("names what is still needed when Continue is pressed early", async () => {
    const user = userEvent.setup();
    const { writes } = mount(setupReport(false, false));
    await screen.findByText("Choose a model provider");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    expect(screen.getByText("Still needed: API key")).toBeTruthy();
    expect(screen.getAllByText("Needed to continue").length).toBe(1);
    expect(writes.length).toBe(0);
    // Typing the key clears the note; the press then goes through.
    await user.type(screen.getByLabelText("API key"), "AIza-secret");
    expect(screen.queryByText("Still needed: API key")).toBeNull();
  });

  it("names both halves of the app when Continue is pressed with neither", async () => {
    const user = userEvent.setup();
    const { writes } = mount(setupReport(true, false));
    await screen.findByRole("radio", { name: /Google Workspace/ });
    await user.click(screen.getByRole("button", { name: "Continue" }));
    expect(
      screen.getByText("Still needed: Client ID, Client secret"),
    ).toBeTruthy();
    expect(writes.length).toBe(0);
  });

  // The key is the reader's only copy. Clearing it after the FIRST write left a
  // failed binding with an empty field, a disabled Continue, a step still
  // unconfigured and a reload that restored exactly that — the only way on was
  // to re-paste a key the server already held.
  it("keeps the key on screen when the binding fails, so Continue still works", async () => {
    const user = userEvent.setup();
    const { writes } = mount(setupReport(false, false), ["/ai/routing"]);
    await screen.findByText("Choose a model provider");
    const key = screen.getByLabelText<HTMLInputElement>("API key");
    await user.type(key, "AIza-secret");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(writes.length).toBe(2));

    // The field still holds it, and the button is pressable again.
    await waitFor(() => expect(key.value).toBe("AIza-secret"));
    expect(
      screen.getByRole("button", { name: "Continue" }).getAttribute("disabled"),
    ).toBeNull();
  });

  // A first-time admin should not have to know a model id by heart. The sheet
  // the installation was seeded with is what it can price, so it is what the
  // field offers — per lane, because an embedder cannot serve a chat tier.
  it("offers the seeded models for the chosen vendor, per lane", async () => {
    const user = userEvent.setup();
    mount(setupReport(false, false));
    await screen.findByText("Choose a model provider");

    await user.click(screen.getByRole("combobox", { name: "Model" }));
    expect(
      within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual([
      "gemini-3.1-flash-liteUS$0.25 → US$1.50",
      "gemini-3.5-flashUS$1.50 → US$9.00",
    ]);
    await user.keyboard("{Escape}");

    await user.click(screen.getByRole("combobox", { name: "Embedding model" }));
    expect(
      within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["gemini-embedding-001US$0.15"]);
  });

  it("binds the model a reader picks off the list", async () => {
    const user = userEvent.setup();
    const { writes } = mount(setupReport(false, false));
    await screen.findByText("Choose a model provider");

    await pickSuggestion(
      user,
      screen.getByRole("combobox", { name: "Model" }),
      /^gemini-3\.5-flash/,
    );
    await user.type(screen.getByLabelText("API key"), "AIza-secret");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() => expect(writes.length).toBe(2));
    const routing = writes[1].body as {
      tiers: Record<string, { model: string }>;
    };
    expect(routing.tiers.premium.model).toBe("gemini-3.5-flash");
  });

  // The sheet is a starting point, never a permitted list: the server takes any
  // id its vendor serves, and an admin arriving with a model we have not priced
  // must not be stopped at the door.
  it("binds a model the seeded sheet does not carry", async () => {
    const user = userEvent.setup();
    const { writes } = mount(setupReport(false, false));
    await screen.findByText("Choose a model provider");

    const model = screen.getByRole("combobox", { name: "Model" });
    await user.clear(model);
    await user.type(model, "gemini-4-experimental-0731");
    await user.type(screen.getByLabelText("API key"), "AIza-secret");
    await user.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() => expect(writes.length).toBe(2));
    const routing = writes[1].body as {
      tiers: Record<string, { model: string }>;
    };
    expect(routing.tiers.premium.model).toBe("gemini-4-experimental-0731");
  });

  // Switching vendor re-seeds the fields AND what they offer: the previous
  // vendor's ids mean nothing to this one, and offering them would be worse
  // than offering nothing.
  it("re-offers on the new vendor when the provider changes", async () => {
    const user = userEvent.setup();
    mount(setupReport(false, false));
    await screen.findByText("Choose a model provider");

    await pickOption(
      user,
      screen.getByRole("combobox", { name: "Provider" }),
      "OpenRouter",
    );
    await user.click(screen.getByRole("combobox", { name: "Model" }));
    expect(
      within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual([
      "mistralai/mistral-large-2512US$0.50 → US$1.50",
      "mistralai/mistral-small-3.2-24b-instructUS$0.10 → US$0.30",
    ]);
  });
});

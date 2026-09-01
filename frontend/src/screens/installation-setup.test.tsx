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
import { pickOption, pickSuggestion } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "./company.fixtures";
import { InstallationSetup, outstandingStep } from "./installation-setup";

afterEach(() => {
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

  // The regression this screen was built wrong for. An installation with no
  // Google app never reaches the company form, so `GET /company` keeps
  // answering 404 and the shell's onboarding gate rewrites every route back to
  // onboarding — an operator deploying without a Google app was locked out of
  // the whole product, with `PUT /v1/company` the only way past.
  it("lets a reader through with the models bound and no Google app", async () => {
    // The panel is gone entirely rather than replaced by a Google one: the app
    // is stored from settings, where the card also shows the redirect URIs
    // Google's console asks for.
    const { container, qc } = mount(setupReport(true, false));
    await reportArrived(qc);
    expect(container.innerHTML).toBe("");
  });

  // Nothing at all when there is nothing outstanding, so a caller can put this
  // in front of the next screen without asking the same question twice.
  it("draws nothing once every blocking step is done", async () => {
    const { container, qc } = mount(setupReport(true, true));
    await reportArrived(qc);
    expect(container.innerHTML).toBe("");
  });

  // A frontend older than its server, meeting a blocking step it has no panel
  // for. It lets the reader PAST rather than drawing the panel it does have —
  // the model form under an `oauth_app` heading would take a reader's API key
  // against a step they were never asked about — and past is what matters,
  // because the caller gates on this same predicate and would otherwise hold an
  // empty screen in front of them forever. The server pins ai_models as the
  // only blocker (TestOnlyTheModelBindingBlocksFirstRun); this is the far side
  // of that, for the deployment where the two versions disagree.
  it("lets a reader past a blocking step it has no panel for", async () => {
    const report: { complete: boolean; steps: Step[] } = {
      complete: false,
      steps: [
        { step: "ai_models", configured: true, blocking: true },
        { step: "oauth_app", configured: false, blocking: true },
      ],
    };
    // The caller's own question, asked the caller's way. An empty screen alone
    // would not tell the two failures apart: a gate that lets nobody through
    // and a gate that is finished both draw nothing, and it is this answer the
    // onboarding act reads to decide whether to keep the gate up at all.
    expect(outstandingStep(report)).toBeUndefined();

    const { container, qc } = mount(report);
    await reportArrived(qc);
    expect(container.innerHTML).toBe("");
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

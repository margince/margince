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
import { pickOption, pickSuggestion } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "./company.fixtures";
import { InstallationSetup } from "./installation-setup";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Step = { step: string; configured: boolean; blocking: boolean };

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

/** The server's answer, in the order the server gives it. */
function setupReport(
  ai: boolean,
  google: boolean,
): { complete: boolean; steps: Step[] } {
  return {
    complete: ai && google,
    steps: [
      { step: "ai_models", configured: ai, blocking: true },
      { step: "google_app", configured: google, blocking: true },
    ],
  };
}

function mount(
  report: ReturnType<typeof setupReport>,
  /** Paths whose write should fail, so a half-finished run can be exercised. */
  refuse: readonly string[] = [],
) {
  const writes: { url: string; body: unknown }[] = [];
  // Counted because WHEN the setup report is re-read is a product decision on
  // this screen: the ignition holds it back so the reader, not the query,
  // decides when the model step is over.
  const setupReads: number[] = [];
  const fetchMock = vi.fn(async (request: Request) => {
    const url = new URL(request.url).pathname;
    if (request.method === "GET" && url.endsWith("/installation/setup")) {
      setupReads.push(Date.now());
    }
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
  render(<InstallationSetup />, { wrapper: Wrap });
  return { writes, setupReads };
}

describe("the first-run setup gate", () => {
  // The order is the product decision the server pins: somebody with no model
  // bound cannot be shown a cold start, while the mailbox can wait.
  it("asks for the model binding before the platform question", async () => {
    mount(setupReport(false, false));
    expect(await screen.findByText("Choose a model provider")).toBeTruthy();
    expect(
      screen.queryByText("What does your organization run on?"),
    ).toBeNull();
  });

  it("asks what the organization runs on once the models are bound", async () => {
    mount(setupReport(true, false));
    expect(
      await screen.findByText("What does your organization run on?"),
    ).toBeTruthy();
    expect(screen.queryByText("Choose a model provider")).toBeNull();
  });

  // Nothing at all when there is nothing outstanding, so a caller can put this
  // in front of the next screen without asking the same question twice.
  it("draws nothing once every blocking step is done", async () => {
    mount(setupReport(true, true));
    await waitFor(() =>
      expect(screen.queryByText("Choose a model provider")).toBeNull(),
    );
    expect(
      screen.queryByText("What does your organization run on?"),
    ).toBeNull();
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

  it("sends the Google app as one pair", async () => {
    const user = userEvent.setup();
    const { writes } = mount(setupReport(true, false));
    await screen.findByText("What does your organization run on?");
    await user.type(
      screen.getByLabelText("Client ID"),
      "123-abc.apps.googleusercontent.com",
    );
    await user.type(screen.getByLabelText("Client secret"), "GOCSPX-secret");
    await user.click(screen.getByRole("button", { name: "Continue" }));
    await waitFor(() => expect(writes.length).toBe(1));
    expect(writes[0].url).toBe("/v1/installation/google-app");
    expect(writes[0].body).toEqual({
      client_id: "123-abc.apps.googleusercontent.com",
      client_secret: "GOCSPX-secret",
    });
  });

  // Binding a model is the one write in first run that the screen marks rather
  // than getting out of the way of. What that costs mechanically is a deferred
  // refetch, and these are the two halves of it.
  describe("the ignition", () => {
    async function bind(): Promise<ReturnType<typeof mount>> {
      const user = userEvent.setup();
      const harness = mount(setupReport(false, false));
      await screen.findByText("Choose a model provider");
      await user.type(screen.getByLabelText("API key"), "AIza-secret");
      await user.click(screen.getByRole("button", { name: "Continue" }));
      await waitFor(() => expect(harness.writes.length).toBe(2));
      return harness;
    }

    it("holds the screen after the binding lands, and names whose key was sealed", async () => {
      const { setupReads } = await bind();
      // The sequence is on screen, not the next question.
      expect(
        await screen.findByText(/sealed in the vault · Google Gemini/),
      ).toBeTruthy();
      expect(screen.getByText("It has a pulse.")).toBeTruthy();
      // And the one thing it may still not do, which is the point of saying any
      // of it here.
      expect(screen.getByText(/unless you say so/)).toBeTruthy();
      // The report has NOT been re-read: the write landed, and the screen is the
      // reader's to leave.
      const readsWhileWatching = setupReads.length;
      await waitFor(() => expect(setupReads.length).toBe(readsWhileWatching));
    });

    it("re-reads the report only once the reader presses past it", async () => {
      const user = userEvent.setup();
      const { setupReads } = await bind();
      await screen.findByText(/sealed in the vault/);
      const before = setupReads.length;
      await user.click(screen.getByRole("button", { name: "Carry on" }));
      await waitFor(() => expect(setupReads.length).toBeGreaterThan(before));
    });
  });

  // The platform answer is one question covering mail AND sign-in, and what it
  // changes on screen is which gap the reader is told about. Every answer has
  // one: the two that need no Google app here cannot finish first run, and the
  // one that does still does not turn the login door on. A screen that names two
  // of the three reads as a guarantee for the third.
  describe("what each platform answer says it still leaves undone", () => {
    // Reads the notices as the reader sees them — by their words rather than by
    // a live-region role, so which of the two announces stays the surface's
    // decision and not something this test pins.
    async function choose(label: string): Promise<void> {
      const user = userEvent.setup();
      mount(setupReport(true, false));
      await screen.findByText("What does your organization run on?");
      await user.click(screen.getByRole("radio", { name: new RegExp(label) }));
    }

    const STUCK = /cannot finish yet/;

    it("tells the Google path that saving the app does not enable sign-in", async () => {
      await choose("Google Workspace");
      expect(screen.getByText(/MARGINCE_GMAIL_CLIENT_ID/)).toBeTruthy();
      // And does NOT claim first run is stuck: this path can finish.
      expect(screen.queryByText(STUCK)).toBeNull();
    });

    it("sends the Microsoft path to whoever runs the server, and says it cannot finish", async () => {
      await choose("Microsoft 365");
      expect(screen.getByText(/MARGINCE_GRAPH_CLIENT_ID/)).toBeTruthy();
      expect(screen.getByText(STUCK)).toBeTruthy();
    });

    it("tells the IMAP path the credentials live on the mailbox", async () => {
      await choose("Neither");
      expect(
        screen.getByText(/IMAP mailbox carries its own host/),
      ).toBeTruthy();
      expect(screen.getByText(STUCK)).toBeTruthy();
    });

    // The fields stay usable on every answer, because pasting an app is the only
    // way past the blocking step — hiding them on the two paths that do not need
    // one would leave a reader with a refusal and no way to answer it.
    it("keeps the app fields usable whichever platform is chosen", async () => {
      await choose("Microsoft 365");
      expect(screen.getByLabelText("Client ID")).toBeTruthy();
      expect(screen.getByLabelText("Client secret")).toBeTruthy();
    });
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

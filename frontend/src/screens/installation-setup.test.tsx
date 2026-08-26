/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { LocaleProvider } from "../i18n";
import { jsonResponse } from "./company.fixtures";
import { InstallationSetup } from "./installation-setup";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Step = { step: string; configured: boolean; blocking: boolean };

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
  const fetchMock = vi.fn(async (request: Request) => {
    const url = new URL(request.url).pathname;
    if (request.method !== "GET") {
      writes.push({ url, body: JSON.parse(await request.text()) });
      if (refuse.some((p) => url.endsWith(p))) {
        return jsonResponse({ title: "refused" }, 500);
      }
      return new Response(null, { status: 204 });
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
  return { writes };
}

describe("the first-run setup gate", () => {
  // The order is the product decision the server pins: somebody with no model
  // bound cannot be shown a cold start, while the mailbox can wait.
  it("asks for the model binding before the Google app", async () => {
    mount(setupReport(false, false));
    expect(await screen.findByText("Choose a model provider")).toBeTruthy();
    expect(screen.queryByText("Connect a Google app")).toBeNull();
  });

  it("asks for the Google app once the models are bound", async () => {
    mount(setupReport(true, false));
    expect(await screen.findByText("Connect a Google app")).toBeTruthy();
    expect(screen.queryByText("Choose a model provider")).toBeNull();
  });

  // Nothing at all when there is nothing outstanding, so a caller can put this
  // in front of the next screen without asking the same question twice.
  it("draws nothing once every blocking step is done", async () => {
    mount(setupReport(true, true));
    await waitFor(() =>
      expect(screen.queryByText("Choose a model provider")).toBeNull(),
    );
    expect(screen.queryByText("Connect a Google app")).toBeNull();
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
    await screen.findByText("Connect a Google app");
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
});

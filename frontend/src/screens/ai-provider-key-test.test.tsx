/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiProviderKeysCard } from "./ai-provider-keys";

// The Test button on a provider row: the server asks the vendor with the key
// it already holds, and the row prints the answer as a closed reason.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const READER: GrantSpec = { ai_routing: ["read"] };
const EDITOR: GrantSpec = { ai_routing: ["read", "update"] };

const PROVIDERS = [
  {
    provider: "gemini",
    configured: true,
    env_var: "GEMINI_API_KEY",
    optional: false,
  },
  {
    provider: "openai",
    configured: false,
    env_var: "OPENAI_API_KEY",
    optional: false,
  },
  {
    provider: "jev",
    configured: true,
    env_var: "TYPESAFE_API_KEY",
    optional: false,
  },
  {
    provider: "jev_compatible",
    configured: false,
    env_var: "JEV_COMPATIBLE_API_KEY",
    optional: true,
  },
];

function backendFor(allow: GrantSpec, answer: () => Response) {
  const tested: string[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.endsWith("/test") && req.method === "POST") {
        tested.push(req.url.split("/ai/provider-keys/")[1]);
        return answer();
      }
      if (req.url.includes("/ai/provider-keys")) {
        if (req.method === "PUT") return new Response(null, { status: 204 });
        return jsonResponse({ providers: PROVIDERS });
      }
      if (
        req.url.includes("/ai/routing") ||
        req.url.includes("/ai-model-rates")
      ) {
        return jsonResponse({}, 404);
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, tested };
}

function render() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AiProviderKeysCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

async function testRow(
  user: ReturnType<typeof userEvent.setup>,
  provider: string,
) {
  await user.click(
    await screen.findByRole("button", { name: `Manage ${provider}` }),
  );
  const row = await screen.findByTestId(`ai-provider-key-${provider}`);
  await user.click(within(row).getByRole("button", { name: /^test$/i }));
  return row;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("testing a provider key", () => {
  it("reports how many models a vendor serves on a pass", async () => {
    const user = userEvent.setup();
    const backend = backendFor(READER, () =>
      jsonResponse({ provider: "gemini", ok: true, model_count: 42 }),
    );
    vi.stubGlobal("fetch", backend.fetchMock);
    render();

    const row = await testRow(user, "gemini");

    expect(await within(row).findByText("Connected")).toBeTruthy();
    expect(within(row).getByText("42 models available")).toBeTruthy();
    // A read: a seat that may not change the key may still ask whether it works.
    expect(backend.tested).toEqual(["gemini/test"]);
  });

  // The distinction the button exists for: a refused key is a key to replace,
  // a throttled one may be fine, and a vendor that is down is neither.
  it.each([
    ["auth_failed", /refused this key/i],
    ["rate_limited", /rate-limiting this key/i],
    ["unreachable", /did not respond/i],
    ["no_endpoint", /no tier uses this provider yet/i],
    ["no_key", /no key is stored/i],
    ["profile_forbids", /does not allow access/i],
    ["not_published", /cannot test this provider/i],
  ])("names the reason a test failed: %s", async (reason, words) => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(READER, () =>
        jsonResponse({ provider: "gemini", ok: false, reason }),
      ).fetchMock,
    );
    render();

    const row = await testRow(user, "gemini");

    expect(await within(row).findByText("Test failed")).toBeTruthy();
    expect(within(row).getByText(words)).toBeTruthy();
  });

  it("says a request that failed outright failed", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(READER, () =>
        jsonResponse({ title: "Forbidden", status: 403 }, 403),
      ).fetchMock,
    );
    render();

    const row = await testRow(user, "gemini");

    expect(await within(row).findByText("Test failed")).toBeTruthy();
  });

  // Every key the server can test gets the button — decision providers too,
  // each tested at the route its host answers. A row with no key and none
  // optional has nothing to test.
  it("offers Test on every row that holds, or may skip, a key", async () => {
    vi.stubGlobal(
      "fetch",
      backendFor(READER, () => jsonResponse({})).fetchMock,
    );
    render();

    const user = userEvent.setup();
    for (const provider of ["gemini", "jev", "jev_compatible", "openai"]) {
      await user.click(
        await screen.findByRole("button", { name: `Manage ${provider}` }),
      );
      const row = await screen.findByTestId(`ai-provider-key-${provider}`);
      const test = within(row).queryByRole("button", { name: /^test$/i });
      // Only the vendor that holds no key and may not skip one has no Test.
      expect(test !== null).toBe(provider !== "openai");
      await user.keyboard("{Escape}");
    }
  });

  // A broker's key endpoint and the decision probe answer yes or no and list
  // nothing, so a pass says the key was accepted rather than "0 models".
  it("says a key was accepted when the test listed no models", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(READER, () => jsonResponse({ provider: "jev", ok: true }))
        .fetchMock,
    );
    render();

    const row = await testRow(user, "jev");

    expect(await within(row).findByText("Connected")).toBeTruthy();
    expect(
      within(row).getByText("The provider accepted the key."),
    ).toBeTruthy();
    expect(within(row).queryByText(/models available/)).toBeNull();
  });

  // A keyless server that answered accepted no key, so the line does not say
  // it did.
  it("says the server answered when no key was sent", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(READER, () =>
        jsonResponse({ provider: "jev_compatible", ok: true }),
      ).fetchMock,
    );
    render();

    const row = await testRow(user, "jev_compatible");

    expect(await within(row).findByText("The server answered.")).toBeTruthy();
    expect(within(row).queryByText(/accepted the key/)).toBeNull();
  });

  // The empty decision request proves the server answered, not that it read
  // the key, so a held key is not called accepted on that evidence.
  it("says an unconfirmed pass could not confirm the key", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(READER, () =>
        jsonResponse({ provider: "jev", ok: true, key_confirmed: false }),
      ).fetchMock,
    );
    render();

    const row = await testRow(user, "jev");

    expect(
      await within(row).findByText(/cannot confirm the key is valid/),
    ).toBeTruthy();
    expect(within(row).queryByText(/accepted the key/)).toBeNull();
  });

  // A result describes the key that was held when it ran; once that key is
  // replaced it says nothing about the one now stored.
  it("clears the result when the key is replaced", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      backendFor(EDITOR, () =>
        jsonResponse({ provider: "gemini", ok: false, reason: "auth_failed" }),
      ).fetchMock,
    );
    render();
    const row = await testRow(user, "gemini");
    expect(await within(row).findByText("Test failed")).toBeTruthy();

    await user.click(within(row).getByRole("button", { name: /^replace$/i }));
    await user.type(within(row).getByPlaceholderText(/paste/i), "sk-new");
    await user.click(within(row).getByRole("button", { name: /save key/i }));

    await waitFor(() =>
      expect(within(row).queryByText("Test failed")).toBeNull(),
    );
  });
});

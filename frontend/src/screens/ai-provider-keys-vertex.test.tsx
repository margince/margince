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
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { type GrantSpec, meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { AiProviderKeysCard } from "./ai-provider-keys";
import { serviceAccountProblem } from "./service-account-key";

// The one vendor keyed by a FILE: Gemini on Vertex takes a Google
// service-account key, pasted or picked, and sends it as the field the server
// takes for that kind. As with every key, nothing here renders one back.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const KEY_EDITOR: GrantSpec = { ai_routing: ["read", "update"] };

const KEY_FILE = JSON.stringify({
  type: "service_account",
  project_id: "acme-eu",
  client_email: "margince@acme-eu.iam.gserviceaccount.com",
  private_key: "-----BEGIN PRIVATE KEY-----\nMIIE\n-----END PRIVATE KEY-----\n",
});

function backendFor(configured: boolean) {
  const puts: Array<{ url: string; body: unknown }> = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow: KEY_EDITOR }));
      }
      if (req.url.includes("/ai/provider-keys")) {
        if (req.method === "PUT") {
          puts.push({ url: req.url, body: await req.json() });
          return new Response(null, { status: 204 });
        }
        return jsonResponse({
          providers: [
            {
              provider: "gemini_vertex",
              configured,
              env_var: "GEMINI_VERTEX_SA_JSON",
              credential_kind: "service_account",
            },
          ],
        });
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, puts };
}

const render = (ui: ReactNode) => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
};

async function openRow(user: ReturnType<typeof userEvent.setup>) {
  const row = await screen.findByTestId("ai-provider-key-gemini_vertex");
  await user.click(
    within(row).getByRole("button", { name: /^(add|replace)$/i }),
  );
  return row;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a service-account provider key", () => {
  it("reads as a configured service-account key, and offers a key-file box rather than a password field", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backendFor(true).fetchMock);
    render(<AiProviderKeysCard />);

    expect(
      await screen.findByText("Service account key configured"),
    ).toBeInTheDocument();
    const row = await openRow(user);
    const box = within(row).getByLabelText("Service-account key (JSON)");
    expect(box.tagName).toBe("TEXTAREA");
    expect(box).toHaveValue("");
    expect(within(row).queryByPlaceholderText(/paste the api key/i)).toBeNull();
  });

  it("reads a picked key file into the box and sends it as service_account_json", async () => {
    const user = userEvent.setup();
    const backend = backendFor(false);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiProviderKeysCard />);

    const row = await openRow(user);
    await user.upload(
      within(row).getByLabelText("Or choose the key file"),
      new File([KEY_FILE], "acme-eu.json", { type: "application/json" }),
    );
    await waitFor(() =>
      expect(
        within(row).getByLabelText("Service-account key (JSON)"),
      ).toHaveValue(KEY_FILE),
    );
    await user.click(within(row).getByRole("button", { name: /save key/i }));

    await waitFor(() => expect(backend.puts).toHaveLength(1));
    expect(backend.puts[0].url).toContain("/ai/provider-keys/gemini_vertex");
    expect(backend.puts[0].body).toEqual({ service_account_json: KEY_FILE });
  });

  it("refuses a paste that is not JSON, says why, and sends nothing", async () => {
    const user = userEvent.setup();
    const backend = backendFor(false);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiProviderKeysCard />);

    const row = await openRow(user);
    await user.click(within(row).getByLabelText("Service-account key (JSON)"));
    await user.paste("AIzaSy-this-is-an-api-key");
    await user.click(within(row).getByRole("button", { name: /save key/i }));

    expect(await within(row).findByText(/this is not json/i)).toBeVisible();
    expect(backend.puts).toHaveLength(0);
  });
});

describe("serviceAccountProblem", () => {
  it("names what is wrong with a paste, and accepts a key file", () => {
    expect(serviceAccountProblem("   ")).toBe("serviceAccountKey.empty");
    expect(serviceAccountProblem("{not json")).toBe(
      "serviceAccountKey.notJson",
    );
    expect(serviceAccountProblem('{"type":"authorized_user"}')).toBe(
      "serviceAccountKey.notServiceAccount",
    );
    expect(serviceAccountProblem("[]")).toBe(
      "serviceAccountKey.notServiceAccount",
    );
    expect(serviceAccountProblem(`  ${KEY_FILE}\n`)).toBeUndefined();
  });
});

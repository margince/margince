/** @vitest-environment jsdom */
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
import { type Locale, LocaleProvider } from "../i18n";
import { AiProviderKeysCard } from "./ai-provider-keys";

// Settings → AI → Model provider keys.
//
// The property this file exists for is a negative: the key has no read path, so
// nothing here may render one. The server never returns it, and this screen must
// not invent a mask, a length or a prefix that implies it could.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const KEY_EDITOR: GrantSpec = { ai_routing: ["read", "update"] };
const KEY_READER: GrantSpec = { ai_routing: ["read"] };

const LISTED = {
  providers: [
    { provider: "gemini", configured: true, env_var: "GEMINI_API_KEY" },
    { provider: "openai", configured: false, env_var: "OPENAI_API_KEY" },
  ],
};

function backendFor(allow: GrantSpec) {
  const puts: Array<{ url: string; body: unknown }> = [];
  const deletes: string[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const req =
        input instanceof Request ? input : new Request(String(input), init);
      if (req.url.endsWith("/v1/me")) {
        return jsonResponse(meFixture({ allow }));
      }
      if (req.url.includes("/ai/provider-keys")) {
        if (req.method === "PUT") {
          puts.push({ url: req.url, body: await req.json() });
          return new Response(null, { status: 204 });
        }
        if (req.method === "DELETE") {
          deletes.push(req.url);
          return new Response(null, { status: 204 });
        }
        return jsonResponse(LISTED);
      }
      throw new Error(`unexpected request: ${req.method} ${req.url}`);
    },
  );
  return { fetchMock, puts, deletes };
}

/** Opens one vendor's paste field, the way a reader does. */
async function openKey(
  user: ReturnType<typeof userEvent.setup>,
  provider: string,
) {
  const row = screen.getByTestId(`ai-provider-key-${provider}`);
  await user.click(
    within(row).getByRole("button", { name: /^(add|replace)$/i }),
  );
  return row;
}

// Returns the client alongside the rendered tree: one case asserts on what
// React Query still HOLDS, which the DOM cannot show.
const render = (ui: ReactNode, locale: Locale = "en") => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return {
    ...rtlRender(
      <QueryClientProvider client={client}>
        <LocaleProvider initial={locale}>{ui}</LocaleProvider>
      </QueryClientProvider>,
    ),
    client,
  };
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("AiProviderKeysCard", () => {
  it("says which vendors hold a key and which do not", async () => {
    vi.stubGlobal("fetch", backendFor(KEY_EDITOR).fetchMock);
    render(<AiProviderKeysCard />);

    expect(await screen.findByText(/^configured$/i)).toBeTruthy();
    expect(screen.getByText(/^not set$/i)).toBeTruthy();
    // Every servable vendor gets a row, not only the configured one — an
    // installation that has configured nothing is the one that needs this card.
    // The row says which state it is in and offers the verb that changes it:
    // Replace where a key is held, Add where none is.
    expect(screen.getByRole("button", { name: /^replace$/i })).toBeTruthy();
    expect(screen.getByRole("button", { name: /^add$/i })).toBeTruthy();
    // And no paste field until one is asked for. Six open password boxes is
    // what this card used to be, and it is not a page anybody could audit.
    expect(screen.queryByPlaceholderText(/paste/i)).toBeNull();
  });

  it("never renders the key, and offers no field that could hold one read back", async () => {
    vi.stubGlobal("fetch", backendFor(KEY_EDITOR).fetchMock);
    render(<AiProviderKeysCard />);
    await screen.findByText(/^configured$/i);

    const user = userEvent.setup();
    await openKey(user, "gemini");
    await openKey(user, "openai");

    // Every input starts EMPTY, including the vendor that has a key. A prefilled
    // or masked value would imply the real one is retrievable and invite a
    // screenshot; it is not retrievable, and the card must not suggest it is.
    for (const input of screen.getAllByPlaceholderText(/paste/i)) {
      expect(input).toHaveValue("");
      // Typed as a password so the browser does not offer to remember a
      // credential this app deliberately never keeps client-side.
      expect(input).toHaveAttribute("type", "password");
    }
  });

  it("sends the pasted key to the vendor's own route, trimmed", async () => {
    const backend = backendFor(KEY_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiProviderKeysCard />);
    await screen.findByText(/^not set$/i);

    const user = userEvent.setup();
    const row = await openKey(user, "openai");
    await user.type(
      within(row).getByPlaceholderText(/paste the api key/i),
      "  sk-openai-pasted  ",
    );
    await user.click(within(row).getByRole("button", { name: /save key/i }));

    await waitFor(() => expect(backend.puts).toHaveLength(1));
    expect(backend.puts[0].url).toContain("/ai/provider-keys/openai");
    // Trimmed before it leaves: a pasted credential carries whatever the
    // clipboard did, and a trailing newline authenticates nothing.
    expect(backend.puts[0].body).toEqual({ api_key: "sk-openai-pasted" });
  });

  it("clears the field on success so the credential does not linger on screen", async () => {
    vi.stubGlobal("fetch", backendFor(KEY_EDITOR).fetchMock);
    render(<AiProviderKeysCard />);
    await screen.findByText(/^not set$/i);

    const user = userEvent.setup();
    const row = await openKey(user, "openai");
    const input = within(row).getByPlaceholderText(/paste the api key/i);
    await user.type(input, "sk-openai");
    await user.click(within(row).getByRole("button", { name: /save key/i }));

    // The field folds away with the row on success, which is the strongest
    // form of "not on screen any more".
    await waitFor(() =>
      expect(within(row).queryByPlaceholderText(/paste/i)).toBeNull(),
    );
  });

  // And it leaves the MUTATION's state with the field.
  //
  // React Query keeps `variables` after a mutation succeeds — normally a
  // convenience, and for this one mutation a credential sitting in memory that
  // the observer and the devtools can both read. The key still travels as a
  // variable, because passing it any other way reintroduces the stale-closure
  // refusal that rule exists to prevent; what changes is how long it stays.
  //
  // Asserted through the query client's own mutation cache rather than through
  // the DOM, because the field being empty says nothing about what React Query
  // still holds — which is exactly how this would regress unnoticed.
  it("drops the credential from the mutation cache once the save settles", async () => {
    vi.stubGlobal("fetch", backendFor(KEY_EDITOR).fetchMock);
    const { client } = render(<AiProviderKeysCard />);
    await screen.findByText(/^not set$/i);

    const user = userEvent.setup();
    const row = await openKey(user, "openai");
    await user.type(
      within(row).getByPlaceholderText(/paste the api key/i),
      "sk-openai-secret",
    );
    await user.click(within(row).getByRole("button", { name: /save key/i }));

    await waitFor(() => {
      const held = client
        .getMutationCache()
        .getAll()
        .map((m) => JSON.stringify(m.state.variables ?? null))
        .join(" ");
      expect(held).not.toContain("sk-openai-secret");
    });
  });

  it("will not submit whitespace", async () => {
    const backend = backendFor(KEY_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiProviderKeysCard />);
    await screen.findByText(/^not set$/i);

    const user = userEvent.setup();
    const row = await openKey(user, "openai");
    await user.type(
      within(row).getByPlaceholderText(/paste the api key/i),
      "   ",
    );
    // Removing a credential is the Remove button; a blank write is a mistake the
    // server would refuse, so the button does not offer it.
    expect(
      within(row).getByRole("button", { name: /save key/i }),
    ).toBeDisabled();
    expect(backend.puts).toHaveLength(0);
  });

  it("offers Remove only where a key is held", async () => {
    const user = userEvent.setup();
    const backend = backendFor(KEY_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiProviderKeysCard />);
    await screen.findByText(/^configured$/i);

    // Removing is behind the row's own verb, with the paste field: it is a
    // change to the credential, not a reading of it.
    await openKey(user, "gemini");
    // One vendor has a key, one does not — so exactly one Remove.
    const removes = screen.getAllByRole("button", { name: /remove/i });
    expect(removes).toHaveLength(1);

    // The button OPENS a confirmation; it does not delete. The credential
    // cannot be read back, so a stray click is unrecoverable and takes every
    // lane bound to the vendor down with it.
    await user.click(removes[0]);
    expect(backend.deletes).toHaveLength(0);

    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: /remove/i }));
    await waitFor(() => expect(backend.deletes).toHaveLength(1));
    expect(backend.deletes[0]).toContain("/ai/provider-keys/gemini");
  });

  // Dismissing the confirmation must leave the key alone — a modal that deletes
  // whichever way it closes is worse than no modal, because it looks like a
  // choice.
  it("keeps the key when the removal confirmation is dismissed", async () => {
    const user = userEvent.setup();
    const backend = backendFor(KEY_EDITOR);
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiProviderKeysCard />);
    await screen.findByText(/^configured$/i);

    await openKey(user, "gemini");
    await user.click(screen.getByRole("button", { name: /remove/i }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: /cancel/i }));

    expect(backend.deletes).toHaveLength(0);
  });

  // The hint interpolates the provider's environment variable, and the
  // catalogs wrote it `{{envVar}}` while the translator matches a SINGLE pair
  // — so the name rendered wrapped in stray braces. Asserting the name alone
  // would have passed on `{GEMINI_API_KEY}`, which is why the brace is what
  // this checks. The catalog test could not see it either: it extracts
  // `{(\w+)}`, and that matches the inner pair of a double one.
  it("names the environment variable in the hint, with no stray braces", async () => {
    vi.stubGlobal("fetch", backendFor(KEY_EDITOR).fetchMock);
    render(<AiProviderKeysCard />);
    await screen.findByText(/^configured$/i);

    const user = userEvent.setup();
    for (const [provider, envVar] of [
      ["gemini", "GEMINI_API_KEY"],
      ["openai", "OPENAI_API_KEY"],
    ]) {
      const row = await openKey(user, provider);
      // The row NAMES the variable whether it is open or not, and the hint
      // under the field explains it. Both readings interpolate, so both are
      // checked for the brace the catalogs once left behind.
      for (const shown of within(row).getAllByText(new RegExp(envVar))) {
        expect(shown.textContent).toContain(envVar);
        expect(shown.textContent).not.toContain(`{${envVar}}`);
        expect(shown.textContent).not.toContain("envVar");
      }
    }
  });

  // A seat that reaches the AI tab on some other grant and holds no
  // `ai_routing:read`. The card says withheld and asks the server for NOTHING:
  // a 403 error box would read as a broken installation, and an absent card
  // would claim this installation holds no credentials — a statement about the
  // data where the truth is only about who may see it.
  it("withholds the list from a seat without the read grant, and asks for nothing", async () => {
    const backend = backendFor({ automation: ["read"] });
    vi.stubGlobal("fetch", backend.fetchMock);
    render(<AiProviderKeysCard />);

    expect(await screen.findByText(/only an operator/i)).toBeTruthy();
    expect(screen.queryByPlaceholderText(/paste/i)).toBeNull();
    const asked = backend.fetchMock.mock.calls.map((c) => String(c[0]));
    expect(asked.some((u) => u.includes("/ai/provider-keys"))).toBe(false);
  });

  it("disables the controls for a reader who may look but not change", async () => {
    vi.stubGlobal("fetch", backendFor(KEY_READER).fetchMock);
    render(<AiProviderKeysCard />);
    await screen.findByText(/^configured$/i);

    // Refused, not hidden: an operator who must ask somebody else to rotate a
    // key still reads which vendors hold one, off the rows themselves. What is
    // refused is the verb that would change one — and unlike a lane row, there
    // is nothing behind it to read: an empty password box states no fact.
    expect(screen.getByRole("button", { name: /^replace$/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /^add$/i })).toBeDisabled();
    expect(screen.getByText(/^not set$/i)).toBeTruthy();
  });

  // A credential must not survive the fold. Closing the editor DROPS what was
  // typed, so a key half-pasted and thought better of does not reappear the
  // next time the row is opened — on a screenshare, or for whoever is at the
  // desk next.
  it("drops a typed key when the editor is closed again", async () => {
    const user = userEvent.setup();
    vi.stubGlobal("fetch", backendFor(KEY_EDITOR).fetchMock);
    render(<AiProviderKeysCard />);
    await screen.findByText(/^not set$/i);

    const row = await openKey(user, "openai");
    await user.type(
      within(row).getByPlaceholderText(/paste the api key/i),
      "sk-typed-then-abandoned",
    );
    // Close, then open again: the field is empty, not holding what was typed.
    await user.click(within(row).getByRole("button", { name: /^add$/i }));
    await user.click(within(row).getByRole("button", { name: /^add$/i }));
    expect(within(row).getByPlaceholderText(/paste the api key/i)).toHaveValue(
      "",
    );
  });
});

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { AdapterFields } from "./ai-routing-fields";

// The Host field says what THIS adapter does with the address it is given. A
// chat broker, OpenRouter's decisions endpoint and a local decision server each
// append a different path, and only one of them has a default — so one sentence
// copied across all three tells two of them the wrong thing.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function mountFields(provider: string, lane: "chat" | "decisions") {
  // The model list is asked of the vendor while the fields are open; an empty
  // answer leaves the host field as the only thing under test.
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => new Response(JSON.stringify({ models: [] }))),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <AdapterFields
          label="Provider"
          lane={lane}
          laneName={lane === "decisions" ? "decisions" : "premium"}
          binding={{ provider, model: "m" }}
          catalogue={[]}
          disabled={false}
          onChange={() => undefined}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the Host field's help", () => {
  it("tells a chat broker that /v1 is added and a host is required", () => {
    mountFields("openai_compatible", "chat");
    expect(screen.getByLabelText("Host")).toHaveAccessibleDescription(
      /\/v1 is added.*Required/,
    );
  });

  // Any server on the Jev wire has no address of its own, so its endpoint is
  // required, and it is the full URL: nothing is appended to it.
  it("asks jev_compatible for its full endpoint, with both shapes it takes", () => {
    mountFields("jev_compatible", "decisions");
    const host = screen.getByLabelText("Host");
    expect(host).toHaveAccessibleDescription(/used as written.*Required/);
    expect(host).toHaveAccessibleDescription(
      /https:\/\/openrouter\.ai\/api\/alpha\/decisions/,
    );
    expect(host).toHaveAccessibleDescription(
      /http:\/\/127\.0\.0\.1:8767\/v1\/systemone/,
    );
    expect(host).not.toHaveAccessibleDescription(/is added/);
  });

  // TypeSafe's own API has a default endpoint, so its host is optional.
  it("offers jev its host, with the official endpoint it falls back to", () => {
    mountFields("jev", "decisions");
    const host = screen.getByLabelText("Host");
    expect(host).toHaveAccessibleDescription(/Leave blank/);
    expect(host).toHaveAttribute(
      "placeholder",
      "https://api.typesafe.ai/v1/systemone",
    );
  });
});

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

  it("tells OpenRouter's decisions endpoint the path it appends", () => {
    mountFields("openrouter_decision", "decisions");
    const host = screen.getByLabelText("Host");
    expect(host).toHaveAccessibleDescription(/\/alpha\/decisions is added/);
    expect(host).not.toHaveAccessibleDescription(/\/v1 is added/);
  });

  // A local decision server has a default address, so its host is optional —
  // and it is offered at all because the default is loopback, which is only
  // right when the server runs beside the API.
  it("offers a local decision server its host, with the default it falls back to", () => {
    mountFields("laya", "decisions");
    const host = screen.getByLabelText("Host");
    expect(host).toHaveAccessibleDescription(/\/v1\/systemone is added/);
    expect(host).toHaveAccessibleDescription(/http:\/\/127\.0\.0\.1:8765/);
    expect(host).toHaveAttribute("placeholder", "http://127.0.0.1:8765");
  });
});

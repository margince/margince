/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { DocumentExtractionPanel } from "./documentextraction";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const FAILED = {
  id: "11111111-1111-1111-1111-111111111111",
  question: "deal_facts",
  status: "failed",
  stalled: false,
  created_at: "2026-08-15T09:00:00Z",
  fields: [],
  omitted: [],
};

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

// A failed reading is on file; the setup read says whether a model is
// connected, and a start answers 409 ai_not_configured when asked anyway.
function serve(bound: boolean | "unlisted") {
  const posts: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(request ? request.url : String(input), "http://x");
      const method = request?.method ?? init?.method ?? "GET";
      if (url.pathname.endsWith("/installation/setup")) {
        return bound === "unlisted"
          ? json({ complete: true, steps: [] })
          : json({
              complete: bound,
              steps: [
                { step: "ai_models", configured: bound, blocking: false },
              ],
            });
      }
      if (method === "POST") {
        posts.push(url.pathname);
        return json(
          { status: 409, code: "ai_not_configured", detail: "No AI model." },
          409,
        );
      }
      return json(FAILED);
    }),
  );
  return posts;
}

function show() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <DocumentExtractionPanel attachmentId="att-1" canAccept />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("Read again with no AI connected", () => {
  it("stays visible, refuses the press and links to AI settings", async () => {
    const posts = serve(false);
    show();

    const link = await screen.findByRole("link", {
      name: englishCopy["connectAi.link"],
    });
    expect(link).toBeTruthy();
    await userEvent
      .setup()
      .click(screen.getByRole("button", { name: "Read again" }));
    expect(posts).toEqual([]);
  });

  it("shows the refusal with the link when the server refuses a press the page did not foresee", async () => {
    const posts = serve("unlisted");
    show();

    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Read again" }));
    expect(posts.length).toBe(1);
    expect(await screen.findByText(/Connect an AI first/)).toBeTruthy();
    expect(
      screen.getByRole("link", { name: englishCopy["connectAi.link"] }),
    ).toBeTruthy();
  });
});

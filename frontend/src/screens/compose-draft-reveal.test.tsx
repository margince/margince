/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { ComposeModal } from "./compose";
import {
  allowedPreview,
  isPreviewDoor,
  previewedAddresses,
} from "./sendpermission.testkit";

// WHERE a finished draft leaves the reader.
//
// The draft answers into the middle of a scrolling drawer and grows everything
// above the body as it lands: the Art. 50 band appears where the draft bar was,
// carrying the disclosure sentence, what the draft was based on and the voice
// version, and the head below it fills with a recipient and a subject. The
// words the rep pressed the button for end up under all of that — off the fold
// on any drawer shorter than the form — so the composer answered "Draft with
// AI" with a notice about a draft the rep could not see.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const DRAFT = {
  subject: "Following up on your pricing question",
  body: "Hi Dung,\n\nYou asked what this would cost for 40 seats.",
  generated_by: "model",
  ai_generated: true,
  ai_disclosure: "This message was drafted with AI assistance.",
};

const PURPOSES = {
  data: [
    {
      id: "p1",
      key: "transactional",
      label: "Deal messages",
      requires_double_opt_in: false,
      created_at: "2026-01-01T00:00:00Z",
    },
  ],
};

function stubRoutes() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request = input instanceof Request ? input : null;
      const url = new URL(
        request ? request.url : String(input),
        "https://test.local",
      );
      const method = request?.method ?? init?.method ?? "GET";
      const key = `${method} ${url.pathname.replace(/^\/v1/, "")}`;
      let body: unknown = null;
      if (method !== "GET") {
        try {
          body = request
            ? await request.clone().json()
            : JSON.parse(String(init?.body));
        } catch {
          body = null;
        }
      }
      if (key === "POST /leads/l-1/draft-email") return jsonResponse(DRAFT);
      if (key === "GET /consent-purposes") return jsonResponse(PURPOSES);
      if (key === "GET /voice-profiles") return jsonResponse({ data: [] });
      if (isPreviewDoor(url.pathname)) {
        return jsonResponse(allowedPreview(previewedAddresses(body)));
      }
      return jsonResponse({});
    }),
  );
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe("where a finished draft leaves the reader", () => {
  it("brings the drafted body into view when the disclosure band raises it", async () => {
    stubRoutes();
    render(
      <ComposeModal
        entityType="lead"
        entityId="l-1"
        recordAddress="dung.ly@newsky.example"
        open
        onClose={vi.fn()}
      />,
    );
    // jsdom has no scrollIntoView, so the composer's optional call finds
    // nothing to spy on until one is put there. The browser always has one.
    const editor = screen.getByRole("textbox", { name: "Body" });
    const reveal = vi.fn();
    editor.scrollIntoView = reveal;

    await userEvent.click(
      screen.getByRole("button", { name: "Draft with AI" }),
    );
    await screen.findByRole("heading", { name: "AI-assisted draft" });

    // The BODY is what is brought back, not the band that displaced it: the
    // band is the notice, and the words are what the press was for.
    await waitFor(() => expect(reveal).toHaveBeenCalled());
    expect(reveal).toHaveBeenCalledWith({ block: "nearest" });
    expect(editor.textContent).toContain("You asked what this would cost");
  });
});

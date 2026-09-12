/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../app/mefixture";
import { ASK_QUESTION_PARAM } from "../app/palette";
import { navigate } from "../app/router";
import { LocaleProvider } from "../i18n";
import { AskAiScreen } from "./ai";

// The palette's question travels in the ADDRESS, and this file is what that
// buys. Arrival is the property under test, and it has two shapes that a
// carrier read once per mount cannot both serve: a reader coming from another
// screen mounts this one, and a reader already standing here does not.
//
// The other half is that the address is EMPTIED once the question is taken.
// That is what stops a reload asking again for as long as the tab is open, and
// what makes the same question carried twice two asks rather than one.

const QUESTION = "how long are captured messages kept";
const SET_ID = "00000000-0000-4000-8000-0000000000a1";

const SET = {
  id: SET_ID,
  name: "How we operate",
  topic_statement: "How this product is operated, day to day.",
  min_similarity: 0.35,
  default_ask: true,
  created_at: "2026-08-01T00:00:00Z",
  coverage: { documents_total: 1, chunks_total: 4, chunks_embedded: 4 },
};

const ANSWER = {
  outcome: "answered",
  generated_by: "model",
  corpus: { id: SET_ID, name: SET.name, topic_statement: SET.topic_statement },
  coverage: SET.coverage,
  claims: [
    {
      chunk_id: "00000000-0000-4000-8000-0000000000c1",
      document_id: "00000000-0000-4000-8000-0000000000b1",
      document_name: "operating.md",
      line: 14,
      column: 3,
      text: "Captured messages are kept for 400 days.",
      quote: "kept for 400 days from the day they arrive",
    },
  ],
};

function jsonResponse(body: unknown) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

/** Every read the surface fans out to, plus the asks it made, in order. */
function backend() {
  const asked: unknown[] = [];
  const fetchMock = vi.fn(
    async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      if (request.url.endsWith("/v1/me")) {
        return jsonResponse(
          meFixture({ allow: { knowledge_corpus: ["read"] } }),
        );
      }
      if (request.url.includes("/ask")) {
        asked.push(await request.json());
        return jsonResponse(ANSWER);
      }
      if (request.url.includes("/knowledge/corpora")) {
        return jsonResponse({ items: [SET] });
      }
      throw new Error(`unexpected request: ${request.method} ${request.url}`);
    },
  );
  return { fetchMock, asked };
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

/** Land on the AI surface carrying a question, the way the palette does. */
function carry(question: string) {
  act(() => {
    navigate({ screen: "ai" }, new Map([[ASK_QUESTION_PARAM, question]]));
  });
}

beforeEach(() => {
  globalThis.location.hash = "#/ai";
});

afterEach(() => {
  cleanup();
  globalThis.location.hash = "";
  vi.unstubAllGlobals();
});

describe("AskAiScreen", () => {
  it("asks the question the address carries and hands the answer back", async () => {
    const stub = backend();
    vi.stubGlobal("fetch", stub.fetchMock);
    globalThis.location.hash = `#/ai?${ASK_QUESTION_PARAM}=${encodeURIComponent(QUESTION)}`;
    render(<AskAiScreen />);

    expect(
      await screen.findByText("Captured messages are kept for 400 days."),
    ).toBeTruthy();
    expect(stub.asked).toEqual([{ question: QUESTION }]);
    // The question is out of the address by the time the answer is on screen.
    // A reload of an address that still carried it would ask again, for as
    // long as the reader kept the tab.
    await waitFor(() => expect(globalThis.location.hash).toBe("#/ai"));
  });

  it("asks again when the same question is carried a second time", async () => {
    const stub = backend();
    vi.stubGlobal("fetch", stub.fetchMock);
    render(<AskAiScreen />);
    await screen.findByLabelText(/your question/i);

    carry(QUESTION);
    await waitFor(() => expect(stub.asked).toHaveLength(1));
    await waitFor(() => expect(globalThis.location.hash).toBe("#/ai"));

    // The identical question again, from a reader who pressed the row twice.
    // It is a second ask because the address moved a second time — which is
    // exactly what emptying it above bought.
    carry(QUESTION);
    await waitFor(() => expect(stub.asked).toHaveLength(2));
    expect(stub.asked).toEqual([
      { question: QUESTION },
      { question: QUESTION },
    ]);
  });

  it("still answers a question typed into the box", async () => {
    const stub = backend();
    vi.stubGlobal("fetch", stub.fetchMock);
    render(<AskAiScreen />);
    const user = userEvent.setup();

    const box = await screen.findByLabelText(/your question/i);
    await user.type(box, QUESTION);
    const submit = screen.getByRole("button", { name: /^ask$/i });
    await waitFor(() => expect(submit).not.toBeDisabled());
    await user.click(submit);

    expect(
      await screen.findByText("Captured messages are kept for 400 days."),
    ).toBeTruthy();
    expect(stub.asked).toEqual([{ question: QUESTION }]);
    // Nothing was carried, so nothing is written to the address either.
    expect(globalThis.location.hash).toBe("#/ai");
  });
});

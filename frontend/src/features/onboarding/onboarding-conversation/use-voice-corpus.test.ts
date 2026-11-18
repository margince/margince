/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  type ConversationEvent,
  initialConversationState,
} from "./conversation-machine";
import { useVoiceCorpus } from "./use-voice-corpus";

// ingest/classifyUpload never throw for a server refusal — that path
// narrates through the server's own safe detail and resolves normally
// (covered elsewhere). What lands in a .catch here is always a client-side
// failure that ran before the server had a chance to explain itself, so its
// raw message must never reach the reader.

const collectingState = {
  ...initialConversationState,
  act: "voice" as const,
  phase: "vo.collecting" as const,
};

afterEach(() => {
  vi.unstubAllGlobals();
});

// The hook marks the restore probe stale after a change, so it needs a client.
function wrapper({ children }: Readonly<{ children: ReactNode }>) {
  return createElement(
    QueryClientProvider,
    { client: new QueryClient() },
    children,
  );
}

describe("useVoiceCorpus — unexpected client-side failures", () => {
  it("addPaste narrates the safe fallback, never the raw exception message, when the request itself throws", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new TypeError(
          "Cannot read properties of undefined (reading 'x')",
        );
      }),
    );
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result } = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      { wrapper },
    );

    act(() => {
      result.current.addPaste("A paragraph I actually wrote.", "Pasted text");
    });

    await waitFor(() => {
      expect(
        dispatch.mock.calls.some(
          ([event]) =>
            event.type === "NARRATION" &&
            event.entry.i18nKey === "ob.conv.voice.ingestUnexpected",
        ),
      ).toBe(true);
    });
    // Never the raw exception text, under any key.
    expect(
      dispatch.mock.calls.some(
        ([event]) =>
          event.type === "NARRATION" &&
          "params" in event.entry &&
          JSON.stringify(event.entry.params ?? {}).includes(
            "Cannot read properties",
          ),
      ),
    ).toBe(false);
  });
});

describe("useVoiceCorpus — a role that may not create a voice profile", () => {
  it("says the refusal is a permission one and never suggests a retry", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const path = new URL(request.url).pathname.replace(/^\/v1/, "");
        if (path === "/voice-profiles" && request.method === "GET") {
          return Response.json({
            data: [],
            page: { next_cursor: null, has_more: false },
          });
        }
        return Response.json(
          {
            code: "permission_denied",
            detail: "voice_profile.create: permission denied",
          },
          { status: 403 },
        );
      }),
    );
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result } = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      { wrapper },
    );

    act(() => {
      result.current.addPaste("A paragraph I actually wrote.", "Pasted text");
    });

    await waitFor(() => {
      expect(result.current.failures[0]?.i18nKey).toBe(
        "common.permissionDenied",
      );
    });
    expect(
      dispatch.mock.calls.some(
        ([event]) =>
          event.type === "NARRATION" &&
          event.entry.i18nKey === "ob.conv.voice.ingestUnexpected",
      ),
    ).toBe(false);
  });
});

describe("useVoiceCorpus — several files refused in one pick", () => {
  it("forgets the refusals of the last pick when the next one starts", async () => {
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result } = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      { wrapper },
    );

    act(() => {
      result.current.addFiles([new File(["x"], "one.png")]);
    });
    expect(result.current.failures).toHaveLength(1);
    act(() => {
      result.current.addFiles([new File(["x"], "two.png")]);
    });

    expect(result.current.failures.map((f) => f.params?.name)).toEqual([
      "two.png",
    ]);
  });
});

describe("useVoiceCorpus — a file too large to send", () => {
  it("names the file and the limit instead of the server's cap phrase", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const path = new URL(request.url).pathname.replace(/^\/v1/, "");
        if (path === "/voice-profiles" && request.method === "GET") {
          return Response.json({
            data: [{ id: "vp-1" }],
            page: { next_cursor: null, has_more: false },
          });
        }
        return Response.json(
          {
            code: "body_too_large",
            detail: "request body exceeds the 1 MiB cap",
          },
          { status: 413 },
        );
      }),
    );
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result } = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      { wrapper },
    );

    act(() => {
      result.current.addFiles([new File(["many words"], "big.txt")]);
    });

    await waitFor(() => {
      expect(result.current.failures).toHaveLength(1);
    });
    expect(result.current.failures[0]).toMatchObject({
      i18nKey: "ob.conv.voice.refusalTooLarge",
      params: { name: "big.txt", limit: "1 MB" },
    });
  });
});

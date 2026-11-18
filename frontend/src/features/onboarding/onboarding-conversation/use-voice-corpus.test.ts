/** @vitest-environment happy-dom */
import { act, renderHook, waitFor } from "@testing-library/react";
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
    const { result } = renderHook(() =>
      useVoiceCorpus({ state: collectingState, dispatch }),
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
    const { result } = renderHook(() =>
      useVoiceCorpus({ state: collectingState, dispatch }),
    );

    act(() => {
      result.current.addPaste("A paragraph I actually wrote.", "Pasted text");
    });

    await waitFor(() => {
      expect(result.current.failure?.i18nKey).toBe("common.permissionDenied");
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

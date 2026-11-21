/** @vitest-environment happy-dom */
import {
  QueryClient,
  QueryClientProvider,
  useQuery,
} from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { afterEach, describe, expect, it, onTestFinished, vi } from "vitest";
import { en } from "../../../i18n/en";
import {
  type ConversationEvent,
  initialConversationState,
} from "./conversation-machine";
import { VOICE_PROBE_KEY, type VoiceRestoreProbe } from "./restore";
import { useVoiceCorpus } from "./use-voice-corpus";
import { documentStats, gate, stubApi, summaryOf } from "./voice-act.testkit";

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

// The hook writes each change into the restore probe's cache, so it needs a client.
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
      expect(result.current.failures[0]).toMatchObject({
        i18nKey: "ob.conv.voice.ingestFailed",
        params: {
          name: "Pasted text – A paragraph I actually wrote.",
          detail: en["common.permissionDenied"],
        },
      });
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

describe("useVoiceCorpus — refusals with no reason of their own", () => {
  it("names each file the server refused, whatever the cause", async () => {
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
          { code: "internal", detail: "The server failed." },
          { status: 500 },
        );
      }),
    );
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result } = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      { wrapper },
    );

    act(() => {
      result.current.addFiles([
        new File(["first words"], "first.txt"),
        new File(["second words"], "second.txt"),
      ]);
    });

    await waitFor(() => {
      expect(result.current.failures).toHaveLength(2);
    });
    expect(
      result.current.failures.map((failure) => failure.params?.name),
    ).toEqual(expect.arrayContaining(["first.txt", "second.txt"]));
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

describe("useVoiceCorpus — the restore probe the screen keeps mounted", () => {
  it("takes an added and a removed sample without reading the probe again", async () => {
    stubApi({
      ingests: [{ stats: documentStats, summary: summaryOf(900) }],
      afterRemove: summaryOf(0),
    });
    const client = new QueryClient();
    client.setQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY, {
      built: false,
      summary: null,
      sources: [],
    });
    const probe = vi.fn(async () => {
      throw new Error("the probe was read again");
    });
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result } = renderHook(
      () => {
        useQuery({
          queryKey: VOICE_PROBE_KEY,
          queryFn: probe,
          staleTime: Infinity,
        });
        return useVoiceCorpus({ state: collectingState, dispatch });
      },
      {
        wrapper: ({ children }) =>
          createElement(QueryClientProvider, { client }, children),
      },
    );
    const cached = () =>
      client.getQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY);

    act(() => {
      result.current.addPaste("A paragraph I actually wrote.", "Pasted text");
    });
    await waitFor(() => expect(cached()?.sources).toHaveLength(1));
    expect(cached()?.summary?.total_words).toBe(900);

    const [added] = result.current.manifest;
    expect(added).toBeDefined();
    if (added) {
      act(() => result.current.removeSample(added));
    }
    await waitFor(() => expect(cached()?.sources).toEqual([]));
    expect(cached()?.summary?.total_words).toBe(0);
    expect(probe).not.toHaveBeenCalled();
  });

  it("drops a removed sample from the probe even when the step closed first", async () => {
    const removal = gate();
    stubApi({
      ingests: [{ stats: documentStats, summary: summaryOf(900) }],
      afterRemove: summaryOf(0),
      removeRelease: removal.held,
    });
    const client = new QueryClient();
    client.setQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY, {
      built: false,
      summary: null,
      sources: [],
    });
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result, unmount } = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      {
        wrapper: ({ children }) =>
          createElement(QueryClientProvider, { client }, children),
      },
    );
    const cached = () =>
      client.getQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY);
    act(() => {
      result.current.addPaste("A paragraph I actually wrote.", "Pasted text");
    });
    await waitFor(() => expect(cached()?.sources).toHaveLength(1));
    const [added] = result.current.manifest;
    if (added) {
      act(() => result.current.removeSample(added));
    }

    unmount();
    removal.open();
    await waitFor(() => expect(cached()?.sources).toEqual([]));
    expect(cached()?.summary?.total_words).toBe(0);
  });

  it("takes an added sample into the probe even when the step closed first", async () => {
    const ingest = gate();
    stubApi({
      ingests: [
        {
          stats: documentStats,
          summary: summaryOf(900),
          release: ingest.held,
        },
      ],
    });
    const client = new QueryClient();
    client.setQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY, {
      built: false,
      summary: null,
      sources: [],
    });
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result, unmount } = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      {
        wrapper: ({ children }) =>
          createElement(QueryClientProvider, { client }, children),
      },
    );
    act(() => {
      result.current.addPaste("A paragraph I actually wrote.", "Pasted text");
    });

    unmount();
    ingest.open();
    const cached = () =>
      client.getQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY);
    await waitFor(() => expect(cached()?.sources).toHaveLength(1));
    expect(cached()?.summary?.total_words).toBe(900);
  });

  it("keeps the newer summary when two ingests settle out of order after the step closed", async () => {
    const older = gate();
    const newer = gate();
    stubApi({
      ingests: [
        { stats: documentStats, summary: summaryOf(900), release: older.held },
        { stats: documentStats, summary: summaryOf(1800), release: newer.held },
      ],
    });
    const client = new QueryClient();
    client.setQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY, {
      built: false,
      summary: null,
      sources: [],
    });
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result, unmount } = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      {
        wrapper: ({ children }) =>
          createElement(QueryClientProvider, { client }, children),
      },
    );
    const cached = () =>
      client.getQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY);
    act(() => {
      result.current.addPaste("The first paragraph I wrote.", "Pasted text");
    });
    act(() => {
      result.current.addPaste("A second paragraph of mine.", "Pasted text");
    });

    unmount();
    newer.open();
    await waitFor(() => expect(cached()?.sources).toHaveLength(1));
    older.open();
    await waitFor(() => expect(cached()?.sources).toHaveLength(2));
    expect(cached()?.summary?.total_words).toBe(1800);
  });

  // The clock stands still, so the probe's own read and the writes before it
  // share one timestamp: only the order of the answers may decide.
  it("takes the meter from the probe read again when the read after a removal fails", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    onTestFinished(() => {
      vi.useRealTimers();
    });
    const reread = gate();
    stubApi({
      ingests: [{ stats: documentStats, summary: summaryOf(900) }],
      afterRemoveStatus: 500,
    });
    const client = new QueryClient();
    client.setQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY, {
      built: false,
      summary: null,
      sources: [],
    });
    const probe = vi.fn(async (): Promise<VoiceRestoreProbe> => {
      await reread.held;
      return { built: false, summary: summaryOf(0), sources: [] };
    });
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result } = renderHook(
      () => {
        useQuery({
          queryKey: VOICE_PROBE_KEY,
          queryFn: probe,
          staleTime: Infinity,
        });
        return useVoiceCorpus({ state: collectingState, dispatch });
      },
      {
        wrapper: ({ children }) =>
          createElement(QueryClientProvider, { client }, children),
      },
    );
    act(() => {
      result.current.addPaste("A paragraph I actually wrote.", "Pasted text");
    });
    await waitFor(() => expect(result.current.manifest).toHaveLength(1));
    const [added] = result.current.manifest;
    if (added) {
      act(() => result.current.removeSample(added));
    }
    await waitFor(() => expect(probe).toHaveBeenCalledTimes(1));
    expect(result.current.meterStale).toBe(true);

    reread.open();
    await waitFor(() => expect(result.current.summary?.total_words).toBe(0));
    expect(result.current.busy).toBe(false);
    expect(
      client.getQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY)?.summary
        ?.total_words,
    ).toBe(0);
  });

  it("keeps the meter unknown for the step that opens again after both reads failed", async () => {
    stubApi({
      ingests: [{ stats: documentStats, summary: summaryOf(900) }],
      afterRemoveStatus: 500,
    });
    const client = new QueryClient();
    client.setQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY, {
      built: false,
      summary: null,
      sources: [],
    });
    const wrapper = ({ children }: Readonly<{ children: ReactNode }>) =>
      createElement(QueryClientProvider, { client }, children);
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const first = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      { wrapper },
    );
    act(() => {
      first.result.current.addPaste("A paragraph I actually wrote.", "Pasted");
    });
    await waitFor(() => expect(first.result.current.manifest).toHaveLength(1));
    const [added] = first.result.current.manifest;
    if (added) {
      act(() => first.result.current.removeSample(added));
    }
    await waitFor(() => expect(first.result.current.meterStale).toBe(true));
    first.unmount();

    const probe = client.getQueryData<VoiceRestoreProbe>(VOICE_PROBE_KEY);
    const again = renderHook(
      () =>
        useVoiceCorpus({
          state: collectingState,
          dispatch,
          initialSummary: probe?.summary ?? null,
          initialMeterStale: probe?.meterStale === true,
        }),
      { wrapper },
    );
    expect(again.result.current.meterStale).toBe(true);
    expect(again.result.current.busy).toBe(true);
  });

  it("lets no ingest issued before a landed removal clear the unknown meter", async () => {
    const late = gate();
    const calls = stubApi({
      ingests: [
        { stats: documentStats, summary: summaryOf(900) },
        { stats: documentStats, summary: summaryOf(1800), release: late.held },
      ],
      afterRemoveStatus: 500,
    });
    const client = new QueryClient();
    const dispatch = vi.fn<(event: ConversationEvent) => void>();
    const { result } = renderHook(
      () => useVoiceCorpus({ state: collectingState, dispatch }),
      {
        wrapper: ({ children }) =>
          createElement(QueryClientProvider, { client }, children),
      },
    );
    act(() => {
      result.current.addPaste("A paragraph I actually wrote.", "Pasted");
    });
    await waitFor(() => expect(result.current.manifest).toHaveLength(1));
    act(() => {
      result.current.addPaste("Another paragraph of mine.", "Pasted");
    });
    await waitFor(() =>
      expect(
        calls.filter(
          (call) =>
            call.method === "POST" &&
            new URL(call.url).pathname.endsWith("/sources"),
        ),
      ).toHaveLength(2),
    );
    const [added] = result.current.manifest;
    if (added) {
      act(() => result.current.removeSample(added));
    }
    await waitFor(() => expect(result.current.meterStale).toBe(true));

    late.open();
    await waitFor(() => expect(result.current.manifest).toHaveLength(1));
    expect(result.current.meterStale).toBe(true);
    expect(result.current.busy).toBe(true);
  });
});

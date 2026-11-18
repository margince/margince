import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { useReducer } from "react";
import { expect, vi } from "vitest";
import {
  ASYNC_UTIL_TIMEOUT_MS,
  SLOWEST_MEASURED_TEST_MS,
} from "../../../../vitest.budget";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import type { ConversationState } from "./conversation-machine";
import {
  conversationReducer,
  initialConversationState,
} from "./conversation-machine";
import { VoiceAct } from "./voice-act";

// The voice act's test harness. It holds wire fixtures, a fetch stub that
// answers the voice routes as the server does, and the act on the real machine.

export type CorpusPreview = components["schemas"]["VoiceCorpusPreviewResult"];
export type CorpusSummary = components["schemas"]["VoiceCorpusSummary"];
export type IngestStats = components["schemas"]["VoiceIngestStats"];

// The two build cases below narrate a poll, so each raises two of its waiters
// to 4000ms to outlast it. A test may spend the sum of its waiters' budgets,
// and the heavier case spends 12000ms. That is past the suite's own ceiling,
// which is derived for tests that wait at the default. So these tests state
// their own ceiling from the same measured allowance the suite uses.
export const POLL_WAITER_MS = 4000;
// The count a dropped file produces renders at the end of a round trip: read
// the file, POST /sources, take the summary back, re-render. The default
// async-util timeout covers a render only. Under the sharded suite that chain
// outran 1000ms, so the waiter gets the poll's allowance.
export const INGEST_WAITER_MS = POLL_WAITER_MS;
export const BUILD_TEST_MS =
  POLL_WAITER_MS * 2 + ASYNC_UTIL_TIMEOUT_MS * 4 + SLOWEST_MEASURED_TEST_MS;

export const PROFILE_ID = "018f3a1b-0000-7000-8000-0000000000d1";
export const BUILD_IDS = [
  "018f3a1b-0000-7000-8000-0000000000e1",
  "018f3a1b-0000-7000-8000-0000000000e2",
];

export function summaryOf(totalWords: number): CorpusSummary {
  return {
    total_words: totalWords,
    target_words: 30000,
    maturity: "collecting",
    quality_band: totalWords >= 800 ? "good" : "thin",
    source_count: 1,
    register_words: { spoken: totalWords },
  };
}

export const conversationalPreview: CorpusPreview = {
  detected_format: "vtt",
  total_words: 5400,
  speakers: [
    { label: "Speaker 1", turns: 12, words: 1240 },
    { label: "Speaker 2", turns: 14, words: 4160 },
  ],
  unattributed_words: 0,
  ingestible_as_transcript: true,
};

export const unattributedPreview: CorpusPreview = {
  detected_format: "vtt",
  total_words: 5400,
  speakers: [],
  unattributed_words: 5400,
  ingestible_as_transcript: false,
};

export const documentPreview: CorpusPreview = {
  detected_format: "txt",
  total_words: 900,
  speakers: [],
  unattributed_words: 900,
  ingestible_as_transcript: false,
};

export const transcriptStats: IngestStats = {
  input_words: 5400,
  kept_words: 1240,
  kept_turns: 12,
  discarded_turns: 14,
  speakers_seen: ["Speaker 1", "Speaker 2"],
};

export const documentStats: IngestStats = {
  input_words: 900,
  kept_words: 900,
  kept_turns: 1,
  discarded_turns: 0,
  speakers_seen: [],
};

// Build poll rows: only the fields the hook reads; the stub serves them as
// plain JSON, as the server would.
// What the server sends when an installation has no AI provider bound. This
// sentence separates "open a setting" from "try again". Without it a reader
// retried a build that could never succeed.
export const UNCONFIGURED_DETAIL =
  "Voice training needs an AI provider, and none is set up yet, so the build did not run. An administrator adds a provider under Settings → AI models → Providers; build again once that is done.";

export type BuildRow = {
  id: string;
  status: string;
  stage: string | null;
  /** The server's own words about a terminal outcome, optional as on the
   * wire. A build with nothing to add sends none, and the screen must read
   * correctly either way. */
  status_detail?: string;
};

export const candidateVersion = {
  profile_version: 3,
  status: "candidate",
  model_name: "test-model",
  profile_json: {
    inference: { identity_summary: "Direct, concrete, writes as I." },
  },
  stats_json: { word_count: 1240 },
};

// A version carrying every optional field parseVoiceInsights reads. The result
// scene's structure (sample card, measured dimensions, reading) is pinned
// against it, apart from the parser's own tests.
export const richVersion = {
  profile_version: 5,
  status: "active",
  model_name: "test-model",
  profile_json: {
    inference: {
      identity_summary: "Direct and warm.",
      thinking_pattern: "Leads with the ask, then the context.",
      signature_moves: [
        { move: "Opens with 'Hey'", quote: "Hey Jordan," },
        { move: "Closes with Cheers", quote: "Cheers, Alex" },
      ],
      avoid: ["Corporate jargon", "Em dashes"],
    },
    sample_drafts: [
      {
        subject: "Following up on Tuesday",
        body: "Hey Jordan, just circling back on this.",
        voice_score: 0.9,
      },
      {
        subject: "Quick nudge",
        body: "Hey Jordan, any update on your end?",
        voice_score: 0.8,
      },
    ],
    guidance: { next_best: "Add two more sent emails." },
  },
  stats_json: { word_count: 4200, mean_sentence_words: 9.5, sample_count: 4 },
};

export type IngestFixture = {
  stats: IngestStats;
  summary: CorpusSummary;
  /** Response latency, to force out-of-order settlement in tests. */
  delayMs?: number;
};

export type StubOptions = {
  preview?: CorpusPreview;
  /** Ingest responses in order: each carries its stats + resulting summary. */
  ingests?: readonly IngestFixture[];
  /** Ingest responses keyed by source_label; wins over the ordered list. */
  ingestsBySource?: Readonly<Record<string, IngestFixture>>;
  /** Poll snapshots per build id, consumed one per GET (last one repeats). */
  builds?: Readonly<Record<string, BuildRow[]>>;
  /** Error status for the build poll GET (resilience tests). */
  buildPollStatus?: number;
  /** RFC 7807 body + status the build POST is refused with, for the cases
   * about what a refused start is allowed to say on the collect scene. */
  buildStartRefusal?: Readonly<{ body: unknown; status: number }>;
  /** The built version the versions GET returns; defaults to the thin
   * candidateVersion fixture most tests never look past. */
  version?: unknown;
  /** Whether /installation/setup reports a model bound; bound by default. */
  modelReady?: boolean;
  /** The meter the sources GET reports once a sample is removed. */
  afterRemove?: CorpusSummary;
};

export function delay(ms: number): Promise<void> {
  return new Promise((resolve) => {
    globalThis.setTimeout(resolve, ms);
  });
}

export function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The stub's memory across one test: which ingest and build fixture is next,
// and the poll rows each build still has to hand out.
type StubCursor = {
  ingestIndex: number;
  buildIndex: number;
  buildPolls: Map<string, BuildRow[]>;
};

const aiProfile = {
  name: "Margince",
  kind: "ai",
  state: "configured",
  inference_mode: "cloud",
  providers: ["gemini"],
  configured_models: [
    { tier: "cheap_cloud", provider: "gemini", model: "gemini-3.5-flash" },
  ],
};

async function answerIngest(
  request: Request,
  options: StubOptions,
  cursor: StubCursor,
): Promise<Response> {
  const body = (await request.clone().json()) as Record<string, unknown>;
  const label = typeof body.source_label === "string" ? body.source_label : "";
  const ingest =
    options.ingestsBySource?.[label] ??
    (options.ingests ?? [])[cursor.ingestIndex];
  if (!ingest) {
    throw new Error("unexpected ingest: no fixture left");
  }
  cursor.ingestIndex += 1;
  if (ingest.delayMs !== undefined) {
    await delay(ingest.delayMs);
  }
  return jsonResponse(
    {
      source: { id: `source-${cursor.ingestIndex}`, version: 1 },
      summary: ingest.summary,
      ingest_stats: ingest.stats,
    },
    201,
  );
}

function answerBuildStart(options: StubOptions, cursor: StubCursor): Response {
  const refusal = options.buildStartRefusal;
  if (refusal !== undefined) {
    return jsonResponse(refusal.body, refusal.status);
  }
  const id = BUILD_IDS[cursor.buildIndex];
  cursor.buildIndex += 1;
  return jsonResponse({ id, status: "queued", stage: null }, 202);
}

function answerBuildPoll(
  path: string,
  options: StubOptions,
  cursor: StubCursor,
): Response {
  if (options.buildPollStatus !== undefined) {
    return jsonResponse(
      { detail: "build fetch failed" },
      options.buildPollStatus,
    );
  }
  const buildId = path.slice(path.lastIndexOf("/") + 1);
  const polls = cursor.buildPolls.get(buildId) ?? [];
  const row = polls.length > 1 ? polls.shift() : polls[0];
  if (!row) {
    throw new Error(`unstubbed build poll: ${buildId}`);
  }
  return jsonResponse(row);
}

function answerRead(path: string, options: StubOptions): Response | null {
  if (path.endsWith("/ai/profile")) {
    return jsonResponse(aiProfile);
  }
  if (path.endsWith("/installation/setup")) {
    const configured = options.modelReady ?? true;
    return jsonResponse({
      complete: configured,
      steps: [{ step: "ai_models", configured, blocking: true }],
    });
  }
  if (path.endsWith("/voice-profiles")) {
    return jsonResponse({
      data: [{ id: PROFILE_ID }],
      page: { next_cursor: null },
    });
  }
  if (path.endsWith("/versions")) {
    return jsonResponse({
      data: [options.version ?? candidateVersion],
      page: { next_cursor: null },
    });
  }
  return null;
}

async function answer(
  request: Request,
  options: StubOptions,
  cursor: StubCursor,
): Promise<Response> {
  const path = new URL(request.url).pathname;
  if (path.endsWith("/sources/preview")) {
    return jsonResponse(options.preview ?? documentPreview);
  }
  if (request.method === "GET" && path.includes("/builds/")) {
    return answerBuildPoll(path, options, cursor);
  }
  const read = request.method === "GET" ? answerRead(path, options) : null;
  if (read !== null) {
    return read;
  }
  if (request.method === "POST" && path.endsWith("/sources")) {
    return answerIngest(request, options, cursor);
  }
  if (request.method === "DELETE" && path.includes("/sources/")) {
    return jsonResponse({ id: path.slice(path.lastIndexOf("/") + 1) });
  }
  if (request.method === "GET" && path.endsWith("/sources")) {
    const summary = options.afterRemove ?? summaryOf(0);
    return jsonResponse({ data: [], summary, page: { next_cursor: null } });
  }
  if (request.method === "POST" && path.endsWith("/builds")) {
    return answerBuildStart(options, cursor);
  }
  throw new Error(`unstubbed request: ${request.method} ${request.url}`);
}

export function stubApi(options: StubOptions = {}) {
  const calls: Request[] = [];
  const cursor: StubCursor = {
    ingestIndex: 0,
    buildIndex: 0,
    buildPolls: new Map(Object.entries(options.builds ?? {})),
  };
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      calls.push(request);
      return answer(request, options, cursor);
    }),
  );
  return calls;
}

export function collectingState(): ConversationState {
  return { ...initialConversationState, act: "voice", phase: "vo.collecting" };
}

export function VoiceHarness({
  initial,
}: Readonly<{ initial: ConversationState }>) {
  const [state, dispatch] = useReducer(conversationReducer, initial);
  return <VoiceAct state={state} dispatch={dispatch} />;
}

export function render(ui: ReactNode) {
  return rtlRender(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

export async function uploadFile(name: string, content: string) {
  const input = document.querySelector<HTMLInputElement>('input[type="file"]');
  expect(input).not.toBeNull();
  if (input) {
    await userEvent.upload(
      input,
      new File([content], name, { type: "text/plain" }),
    );
  }
}

export const testdata = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "..",
  "testing",
  "testdata",
);

// A drop on the window itself, where the hint promises one lands anywhere.
export function dropOnWindow(files: File[]) {
  const drop = new Event("drop", { bubbles: true, cancelable: true });
  Object.defineProperty(drop, "dataTransfer", {
    value: { types: ["Files"], files },
  });
  window.dispatchEvent(drop);
}

// Path-suffix matching so "/sources" never also counts "/sources/preview".
export function requestsTo(calls: Request[], path: string, method: string) {
  return calls.filter(
    (request) =>
      new URL(request.url).pathname.endsWith(path) && request.method === method,
  );
}

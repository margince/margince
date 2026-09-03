/** @vitest-environment jsdom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { OnboardingScreen } from "../onboarding";

// The restore matrix of the conversational shell: which act a reload lands
// in, that the landing is derived from the wizard state's `path` and `step`
// (never from company-exists alone), that recap turns are derived summaries
// rather than replayed narration, and that finishing connect writes the
// completion BEFORE any navigation.

type OnboardingState = components["schemas"]["OnboardingState"];
type CompanySiteRead = components["schemas"]["CompanySiteRead"];
type Proposal = components["schemas"]["OnboardingCompanyProposal"];

const READ_ID = "018f3a1b-0000-7000-8000-0000000000c3";

function readRow(
  status: CompanySiteRead["status"],
  pages = 12,
): CompanySiteRead {
  return {
    id: READ_ID,
    target_kind: "onboarding",
    organization_id: null,
    root_url: "https://gradion.com",
    status,
    status_code: null,
    status_detail: null,
    next_attempt_at: null,
    phase: status === "reading" ? "crawling" : null,
    pages_read: pages,
    pages: [],
    profile_fields: [
      {
        field: "legal_name",
        value: "Gradion GmbH",
        evidence_snippet: "© 2026 Gradion GmbH",
        source_kind: "url",
        source_url: "https://gradion.com",
        confidence: 0.9,
      },
      {
        field: "display_name",
        value: "Gradion",
        evidence_snippet: "Gradion",
        source_kind: "url",
        source_url: "https://gradion.com",
        confidence: 0.9,
      },
    ],
    facts: [],
    comparisons: [],
    people: [],
    legal_entities: [],
    warnings: [],
    draft_version: 2,
    proposal_hash: "proposal-2",
    created_at: "2026-07-22T08:00:00Z",
    updated_at: "2026-07-22T08:10:00Z",
  };
}

function proposalFor(read: CompanySiteRead): Proposal {
  return {
    ready: true,
    fields: read.profile_fields.map((field) => ({
      field: field.field,
      value: field.value,
      confidence: field.confidence,
      evidence_snippet: field.evidence_snippet,
      source_url: field.source_url ?? "https://gradion.com",
    })),
    facts: [],
    open_questions: [],
    remaining_required_fields: [],
    draft_version: read.draft_version,
    proposal_hash: read.proposal_hash,
  };
}

const savedProfile = {
  organization_id: "018f3a1b-0000-7000-8000-0000000000a1",
  display_name: "Gradion",
  website: "gradion.com",
  offer_summary: "Revenue software for manufacturers",
  icp: "Mid-market manufacturers",
};

function stateRow(overrides: Partial<OnboardingState> = {}): OnboardingState {
  return {
    path: "creator",
    step: "read",
    source_mode: "website",
    website_url: "https://gradion.com",
    site_read_id: null,
    company_draft: {},
    selected_fact_keys: [],
    voice_skipped: false,
    connect_skipped: false,
    version: 3,
    completed_at: null,
    created_at: "2026-07-22T08:00:00Z",
    updated_at: "2026-07-22T09:00:00Z",
    ...overrides,
  };
}

type StubOptions = {
  /** GET /onboarding/state; null answers 404 (nothing persisted). */
  state?: OnboardingState | null;
  /** GET /company; null answers 404 (no company confirmed yet). */
  company?: typeof savedProfile | null;
  /** GET /voice-profiles items (the restore probe's first hop). */
  voiceProfiles?: { id: string }[];
  voiceVersions?: { profile_version: number; status: string }[];
  corpusWords?: number;
  /** Words a live paste ingest (POST .../sources) adds on top of
   * `corpusWords`; every GET afterward reports the grown total. */
  pasteWords?: number;
  /** Mutable: set to make PUT /onboarding/state fail with this status. */
  putStatus?: number;
  /** GET /company/site-reads/{id} snapshots, served in order (last one
   * repeats): the restore fetch first, then the resumed poll. */
  reads?: CompanySiteRead[];
  proposal?: Proposal;
};

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubApi(options: StubOptions = {}) {
  const calls: Request[] = [];
  let version = options.state?.version ?? 0;
  let readPoll = 0;
  let wordsNow = options.corpusWords ?? 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      calls.push(request);
      const path = new URL(request.url).pathname;
      if (path.endsWith("/ai/profile")) {
        return jsonResponse({
          name: "Margince",
          kind: "ai",
          state: "configured",
          inference_mode: "cloud",
          providers: ["gemini"],
          configured_models: [],
        });
      }
      if (path.endsWith("/company/context/capabilities")) {
        return jsonResponse({
          onboarding_enabled: true,
          read_enabled: true,
          rollout: "ga",
        });
      }
      if (path.endsWith("/onboarding/state") && request.method === "GET") {
        const row = options.state ?? null;
        return row === null
          ? jsonResponse({ detail: "not started" }, 404)
          : jsonResponse({ ...row, version });
      }
      if (path.endsWith("/onboarding/state") && request.method === "PUT") {
        if (options.putStatus !== undefined) {
          return jsonResponse({ detail: "write failed" }, options.putStatus);
        }
        const body = (await request.clone().json()) as Record<string, unknown>;
        version += 1;
        return jsonResponse({
          ...body,
          path: options.state?.path ?? "creator",
          version,
          completed_at: null,
          created_at: "2026-07-22T08:00:00Z",
          updated_at: "2026-07-22T09:01:00Z",
        });
      }
      if (path.includes("/company/site-reads/") && request.method === "GET") {
        const reads = options.reads ?? [];
        const snapshot = reads[Math.min(readPoll, reads.length - 1)];
        readPoll += 1;
        return snapshot === undefined
          ? jsonResponse({ detail: "read not found" }, 404)
          : jsonResponse(snapshot);
      }
      if (path.endsWith("/onboarding/company/proposal")) {
        return jsonResponse(
          options.proposal ?? proposalFor(readRow("partial")),
        );
      }
      if (
        path.endsWith("/onboarding/company/messages") &&
        request.method === "POST"
      ) {
        const body = (await request.clone().json()) as {
          selected_option?: { field: string; value: string };
        };
        // The authorization round trip: exactly the selected field+value
        // comes back as the confirmed change.
        return jsonResponse({
          kind: "clarification",
          act: "company",
          message: "Recorded.",
          proposed_changes: body.selected_option
            ? [{ ...body.selected_option, reason: "You chose this." }]
            : [],
          citations: [],
          remaining_required_fields: [],
          available_action: "confirm_company",
          ai_runtime: {
            currency: "USD",
            call_attempts: 1,
            tokens_in: 100,
            tokens_out: 20,
            latency_ms: 500,
            estimated_cost_microusd: 0,
            unpriced_calls: 0,
            models: [],
          },
        });
      }
      if (path.endsWith("/company") && request.method === "GET") {
        return options.company
          ? jsonResponse(options.company)
          : jsonResponse({ detail: "no company yet" }, 404);
      }
      if (path.endsWith("/voice-profiles") && request.method === "GET") {
        return jsonResponse({ data: options.voiceProfiles ?? [], page: {} });
      }
      if (path.includes("/voice-profiles/") && path.endsWith("/versions")) {
        return jsonResponse({ data: options.voiceVersions ?? [], page: {} });
      }
      // Every source is previewed before it is written, pasted text included:
      // the server is the one that says whether writing carries speakers.
      if (path.endsWith("/sources/preview")) {
        return jsonResponse({
          detected_format: "txt",
          total_words: options.pasteWords ?? 0,
          speakers: [],
          unattributed_words: options.pasteWords ?? 0,
          ingestible_as_transcript: false,
        });
      }
      if (
        path.includes("/voice-profiles/") &&
        path.endsWith("/sources") &&
        request.method === "POST"
      ) {
        const added = options.pasteWords ?? 0;
        wordsNow += added;
        return jsonResponse({
          ingest_stats: { kept_words: added, input_words: added },
          summary: {
            total_words: wordsNow,
            target_words: 30000,
            maturity: "collecting",
            quality_band: "thin",
            source_count: 1,
            register_words: {},
          },
        });
      }
      if (path.includes("/voice-profiles/") && path.endsWith("/sources")) {
        return jsonResponse({
          data: [],
          summary: {
            total_words: wordsNow,
            target_words: 30000,
            maturity: "collecting",
            quality_band: "thin",
            source_count: wordsNow > 0 ? 1 : 0,
            register_words: {},
          },
          page: {},
        });
      }
      // The connect scene's own cards read the roster fresh, so a reload
      // that lands on that step always fires this — none of these fixtures
      // arrive with a mailbox already connected.
      if (path.endsWith("/connectors") && request.method === "GET") {
        return jsonResponse({ data: [] });
      }
      throw new Error(`unstubbed request: ${request.method} ${request.url}`);
    }),
  );
  return calls;
}

function render(ui: ReactNode) {
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

function requestsTo(calls: Request[], path: string, method: string) {
  return calls.filter(
    (request) => request.url.includes(path) && request.method === method,
  );
}

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

describe("restore into the conversational shell", () => {
  it("a fresh creator starts at the company welcome, no recap", async () => {
    stubApi();
    render(<OnboardingScreen />);

    expect(await screen.findByLabelText(/Your website address/)).toBeTruthy();
    expect(screen.queryByText(/Welcome back/)).toBeNull();
  });

  it("a returning creator at step voice resumes the voice act, not an invite step", async () => {
    stubApi({
      state: stateRow({ step: "voice" }),
      company: savedProfile,
    });
    render(<OnboardingScreen />);

    // The saved company must NOT demote the creator to the member path (the
    // old proxy skipped the voice act for exactly this session), and the
    // restore lands directly on the collect scene. The recap that used to
    // say so in the transcript ("Welcome back...", "Your company profile
    // for Gradion is confirmed.") is gone along with the transcript itself
    // — this heading is what proves the restore landed correctly now.
    expect(await screen.findByText(/Teach me how you write\./)).toBeTruthy();
  });

  it("a corpus already on the server resumes collecting", async () => {
    stubApi({
      state: stateRow({ step: "voice" }),
      company: savedProfile,
      voiceProfiles: [{ id: "018f3a1b-0000-7000-8000-0000000000f1" }],
      corpusWords: 1240,
    });
    render(<OnboardingScreen />);

    // The recap that used to name the honest word count in the transcript
    // ("Your corpus already holds 1,240...") is gone along with the
    // transcript; the collect scene's own meter is where that count lives
    // now, and it reads the server's real total rather than starting at 0.
    expect(
      await screen.findByText("1,240 words", { exact: false }),
    ).toBeTruthy();
  });

  it("the member path comes from the state row and skips voice and results entirely", async () => {
    const calls = stubApi({
      state: stateRow({ path: "member", step: "connect" }),
      company: savedProfile,
    });
    render(<OnboardingScreen />);

    expect(await screen.findByText("Connect your accounts.")).toBeTruthy();
    expect(screen.getByRole("button", { name: /Google/ })).toBeTruthy();
    // Microsoft is a live OAuth path now — the chip opens the same connect
    // panel Google does, no "Soon" placeholder. It starts disabled until the
    // roster fetch verifies nothing is connected yet, same as every mail
    // provider card.
    const microsoft = screen.getByRole("button", { name: /Microsoft/ });
    await waitFor(() => expect(microsoft).not.toBeDisabled());
    // A member restore never probes the voice surface.
    expect(requestsTo(calls, "/voice-profiles", "GET").length).toBe(0);
  });

  it("reopens the invite for a creator whose company is confirmed", async () => {
    const calls = stubApi({
      state: stateRow({ step: "invite" }),
      company: savedProfile,
    });
    render(<OnboardingScreen />);

    expect(
      await screen.findByRole("heading", {
        name: "Will you be working in Margince yourself?",
      }),
    ).toBeTruthy();
    // Both answers are on the page, and reopening the question records
    // nothing: the row already says "invite".
    expect(
      screen.getByRole("radio", { name: /Yes, I'll work in Margince/ }),
    ).toBeTruthy();
    expect(
      screen.getByRole("radio", { name: /No, I'm only setting it up/ }),
    ).toBeTruthy();
    expect(requestsTo(calls, "/onboarding/state", "PUT").length).toBe(0);
  });

  it("accepting the invite opens the voice act and checkpoints step voice", async () => {
    const calls = stubApi({
      state: stateRow({ step: "invite" }),
      company: savedProfile,
    });
    render(<OnboardingScreen />);

    await userEvent.click(
      await screen.findByRole("radio", { name: /Yes, I'll work in Margince/ }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Continue" }));

    await waitFor(() => {
      expect(requestsTo(calls, "/onboarding/state", "PUT").length).toBe(1);
    });
    const body = (await requestsTo(calls, "/onboarding/state", "PUT")[0]
      .clone()
      .json()) as Record<string, unknown>;
    expect(body.step).toBe("voice");
    expect(body.voice_skipped).toBe(false);
  });

  // Declining opens the team act and records both personal steps as skipped
  // on the way in, so a reload lands on the invite form and never reopens the
  // question.
  it("declining the invite opens the team act with voice and connect recorded as skipped", async () => {
    const calls = stubApi({
      state: stateRow({ step: "invite" }),
      company: savedProfile,
    });
    render(<OnboardingScreen />);

    await userEvent.click(
      await screen.findByRole("radio", { name: /No, I'm only setting it up/ }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Continue" }));

    expect(await screen.findByText("Invite the first user.")).toBeTruthy();
    await waitFor(() => {
      expect(requestsTo(calls, "/onboarding/state", "PUT").length).toBe(1);
    });
    const body = (await requestsTo(calls, "/onboarding/state", "PUT")[0]
      .clone()
      .json()) as Record<string, unknown>;
    expect(body.step).toBe("team");
    expect(body.voice_skipped).toBe(true);
    expect(body.connect_skipped).toBe(true);
  });

  // Leaving the team act is a FINISH: the row goes to "complete" before the
  // handoff, so a reload after the write lands on the app rather than back in
  // the journey.
  it("skipping the team act completes setup before the handoff", async () => {
    const calls = stubApi({
      state: stateRow({
        step: "team",
        voice_skipped: true,
        connect_skipped: true,
      }),
      company: savedProfile,
    });
    render(<OnboardingScreen />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Skip for now" }),
    );

    await waitFor(() => {
      expect(requestsTo(calls, "/onboarding/state", "PUT").length).toBe(1);
    });
    const body = (await requestsTo(calls, "/onboarding/state", "PUT")[0]
      .clone()
      .json()) as Record<string, unknown>;
    expect(body.step).toBe("complete");
    // The handoff scene has the surface.
    await waitFor(() =>
      expect(screen.queryByText("Invite the first user.")).toBeNull(),
    );
  });

  // A row written before the invite existed, parked at the recap that used to
  // follow the voice act: it lands on connect, where the recap led.
  it("lands a legacy results row on the merged connect screen", async () => {
    stubApi({
      state: stateRow({ step: "results", voice_skipped: true }),
      company: savedProfile,
    });
    render(<OnboardingScreen />);

    expect(await screen.findByText("Connect your accounts.")).toBeTruthy();
  });

  // Leaving the voice act lands directly on the merged connect screen — mail
  // and LinkedIn together, no separate network-ask act to pass through — and
  // checkpoints step "connect" immediately: arriving here already shows
  // everything, so there is nothing a reload could strand behind it.
  it("continuing out of the voice act lands on the merged connect screen and checkpoints it on arrival", async () => {
    const calls = stubApi({
      state: stateRow({ step: "voice", voice_skipped: true }),
      company: savedProfile,
    });
    render(<OnboardingScreen />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Continue" }),
    );

    expect(await screen.findByText("Connect your accounts.")).toBeTruthy();
    // The LinkedIn card is on this same screen, unopened until asked for.
    expect(
      screen.queryByRole("button", { name: "Skip LinkedIn for now" }),
    ).toBeNull();
    await waitFor(() => {
      expect(requestsTo(calls, "/onboarding/state", "PUT").length).toBe(1);
    });
    const body = (await requestsTo(calls, "/onboarding/state", "PUT")[0]
      .clone()
      .json()) as Record<string, unknown>;
    expect(body.step).toBe("connect");
  });

  it("skipping LinkedIn on the merged screen records no further checkpoint — the arrival already did", async () => {
    const calls = stubApi({
      state: stateRow({ step: "voice", voice_skipped: true }),
      company: savedProfile,
    });
    render(<OnboardingScreen />);

    await userEvent.click(
      await screen.findByRole("button", { name: "Continue" }),
    );
    await waitFor(() => {
      expect(requestsTo(calls, "/onboarding/state", "PUT").length).toBe(1);
    });

    await userEvent.click(screen.getByRole("button", { name: /LinkedIn/ }));
    await userEvent.click(
      await screen.findByRole("button", { name: "Skip LinkedIn for now" }),
    );

    expect(
      await screen.findByText("Skipped: add it later in Settings"),
    ).toBeTruthy();
    // LinkedIn's own resolution never writes wizard state; only mail does.
    expect(requestsTo(calls, "/onboarding/state", "PUT").length).toBe(1);
  });

  it("a completed journey navigates straight into the workspace", async () => {
    stubApi({ state: stateRow({ step: "complete" }), company: savedProfile });
    render(<OnboardingScreen />);

    await waitFor(() => {
      expect(window.location.hash).toBe("#/home");
    });
  });
});

describe("reload adoption of a persisted read", () => {
  it("a reload after the terminal lands straight in the review, without replaying narration", async () => {
    stubApi({
      state: stateRow({ step: "confirm", site_read_id: READ_ID }),
      reads: [readRow("partial", 40)],
    });
    render(<OnboardingScreen />);

    // The terminal outcome and the review arrive through the normal
    // conclude path; the per-field narration ("Welcome back, I finished
    // reading...", "Learned...") is recap, and the transcript it used to
    // narrate into no longer renders at all — the deck's own heading is
    // what proves the reload landed on the review.
    expect(
      await screen.findByRole("button", { name: "Confirm the profile" }),
    ).toBeTruthy();
  });

  it("a reload after the terminal still asks the proposal's open question first", async () => {
    const partial = readRow("partial", 40);
    stubApi({
      state: stateRow({ step: "confirm", site_read_id: READ_ID }),
      reads: [partial],
      proposal: {
        ...proposalFor(partial),
        open_questions: [
          {
            id: "clarify:legal_name:2",
            question: "Which legal entity is this installation for?",
            field: "legal_name",
            options: [
              {
                value: "Gradion GmbH",
                label: "Gradion GmbH",
                evidence_url: "https://gradion.com/impressum",
                evidence_snippet: "Gradion GmbH, Berlin",
                detail: null,
              },
              {
                value: "Gradion Holding GmbH",
                label: "Gradion Holding GmbH",
                evidence_url: "https://gradion.com/impressum",
                evidence_snippet: "Gradion Holding GmbH, Berlin",
                detail: null,
              },
            ],
            allow_free_text: false,
          },
        ],
      },
    });
    render(<OnboardingScreen />);

    expect(
      await screen.findByRole("heading", {
        name: /Which legal entity is this installation for\?/,
      }),
    ).toBeTruthy();

    // The decision scene: choose, then confirm.
    await userEvent.click(
      screen.getByRole("radio", { name: /Gradion Holding GmbH/ }),
    );
    await userEvent.click(screen.getByRole("button", { name: "Continue" }));

    expect(
      await screen.findByRole("button", { name: "Confirm the profile" }),
    ).toBeTruthy();
  });

  it("a reload mid-crawl resumes polling into the review", async () => {
    stubApi({
      state: stateRow({ step: "read", site_read_id: READ_ID }),
      reads: [
        readRow("reading", 12),
        readRow("reading", 30),
        readRow("partial", 40),
      ],
    });
    render(<OnboardingScreen />);

    // "Welcome back. I am still reading..." was the recap's own narration
    // of the resume; the transcript it used to appear in is gone, and the
    // review actually landing (through three straight snapshots) is the
    // surviving proof the poll picked back up rather than starting cold.
    expect(
      await screen.findByRole(
        "button",
        { name: "Confirm the profile" },
        {
          timeout: 8000,
        },
      ),
    ).toBeTruthy();
  }, 20000);

  it("a failed read reopens fresh with an honest line", async () => {
    stubApi({
      state: stateRow({ step: "read", site_read_id: READ_ID }),
      reads: [readRow("failed")],
    });
    render(<OnboardingScreen />);

    expect(
      await screen.findByText(/My earlier read of gradion\.com did not finish/),
    ).toBeTruthy();
    expect(
      await screen.findByRole("textbox", { name: /Your website address/ }),
    ).toBeTruthy();
    expect(screen.queryByText(/Continue/)).toBeNull();
  });
});

describe("finishing the connect act", () => {
  it("persists completion BEFORE navigating; a failed write is narrated and retryable", async () => {
    // The handoff plays a build scene before it navigates. This case is about
    // the ordering — completion recorded first, navigation only after — so it
    // takes the reduced-motion path, where the scene resolves on its first
    // commit. The scene's own timing is pinned in onboarding-build-scene.test.
    vi.stubGlobal("matchMedia", (query: string) => ({
      matches: query.includes("prefers-reduced-motion"),
      media: query,
      onchange: null,
      addEventListener: () => undefined,
      removeEventListener: () => undefined,
      addListener: () => undefined,
      removeListener: () => undefined,
      dispatchEvent: () => false,
    }));
    const options: StubOptions = {
      state: stateRow({ path: "member", step: "connect" }),
      company: savedProfile,
      putStatus: 500,
    };
    const calls = stubApi(options);
    render(<OnboardingScreen />);

    await userEvent.click(
      await screen.findByRole("button", { name: /Skip connecting for now/ }),
    );

    // The write failed: the failure is said out loud, nothing navigated.
    expect(
      await screen.findByText(/I could not record the finish\. Try again\./),
    ).toBeTruthy();
    expect(window.location.hash).toBe("");

    // The retry succeeds: completion lands, THEN the shell navigates.
    options.putStatus = undefined;
    await userEvent.click(
      screen.getByRole("button", { name: /Skip connecting for now/ }),
    );
    await waitFor(() => {
      expect(window.location.hash).toBe("#/home");
    });
    const writes = requestsTo(calls, "/onboarding/state", "PUT");
    expect(writes.length).toBeGreaterThan(0);
    const body = (await writes[writes.length - 1].clone().json()) as Record<
      string,
      unknown
    >;
    expect(body.step).toBe("complete");
    expect(body.connect_skipped).toBe(true);
    // The finish never rewrites the voice outcome recorded earlier.
    expect(body.voice_skipped).toBe(false);
  });
});

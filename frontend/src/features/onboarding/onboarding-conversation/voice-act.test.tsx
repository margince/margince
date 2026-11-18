/** @vitest-environment happy-dom */
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en } from "../../../i18n/en";
import {
  conversationReducer,
  initialConversationState,
} from "./conversation-machine";
import { run } from "./test-fixtures";
import {
  BUILD_IDS,
  BUILD_TEST_MS,
  collectingState,
  conversationalPreview,
  documentPreview,
  documentStats,
  dropOnWindow,
  INGEST_WAITER_MS,
  render,
  requestsTo,
  richVersion,
  stubApi,
  summaryOf,
  testdata,
  transcriptStats,
  UNCONFIGURED_DETAIL,
  unattributedPreview,
  uploadFile,
  VoiceHarness,
} from "./voice-act.testkit";

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the conversational voice act", () => {
  it("asks the speaker question for a conversational file and ingests with the chosen speaker_label", async () => {
    const calls = stubApi({
      preview: conversationalPreview,
      ingests: [{ stats: transcriptStats, summary: summaryOf(1240) }],
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("call.vtt", "WEBVTT transcript content");

    // The decision is the whole work surface, not a rail card: the question
    // and its server-derived speaker options render there.
    expect(
      await screen.findByText(/Which one is you\? Only your own words count/),
    ).toBeTruthy();
    expect(screen.getByText("words: 1,240 · turns: 12")).toBeTruthy();
    expect(screen.getByText("words: 4,160 · turns: 14")).toBeTruthy();

    await userEvent.click(screen.getByRole("radio", { name: /Speaker 1/ }));
    await userEvent.click(
      screen.getByRole("button", { name: "Use this speaker" }),
    );

    await waitFor(() => {
      expect(requestsTo(calls, "/sources", "POST").length).toBe(1);
    });
    const body = (await requestsTo(calls, "/sources", "POST")[0]
      .clone()
      .json()) as Record<string, unknown>;
    expect(body.format).toBe("transcript");
    expect(body.speaker_label).toBe("Speaker 1");
    expect(body.register).toBe("spoken");

    // The server's kept-of-total stats land on the collect scene's own
    // sources list, once — never a second copy of the same fact in the rail.
    // Both figures are word COUNTS, so they carry the reader's own grouping
    // (en-GB here) rather than a bare digit run.
    expect(await screen.findByText(/Kept 1,240 of 5,400 words/)).toBeTruthy();
  });

  it("ingests a document directly and reacts with the server word count", async () => {
    const calls = stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(900) }],
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("notes.md", "Plain prose I wrote myself.");

    // The server's word count lands on the collect scene's own sources
    // list, not as a second bubble in the rail.
    expect(
      await screen.findByText("900 words", {}, { timeout: INGEST_WAITER_MS }),
    ).toBeTruthy();
    const body = (await requestsTo(calls, "/sources", "POST")[0]
      .clone()
      .json()) as Record<string, unknown>;
    expect(body.format).toBe("text");
    expect(body.speaker_label).toBeNull();
    // No speaker question was ever asked for single-author prose.
    expect(screen.queryByText(/Which one is you/)).toBeNull();
  });

  it("accepts a drop anywhere in the window — composer, artifact, gaps included — and neutralizes the browser default", async () => {
    const calls = stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(900) }],
    });
    render(<VoiceHarness initial={collectingState()} />);

    // The drop lands on window, NOT on the thread div: the hint promises
    // "anywhere in this conversation", and an unhandled drop would navigate
    // the browser to the file, destroying the onboarding session.
    const drop = new Event("drop", { bubbles: true, cancelable: true });
    Object.defineProperty(drop, "dataTransfer", {
      value: {
        types: ["Files"],
        files: [
          new File(["Plain prose I wrote myself."], "notes.md", {
            type: "text/plain",
          }),
        ],
      },
    });
    window.dispatchEvent(drop);

    expect(drop.defaultPrevented).toBe(true);
    expect(
      await screen.findByText("900 words", {}, { timeout: INGEST_WAITER_MS }),
    ).toBeTruthy();
    expect(requestsTo(calls, "/sources", "POST").length).toBe(1);

    // A text-selection drag is NOT claimed: the composer's native
    // drag-to-insert must keep working.
    const textDrag = new Event("drop", { bubbles: true, cancelable: true });
    Object.defineProperty(textDrag, "dataTransfer", {
      value: { types: ["text/plain"], files: [] },
    });
    window.dispatchEvent(textDrag);
    expect(textDrag.defaultPrevented).toBe(false);
  });

  it("neutralizes a stray drop outside the collecting phases without ingesting", () => {
    const calls = stubApi({});
    render(
      <VoiceHarness
        initial={{
          ...initialConversationState,
          act: "voice",
          phase: "vo.skipped",
        }}
      />,
    );

    const drop = new Event("drop", { bubbles: true, cancelable: true });
    Object.defineProperty(drop, "dataTransfer", {
      value: { types: ["Files"], files: [new File(["text"], "stray.txt")] },
    });
    window.dispatchEvent(drop);

    expect(drop.defaultPrevented).toBe(true);
    expect(requestsTo(calls, "/sources", "POST").length).toBe(0);
  });

  // The board is the only surface left: with no rail thread, a drop that went
  // nowhere has to be answered where the reader is looking, and the answer
  // has to say which formats WOULD have worked.
  it("names a dropped file it cannot read on the board, with the formats it takes, and ingests nothing", async () => {
    const calls = stubApi({});
    render(<VoiceHarness initial={collectingState()} />);

    dropOnWindow([new File(["binary"], "photo.png", { type: "image/png" })]);

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe(
      "I cannot read photo.png. Supported formats: .txt, .md, .pdf, .docx, .vtt, .srt, .json.",
    );
    expect(requestsTo(calls, "/sources/preview", "POST").length).toBe(0);
    expect(requestsTo(calls, "/sources", "POST").length).toBe(0);
  });

  it("takes a dropped PDF in as its text, on the same wire a text file uses", async () => {
    const calls = stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(900) }],
    });
    render(<VoiceHarness initial={collectingState()} />);

    dropOnWindow([
      new File([readFileSync(join(testdata, "proposal.pdf"))], "proposal.pdf", {
        type: "application/pdf",
      }),
    ]);

    expect(
      await screen.findByText("900 words", {}, { timeout: INGEST_WAITER_MS }),
    ).toBeTruthy();
    const ingest = requestsTo(calls, "/sources", "POST");
    expect(ingest.length).toBe(1);
    const body = (await ingest[0].clone().json()) as Record<string, unknown>;
    expect(body.format).toBe("text");
    expect(body.source_label).toBe("proposal.pdf");
    expect(body.content).toContain("Here is the proposal we discussed.");
  });

  it("refuses an unattributed transcript honestly and counts nothing", async () => {
    const calls = stubApi({ preview: unattributedPreview });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("raw.vtt", "unlabelled transcript text");

    expect(
      await screen.findByText(
        /I cannot tell which words are yours, so none were counted/,
      ),
    ).toBeTruthy();
    expect(requestsTo(calls, "/sources", "POST").length).toBe(0);
    expect(screen.queryByText(/words kept/)).toBeNull();
  });

  it("says before any sample that no AI is connected, and holds the build", async () => {
    const calls = stubApi({
      modelReady: false,
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(820) }],
    });
    render(<VoiceHarness initial={collectingState()} />);

    expect(
      await screen.findByRole("link", { name: en["connectAi.link"] }),
    ).toBeTruthy();
    await uploadFile("one.md", "First document.");
    expect(
      await screen.findAllByText(
        /820 words/,
        {},
        { timeout: INGEST_WAITER_MS },
      ),
    ).not.toHaveLength(0);
    // Samples are still taken; only the build waits for a provider.
    await userEvent.click(
      screen.getByRole("button", { name: /Build my voice profile/ }),
    );
    expect((await screen.findByRole("alert")).textContent).toContain(
      en["connectAi.first"],
    );
    expect(requestsTo(calls, "/builds", "POST")).toHaveLength(0);
    expect(
      screen.getByRole("button", { name: en["ob.conv.voice.skipped"] }),
    ).toHaveProperty("disabled", false);
  });

  // The scene shows the build action from the start and states its
  // precondition beside it: an affordance that appears out of nowhere at 800
  // words is one nobody could plan for. Enabled is what the floor gates.
  it("enables the build only at the server floor of 800 words", async () => {
    stubApi({
      preview: documentPreview,
      ingests: [
        { stats: documentStats, summary: summaryOf(500) },
        { stats: documentStats, summary: summaryOf(820) },
      ],
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("one.md", "First document.");
    expect(
      await screen.findByText(
        "500 of 800 words",
        {},
        { timeout: INGEST_WAITER_MS },
      ),
    ).toBeTruthy();
    // Below the floor the button still presses: the press names the floor
    // on the rail and starts nothing.
    await userEvent.click(
      screen.getByRole("button", { name: /Build my voice profile/ }),
    );
    expect((await screen.findByRole("alert")).textContent).toContain("800");

    await uploadFile("two.md", "Second document.");
    // At the floor the reason is gone with the block it named.
    await waitFor(() => {
      expect(screen.queryByRole("alert")).toBeNull();
    });
    expect(screen.queryByText("500 of 800 words")).toBeNull();
  });

  it(
    "narrates build stages and lands the succeeded result card with the candidate note",
    async () => {
      stubApi({
        preview: documentPreview,
        ingests: [{ stats: documentStats, summary: summaryOf(820) }],
        builds: {
          [BUILD_IDS[0]]: [
            { id: BUILD_IDS[0], status: "running", stage: "extract" },
            { id: BUILD_IDS[0], status: "succeeded", stage: null },
          ],
        },
      });
      render(<VoiceHarness initial={collectingState()} />);

      await uploadFile("one.md", "Enough material.");
      await userEvent.click(
        await screen.findByRole("button", { name: /Build my voice profile/ }),
      );

      expect(
        await screen.findByText(/Extracting your writing patterns/, undefined, {
          timeout: 4000,
        }),
      ).toBeTruthy();
      expect(
        await screen.findByText(/This is your voice profile\./, undefined, {
          timeout: 4000,
        }),
      ).toBeTruthy();
      expect(
        await screen.findByText(/needs your review before it goes live/),
      ).toBeTruthy();
      expect(await screen.findByText(/Direct, concrete/)).toBeTruthy();
    },
    BUILD_TEST_MS,
  );

  it(
    "offers a retry after a failed build and a second build proceeds",
    async () => {
      const calls = stubApi({
        preview: documentPreview,
        ingests: [{ stats: documentStats, summary: summaryOf(820) }],
        builds: {
          [BUILD_IDS[0]]: [
            {
              id: BUILD_IDS[0],
              status: "failed",
              stage: null,
              status_detail: UNCONFIGURED_DETAIL,
            },
          ],
          [BUILD_IDS[1]]: [
            { id: BUILD_IDS[1], status: "succeeded", stage: null },
          ],
        },
      });
      render(<VoiceHarness initial={collectingState()} />);

      await uploadFile("one.md", "Enough material.");
      await userEvent.click(
        await screen.findByRole("button", { name: /Build my voice profile/ }),
      );

      expect(
        await screen.findByText(
          en["ob.conv.voice.continueFailedStatus"],
          undefined,
          {
            timeout: 4000,
          },
        ),
      ).toBeTruthy();
      // The headline is the same for every broken build, so what the reader
      // acts on is the server's sentence under it. Without this the screen
      // says only "the build did not finish" and the one actionable fact —
      // that no provider is configured — never reaches them.
      expect(await screen.findByText(UNCONFIGURED_DETAIL)).toBeTruthy();

      await userEvent.click(
        screen.getByRole("button", { name: /Retry build/ }),
      );

      expect(
        await screen.findByText(/This is your voice profile\./, undefined, {
          timeout: 4000,
        }),
      ).toBeTruthy();
      expect(requestsTo(calls, "/builds", "POST").length).toBe(2);
    },
    BUILD_TEST_MS,
  );

  it("keeps the newest-by-request-order summary when ingest responses settle out of order", async () => {
    stubApi({
      preview: documentPreview,
      // The first upload's response is held back past the second's: the
      // stale 500-word summary settles last and must not roll the meter
      // (or the build gate) back below the floor.
      ingestsBySource: {
        "one.md": {
          stats: documentStats,
          summary: summaryOf(500),
          delayMs: 150,
        },
        "two.md": { stats: documentStats, summary: summaryOf(820) },
      },
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("one.md", "First document.");
    await uploadFile("two.md", "Second document.");

    // Both sources land in the scene's own list regardless of settlement
    // order — the rail carries neither reaction.
    await waitFor(() => {
      expect(screen.getAllByText("900 words").length).toBe(2);
    });
    // The one-time floor-reached announcement echoes this same sentence into
    // a second, visually-hidden node, so the visible line is queried by its
    // own class rather than by text (which would now match both).
    await waitFor(() => {
      expect(document.querySelector(".ob-voice-meter-line")?.textContent).toBe(
        "820 words: enough to build. More words improve it.",
      );
    });
    expect(
      await screen.findByRole("button", { name: /Build my voice profile/ }),
    ).toBeTruthy();
    expect(screen.queryByText(/I need at least 800/)).toBeNull();
    expect(screen.queryByText("500 of 800 words")).toBeNull();
  });

  it("concludes as failed with the retry chip when the build poll keeps erroring", async () => {
    stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(820) }],
      buildPollStatus: 500,
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("one.md", "Enough material.");
    await userEvent.click(
      await screen.findByRole("button", { name: /Build my voice profile/ }),
    );

    // The act never sits silent in vo.building: a poll that keeps erroring
    // still lands on the one failed outcome the surface knows how to show,
    // with the retry chip back on offer.
    expect(
      await screen.findByText(en["ob.conv.voice.continueFailedStatus"]),
    ).toBeTruthy();
    expect(screen.getByRole("button", { name: /Retry build/ })).toBeTruthy();
  });

  // What a refused build start may say on the collect scene. The two halves
  // of the same rule: the server's own sentence is the best thing a reader
  // can be given, and a failure the server put no words to is answered in
  // the reader's language rather than in whatever the exception happened to
  // carry.
  it("shows the server's own refusal on the collect scene when the build cannot start", async () => {
    stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(820) }],
      buildStartRefusal: {
        status: 422,
        body: {
          code: "validation_error",
          detail: "at least 800 eligible own-authored words are required",
          details: {
            errors: [
              {
                field: "corpus",
                code: "invalid",
                message:
                  "at least 800 eligible own-authored words are required",
              },
            ],
          },
        },
      },
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("one.md", "Enough material.");
    await userEvent.click(
      await screen.findByRole("button", { name: /Build my voice profile/ }),
    );

    expect(
      await screen.findByText(
        "at least 800 eligible own-authored words are required",
      ),
    ).toBeTruthy();
  });

  it("answers a refusal carrying no words for a reader with the shared line", async () => {
    stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(820) }],
      buildStartRefusal: { status: 502, body: { status: 502 } },
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("one.md", "Enough material.");
    await userEvent.click(
      await screen.findByRole("button", { name: /Build my voice profile/ }),
    );

    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toBe("The request failed. No cause reported.");
  });

  it("keeps late events from a superseded build inert after the retry", () => {
    // Machine-level correlation, reusing the shared fold helper: once build
    // two is the active run, build one's late terminal changes nothing.
    const retried = run(
      [
        { type: "BUILD_STARTED", buildId: BUILD_IDS[0] },
        {
          type: "BUILD_TERMINAL",
          buildId: BUILD_IDS[0],
          status: "failed",
          detail: null,
        },
        { type: "BUILD_STARTED", buildId: BUILD_IDS[1] },
      ],
      collectingState(),
    );
    expect(retried.phase).toBe("vo.building");

    const afterStale = conversationReducer(retried, {
      type: "BUILD_TERMINAL",
      buildId: BUILD_IDS[0],
      status: "succeeded",
      detail: null,
    });
    expect(afterStale).toBe(retried);
  });
});

// The rule this act follows: the work happens on the surface, the rail only
// narrates. These pin the three places that rule used to break.
describe("the voice act's surface/rail split", () => {
  // There is no rail to keep these off any more — OnboardingStage is one
  // room, not a board beside a conversation thread — so what these cases
  // pin now is only the positive half: the decision, and every action that
  // used to have a rail copy to disagree with, lives on the one surface.
  it("keeps the speaker decision on the board, radio options and all", async () => {
    stubApi({
      preview: conversationalPreview,
      ingests: [{ stats: transcriptStats, summary: summaryOf(1240) }],
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("call.vtt", "WEBVTT transcript content");
    await screen.findByText(/Which one is you\? Only your own words count/);

    expect(screen.getAllByRole("radio").length).toBeGreaterThan(0);
  });

  it("renders a skipped voice act's Continue action on the surface's own pinned bar", () => {
    stubApi({});
    render(
      <VoiceHarness
        initial={{
          ...initialConversationState,
          act: "voice",
          phase: "vo.skipped",
        }}
      />,
    );

    const bar = document.querySelector(".ob-stage-acts");
    expect(bar).not.toBeNull();
    expect(
      within(bar as HTMLElement).getByRole("button", { name: "Continue" }),
    ).toBeTruthy();
  });

  it("renders the succeeded result as structured sections, each from the same parsed data", async () => {
    stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(820) }],
      builds: {
        [BUILD_IDS[0]]: [
          { id: BUILD_IDS[0], status: "succeeded", stage: null },
        ],
      },
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("one.md", "Enough material.");
    await userEvent.click(
      await screen.findByRole("button", { name: /Build my voice profile/ }),
    );

    // The portrait leads in its own hero, and the confirm action sits on the
    // stage's own rail beside the board.
    const identity = await screen.findByText(/Direct, concrete/);
    expect(identity.closest(".ob-voice-result-hero")).not.toBeNull();
    const bar = document.querySelector(".ob-stage-acts");
    expect(bar).not.toBeNull();
    expect(
      within(bar as HTMLElement).getByRole("button", {
        name: "That is me",
      }),
    ).toBeTruthy();
  });

  it("shows the sample as a real draft and switches to another one already in hand", async () => {
    stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(820) }],
      builds: {
        [BUILD_IDS[0]]: [
          { id: BUILD_IDS[0], status: "succeeded", stage: null },
        ],
      },
      version: richVersion,
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("one.md", "Enough material.");
    await userEvent.click(
      await screen.findByRole("button", { name: /Build my voice profile/ }),
    );

    // Each draft is a press away by its subject; the letter shows one at a time.
    const first = await screen.findByRole("button", {
      name: "Following up on Tuesday",
    });
    expect(first.getAttribute("aria-pressed")).toBe("true");
    const letter = () => document.querySelector(".vdna-letter-head");
    expect(letter()?.textContent).toContain("Following up on Tuesday");
    await userEvent.click(screen.getByRole("button", { name: "Quick nudge" }));
    expect(letter()?.textContent).toContain("Quick nudge");
    expect(letter()?.textContent).not.toContain("Following up on Tuesday");
  });

  it("measures only the dimension it actually scored, as a readout rather than a control", async () => {
    stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(820) }],
      builds: {
        [BUILD_IDS[0]]: [
          { id: BUILD_IDS[0], status: "succeeded", stage: null },
        ],
      },
      version: richVersion,
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("one.md", "Enough material.");
    await userEvent.click(
      await screen.findByRole("button", { name: /Build my voice profile/ }),
    );

    expect(await screen.findByText("Sentence length")).toBeTruthy();
    expect(screen.getByText("Measured: 1")).toBeTruthy();
    expect(screen.getByText("9.5 words per sentence on average.")).toBeTruthy();
    // A readout, not a control: nothing here is focusable or draggable.
    expect(document.querySelector(".ob-voice-dim-track input")).toBeNull();
    expect(document.querySelector('[role="slider"]')).toBeNull();
    // The reference's other four axes (formality, warmth, directness,
    // vocabulary) have no server-measured equivalent, so none render.
    expect(screen.queryByText("Formality")).toBeNull();
  });

  it("keeps the how-you-think, moves, avoid, and improvement sections beside the sample", async () => {
    stubApi({
      preview: documentPreview,
      ingests: [{ stats: documentStats, summary: summaryOf(820) }],
      builds: {
        [BUILD_IDS[0]]: [
          { id: BUILD_IDS[0], status: "succeeded", stage: null },
        ],
      },
      version: richVersion,
    });
    render(<VoiceHarness initial={collectingState()} />);

    await uploadFile("one.md", "Enough material.");
    await userEvent.click(
      await screen.findByRole("button", { name: /Build my voice profile/ }),
    );

    expect(
      await screen.findByText("Leads with the ask, then the context."),
    ).toBeTruthy();
    expect(screen.getByText("Hey Jordan,")).toBeTruthy();
    expect(screen.getByText("Corporate jargon")).toBeTruthy();
    expect(screen.getByText(/Add two more sent emails\./)).toBeTruthy();
    // The sample card's own "why" line draws on the same move names, at a
    // glance rather than in full.
    expect(
      screen.getByText(/Opens with 'Hey', Closes with Cheers/),
    ).toBeTruthy();
  });
});

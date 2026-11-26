/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SettingsScreen } from "../features/settings/settings";
import { LocaleProvider } from "../i18n";
import { parseHash } from "../lib/router";
import { meFixture } from "./mefixture";
import { ReviewActionsProvider } from "./reviewactions";
import {
  emptyPage,
  fileInput,
  jsonResponse,
  PROFILE,
  SOURCE,
  SUMMARY,
  VOICE_EDITOR,
} from "./voice-dna.testkit";

// The Writing voice page remounts each tab, so work still running when the
// reader switches tabs must live in the page. These cases render the settings
// screen itself, so they fail if that stops being true.

// The settings screen itself at one Voice tab, so a tab change remounts what
// production remounts and keeps what production keeps.
function settingsVoicePage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return (tab: string) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ReviewActionsProvider>
          <SettingsScreen route={parseHash(`#/settings/voice/${tab}`)} />
        </ReviewActionsProvider>
      </LocaleProvider>
    </QueryClientProvider>
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the Writing voice page across its tabs", () => {
  it("keeps a waiting speaker question when the tab changes", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const path = new URL(request.url).pathname.replace(/^\/v1/, "");
        if (path === "/me") {
          return jsonResponse(meFixture({ allow: VOICE_EDITOR }));
        }
        if (path === "/voice-profiles") {
          return jsonResponse({ data: [PROFILE], page: emptyPage.page });
        }
        if (path === "/voice-profiles/vp-1/sources/preview") {
          return jsonResponse({
            total_words: 1000,
            detected_format: "vtt",
            ingestible_as_transcript: true,
            unattributed_words: 0,
            speakers: [
              { label: "Lars", words: 640, turns: 12 },
              { label: "Sam", words: 360, turns: 9 },
            ],
          });
        }
        if (path === "/voice-profiles/vp-1/sources") {
          return jsonResponse({ data: [SOURCE], summary: SUMMARY });
        }
        return jsonResponse(emptyPage);
      }),
    );
    const page = settingsVoicePage();
    const view = rtlRender(page("samples"));
    await screen.findByRole("heading", { name: "Make it sharper" });
    await user.upload(
      fileInput(),
      new File(["Lars: we ship Friday. Sam: agreed."], "standup.vtt", {
        type: "text/plain",
      }),
    );
    expect(await screen.findByText(/Which speaker are you/)).toBeTruthy();
    view.rerender(page("rules"));
    await screen.findByRole("heading", { name: "Your rules" });
    expect(screen.getByText(/Which speaker are you/)).toBeTruthy();
  });

  it("holds the rebuild on another tab until an upload lands", async () => {
    const user = userEvent.setup();
    let releasePreview: () => void = () => {};
    const previewHeld = new Promise<void>((resolve) => {
      releasePreview = resolve;
    });
    let releasePost: () => void = () => {};
    const postHeld = new Promise<void>((resolve) => {
      releasePost = resolve;
    });
    let postStarted = false;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const path = new URL(request.url).pathname.replace(/^\/v1/, "");
        if (path === "/me") {
          return jsonResponse(meFixture({ allow: VOICE_EDITOR }));
        }
        if (path === "/voice-profiles") {
          const built = {
            ...PROFILE,
            maturity: "provisional",
            profile_version: 2,
          };
          return jsonResponse({ data: [built], page: emptyPage.page });
        }
        if (path === "/voice-profiles/vp-1/sources/preview") {
          await previewHeld;
          return jsonResponse({
            total_words: 1000,
            detected_format: "txt",
            ingestible_as_transcript: false,
            unattributed_words: 1000,
            speakers: [],
          });
        }
        if (path === "/voice-profiles/vp-1/sources") {
          if (request.method === "POST") {
            postStarted = true;
            await postHeld;
          }
          return request.method === "POST"
            ? jsonResponse(
                {
                  source: {},
                  summary: SUMMARY,
                  ingest_stats: {
                    input_words: 4,
                    kept_words: 4,
                    kept_turns: 0,
                    discarded_turns: 0,
                    speakers_seen: [],
                  },
                },
                201,
              )
            : jsonResponse({ data: [SOURCE], summary: SUMMARY });
        }
        return jsonResponse(emptyPage);
      }),
    );
    const page = settingsVoicePage();
    const view = rtlRender(page("samples"));
    await screen.findByRole("heading", { name: "Make it sharper" });
    await user.upload(
      fileInput(),
      new File(["We ship on Friday."], "notes.txt", { type: "text/plain" }),
    );
    view.rerender(page("builds"));
    const rebuild = await screen.findByRole("button", {
      name: /Rebuild Voice DNA/,
    });
    expect(rebuild).toHaveProperty("disabled", true);
    releasePreview();
    await waitFor(() => expect(postStarted).toBe(true));
    expect(
      screen.getByRole("button", { name: /Rebuild Voice DNA/ }),
    ).toHaveProperty("disabled", true);
    releasePost();
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: /Rebuild Voice DNA/ }),
      ).toHaveProperty("disabled", false),
    );
  });
});

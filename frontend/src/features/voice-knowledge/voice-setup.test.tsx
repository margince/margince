// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { SetupSteps } from "../../components/setup-steps";
import { LocaleProvider } from "../../i18n";
import { settingsReach } from "../../lib/settingscatalog";
import { useSetupChecklist } from "../../lib/setup-checklist";
import { meFixture } from "../../testing/mefixture";
import { candidateVersion } from "../../testing/voice-candidate";
import { VoiceDnaCard } from "./voice-dna";

// The setup checklist and the Writing voice page drawn together over one
// synthetic server. Mail, calendar and booking are done, so the voice step
// alone decides whether setup is finished.

type VoiceProfile = components["schemas"]["VoiceProfile"];
type VoiceProfileVersion = components["schemas"]["VoiceProfileVersion"];
type VoiceBuild = components["schemas"]["VoiceBuild"];

const VOICE_EDITOR = { voice_profile: ["read", "create", "update"] } as const;
const REACH = settingsReach(meFixture({ allow: VOICE_EDITOR }));
const PAGE = { next_cursor: null, has_more: false };

const PROFILE: VoiceProfile = {
  id: "vp-1",
  owner_id: "u1",
  status: "collecting",
  maturity: "provisional",
  quality_band: "good",
  voice_profile_md: "",
  profile_version: 0,
  personality_md: "",
  auto_learning_enabled: false,
  active_source_hash: null,
  candidate_version: null,
  last_built_at: null,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-07-01T00:00:00Z",
  updated_at: "2026-07-01T00:00:00Z",
  archived_at: null,
};

const SUMMARY: components["schemas"]["VoiceCorpusSummary"] = {
  total_words: 1200,
  target_words: 30000,
  maturity: "provisional",
  quality_band: "good",
  source_count: 3,
  register_words: { general: 1200 },
};

function runningBuild(): VoiceBuild {
  return {
    id: "vb-1",
    profile_id: "vp-1",
    reason: "manual",
    status: "running",
    stage: "extract",
    source_hash: "h",
    source_count: 3,
    result_version: null,
    candidate_action: "none",
    status_code: null,
    status_detail: null,
    next_attempt_at: null,
    version: 2,
    created_at: "2026-07-01T00:00:00Z",
    started_at: "2026-07-01T00:00:01Z",
    completed_at: null,
    updated_at: "2026-07-01T00:00:01Z",
    archived_at: null,
  };
}

// What the synthetic server holds. Tests move it the way a build or an
// activation would, and every read answers from it.
type Server = { profile: VoiceProfile; versions: VoiceProfileVersion[] };
let server: Server;
let noProfile = false;
let applyStatus = 200;

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function firstCandidate(): Server {
  return {
    profile: { ...PROFILE, candidate_version: 1 },
    versions: [
      candidateVersion(["Too few samples for a separate evaluation."]),
    ],
  };
}

function activate(state: Server): Server {
  return {
    profile: { ...state.profile, profile_version: 1, candidate_version: null },
    versions: state.versions.map((version) => ({
      ...version,
      status: "active",
    })),
  };
}

function answer(request: Request): Response {
  const path = new URL(request.url).pathname.replace(/^\/v1/, "");
  if (path === "/me") return json(meFixture({ allow: VOICE_EDITOR }));
  if (path === "/connectors") {
    return json({
      data: [
        {
          id: "c-1",
          provider: "gmail",
          status: "connected",
          can_backfill: true,
          scopes: [],
          account_label: "ada@example.test",
        },
        {
          id: "c-2",
          provider: "gcal",
          status: "connected",
          can_backfill: true,
          scopes: ["https://www.googleapis.com/auth/calendar.events"],
          account_label: "ada@example.test",
        },
      ],
      providers: ["gmail", "graph", "imap"].map((provider) => ({
        provider,
        reason: "ready",
      })),
    });
  }
  if (path === "/scheduling/profile") {
    return json({
      enabled: true,
      provider: "gcal",
      calendar_id: "primary",
      duration_minutes: 30,
      notice_minutes: 240,
      horizon_days: 30,
      buffer_minutes: 0,
      title: "Intro call",
      location: "",
    });
  }
  if (path === "/voice-profiles")
    return json({ data: noProfile ? [] : [server.profile], page: PAGE });
  if (path === "/voice-profiles/vp-1/sources")
    return json({ data: [], summary: SUMMARY });
  if (path === "/voice-profiles/vp-1/versions")
    return json({ data: server.versions, page: PAGE });
  if (path === "/voice-profiles/vp-1/versions/1/apply") {
    if (applyStatus !== 200) {
      return json(
        {
          status: applyStatus,
          code: "conflict",
          title: "The version changed.",
        },
        applyStatus,
      );
    }
    server = activate(server);
    return json(server.versions[0]);
  }
  if (path === "/voice-profiles/vp-1/learning") {
    return json({
      drafted: 0,
      edited_sent: 0,
      rejected: 0,
      transformations: [],
    });
  }
  return json({ data: [], page: PAGE });
}

function SetupPanel() {
  const setup = useSetupChecklist(REACH);
  if (!setup.settled) return null;
  return (
    <section aria-label="Setup">
      {setup.done === setup.total ? (
        <p>Setup complete</p>
      ) : (
        <SetupSteps steps={setup.steps} count={setup} />
      )}
    </section>
  );
}

function renderBoth(
  client = new QueryClient({ defaultOptions: { queries: { retry: false } } }),
) {
  const view = render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SetupPanel />
        <VoiceDnaCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { ...view, client };
}

async function setupPanel() {
  return screen.findByRole("region", { name: "Setup" });
}

beforeEach(() => {
  noProfile = false;
  applyStatus = 200;
  globalThis.location.hash = "#/settings/voice";
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => answer(request)),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("finishing setup with a trained voice awaiting review", () => {
  it("offers Review voice, never the sample prompt, and focuses the review", async () => {
    server = firstCandidate();
    const user = userEvent.setup();
    renderBoth();

    const setup = await setupPanel();
    expect(
      await within(setup).findByText(
        "Your voice is trained and ready to review.",
      ),
    ).toBeTruthy();
    expect(within(setup).queryByText(/more words you wrote/)).toBeNull();
    expect(
      within(setup).queryByRole("button", { name: "Add samples" }),
    ).toBeNull();

    // Twice: the second press lands while the reader is already on the page.
    for (let press = 0; press < 2; press++) {
      (document.activeElement as HTMLElement | null)?.blur();
      await user.click(
        within(setup).getByRole("button", { name: "Review voice" }),
      );
      await waitFor(() =>
        expect(document.activeElement?.id).toBe("vdna-review"),
      );
      expect(globalThis.location.hash).not.toContain("focus=");
    }
    const review = document.getElementById("vdna-review");
    expect(review?.textContent).toContain("Why this version needs review");
    // Opening the review activates nothing.
    expect(server.profile.profile_version).toBe(0);
    expect(
      within(review as HTMLElement).getByRole("button", {
        name: "Discard this version",
      }),
    ).toBeTruthy();
    expect(review?.textContent).toContain(
      "Setup is done once you use a version.",
    );
  });

  it("finishes setup on every surface once the version is applied", async () => {
    server = firstCandidate();
    const user = userEvent.setup();
    renderBoth();
    await within(await setupPanel()).findByRole("button", {
      name: "Review voice",
    });

    await user.click(
      await screen.findByRole("button", { name: "Use this version" }),
    );

    expect(
      await within(await setupPanel()).findByText("Setup complete"),
    ).toBeTruthy();
  });

  it("does not claim completion when the apply fails", async () => {
    server = firstCandidate();
    applyStatus = 409;
    const user = userEvent.setup();
    renderBoth();
    const setup = await setupPanel();
    await within(setup).findByRole("button", { name: "Review voice" });

    await user.click(
      await screen.findByRole("button", { name: "Use this version" }),
    );

    expect(await screen.findByText("The version changed.")).toBeTruthy();
    expect(
      within(setup).getByRole("button", { name: "Review voice" }),
    ).toBeTruthy();
    expect(within(setup).queryByText("Setup complete")).toBeNull();
  });

  it("keeps setup complete for an active voice with a newer candidate", async () => {
    const active = activate(firstCandidate());
    server = {
      profile: { ...active.profile, candidate_version: 2 },
      versions: [
        { ...candidateVersion([]), id: "v-2", profile_version: 2 },
        ...active.versions,
      ],
    };
    renderBoth();

    expect(
      await within(await setupPanel()).findByText("Setup complete"),
    ).toBeTruthy();
    expect(await screen.findByText("Voice v2 ready for review")).toBeTruthy();
  });

  it("shows a build that finished after the page stopped watching once the reader returns", async () => {
    server = {
      profile: { ...PROFILE, latest_build: runningBuild() },
      versions: [],
    };
    const first = renderBoth();
    const setup = await setupPanel();
    expect(
      await within(setup).findByText(
        "Building your voice. It continues if you leave.",
      ),
    ).toBeTruthy();
    expect(
      within(setup).getByRole("button", { name: "View progress" }),
    ).toBeTruthy();
    first.unmount();

    // The build finished in the background with a candidate to review.
    server = {
      profile: {
        ...PROFILE,
        candidate_version: 1,
        latest_build: {
          ...runningBuild(),
          status: "succeeded",
          result_version: 1,
          candidate_action: "review_required",
        },
      },
      versions: [candidateVersion([])],
    };
    renderBoth(first.client);

    expect(
      await within(await setupPanel()).findByRole("button", {
        name: "Review voice",
      }),
    ).toBeTruthy();
    expect(await screen.findByText("Voice v1 ready for review")).toBeTruthy();
  });

  it("quotes the server's count of missing words for a thin corpus", async () => {
    server = {
      profile: {
        ...PROFILE,
        maturity: "collecting",
        first_build_words_remaining: 380,
      },
      versions: [],
    };
    renderBoth();

    expect(
      await within(await setupPanel()).findByText(
        "Needs about 380 more words you wrote.",
      ),
    ).toBeTruthy();
  });

  it("lands Add samples on the intake when no profile exists yet", async () => {
    noProfile = true;
    server = firstCandidate();
    const user = userEvent.setup();
    renderBoth();
    const setup = await setupPanel();
    expect(
      within(setup).getByText("Needs writing samples of your own."),
    ).toBeTruthy();

    await user.click(
      within(setup).getByRole("button", { name: "Add samples" }),
    );

    await waitFor(() =>
      expect(document.activeElement?.id).toBe("vdna-sharpen"),
    );
    const intake = document.getElementById("vdna-sharpen");
    expect(
      intake && within(intake).getByRole("button", { name: /Add text/ }),
    ).toBeTruthy();
    expect(globalThis.location.hash).not.toContain("focus=");
  });
});

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { tableRowCells } from "../../testing/tablerow.testkit";
import { VoiceSamples } from "./voice-samples";
import { VoiceHistory } from "./voice-versions";

type Source = components["schemas"]["VoiceCorpusSource"];
type Version = components["schemas"]["VoiceProfileVersion"];

function source(over: Partial<Source>): Source {
  return {
    id: "vs-1",
    origin: "manual",
    kind: "proposal",
    register: "long_form",
    weight: 1,
    source_label: "Q3 proposal",
    source_ref: "settings:paste:1",
    word_count: 6_000,
    included: true,
    exclusion_reason: null,
    extractor_version: "1",
    occurred_at: "2026-07-01T00:00:00Z",
    retention_until: null,
    content_erased_at: null,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-07-01T00:00:00Z",
    updated_at: "2026-07-01T00:00:00Z",
    archived_at: null,
    ...over,
  };
}

const SOURCES = [
  source({ version: 3 }),
  source({
    id: "vs-2",
    kind: "linkedin",
    register: "social",
    source_label: "Post on audits",
    source_ref: "settings:paste:2",
    word_count: 300,
    included: false,
    exclusion_reason: "owner_excluded",
  }),
  source({
    id: "vs-3",
    kind: "email",
    register: "email",
    source_label: "Renewal note",
    source_ref: "settings:paste:3",
    word_count: 2_500,
  }),
];

const SUMMARY: components["schemas"]["VoiceCorpusSummary"] = {
  total_words: 8_500,
  target_words: 30000,
  maturity: "building",
  quality_band: "good",
  source_count: 3,
  register_words: { long_form: 6_000, email: 2_500 },
};

const EVALUATION: Version["evaluation"] = {
  held_out_prompts: 5,
  repeats_per_prompt: 3,
  active_median_voice_score: null,
  anti_ai_hard_failures: 0,
  structured_output_valid: true,
  corpus_citations_valid: true,
  identity_word_jaccard: 1,
  signature_set_jaccard: 1,
  removed_avoid_rules: 0,
  removed_register_rules: 0,
  classification: "routine",
  passed: true,
};

function version(n: number, status: Version["status"]): Version {
  return {
    id: `v-${n}`,
    profile_id: "vp-1",
    profile_version: n,
    status,
    voice_profile_md: "",
    profile_json: {},
    stats_json: {},
    source_hash: `h${n}`,
    source_count: n + 1,
    reason: n === 1 ? "onboarding" : "manual",
    predecessor_version: n === 1 ? null : n - 1,
    model_provider: "m",
    model_name: "m",
    builder_version: "1",
    source: "manual",
    captured_by: "human:u1",
    activation_policy_version: "1",
    evaluation: EVALUATION,
    review_reasons: [],
    version: 1,
    created_at: `2026-07-0${n}T00:00:00Z`,
    updated_at: `2026-07-0${n}T00:00:00Z`,
    archived_at: null,
    activated_at: null,
  };
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function stubApi() {
  const writes: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname.replace(/^\/v1/, "");
      if (request.method !== "GET") {
        const pin = request.headers.get("If-Match");
        writes.push(
          `${request.method} ${path}${pin ? ` If-Match ${pin}` : ""}`,
        );
        return new Response(null, { status: 204 });
      }
      if (path === "/voice-profiles/vp-1/sources") {
        return jsonResponse({ data: SOURCES, summary: SUMMARY });
      }
      if (path === "/voice-profiles/vp-1/versions") {
        return jsonResponse({
          data: [
            version(1, "superseded"),
            version(2, "active"),
            version(3, "candidate"),
          ],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (path === "/voice-profiles/vp-1/learning") {
        return jsonResponse({ drafted: 0, edited_sent: 0, rejected: 0 });
      }
      return jsonResponse({}, 404);
    }),
  );
  return writes;
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

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("VoiceSamples", () => {
  it("names each sample with its kind, and marks only an excluded one", async () => {
    stubApi();
    render(<VoiceSamples profileId="vp-1" canEdit onChanged={() => {}} />);

    const kept = await tableRowCells("Current samples", "Q3 proposal");
    expect(kept.Sample).toHaveTextContent("Proposal");
    expect(kept.Words).toHaveTextContent("6,000");
    expect(kept.State).toBeEmptyDOMElement();
    const left = await tableRowCells("Current samples", "Post on audits");
    expect(within(left.State).getByText("Excluded")).toBeInTheDocument();
    expect(
      within(left.State).getByText("Left out of your voice"),
    ).toBeInTheDocument();
  });

  it("warns of the band it drops to before removing a sample, and removes only on confirm", async () => {
    const writes = stubApi();
    const user = userEvent.setup();
    render(<VoiceSamples profileId="vp-1" canEdit onChanged={() => {}} />);

    await user.click(
      await screen.findByRole("button", { name: "Actions for Q3 proposal" }),
    );
    await user.click(screen.getByRole("button", { name: "Remove sample" }));
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Remove Q3 proposal?")).toBeInTheDocument();
    expect(
      dialog.getByText("Removing this lowers the voice from good to thin."),
    ).toBeInTheDocument();
    expect(writes).toEqual([]);

    await user.click(dialog.getByRole("button", { name: "Remove sample" }));
    await waitFor(() =>
      expect(writes).toEqual([
        "DELETE /voice-profiles/vp-1/sources/vs-1 If-Match 3",
      ]),
    );
    await waitFor(() =>
      expect(document.activeElement?.textContent).toMatch(
        /^[\d,]+ of [\d,]+ words$/,
      ),
    );
  });

  it("offers no verbs to a seat that cannot edit", async () => {
    stubApi();
    render(
      <VoiceSamples profileId="vp-1" canEdit={false} onChanged={() => {}} />,
    );

    await tableRowCells("Current samples", "Q3 proposal");
    expect(
      screen.queryByRole("button", { name: /^Actions for / }),
    ).not.toBeInTheDocument();
  });
});

describe("VoiceHistory", () => {
  it("badges only a version that asks something of the reader, and names the active one", async () => {
    stubApi();
    render(<VoiceHistory profileId="vp-1" canEdit onChanged={() => {}} />);

    const active = await tableRowCells("Versions and learning", "Version 2");
    expect(active.Status).toHaveTextContent("Active");
    expect(active.Status.querySelector(".badge-label")).toBeNull();
    const superseded = await tableRowCells(
      "Versions and learning",
      "Version 1",
    );
    expect(superseded.Status).toBeEmptyDOMElement();
    const candidate = await tableRowCells("Versions and learning", "Version 3");
    expect(candidate.Status).toHaveTextContent("Awaiting review");
  });

  it("rolls back only after the reader confirms", async () => {
    const writes = stubApi();
    const user = userEvent.setup();
    render(<VoiceHistory profileId="vp-1" canEdit onChanged={() => {}} />);

    await user.click(
      await screen.findByRole("button", { name: "Actions for Version 1" }),
    );
    await user.click(
      screen.getByRole("button", { name: "Roll back to version 1" }),
    );
    const dialog = within(screen.getByRole("dialog"));
    expect(dialog.getByText("Roll back to version 1?")).toBeInTheDocument();
    expect(writes).toEqual([]);

    await user.click(
      dialog.getByRole("button", { name: "Roll back to version 1" }),
    );
    await waitFor(() =>
      expect(writes).toEqual(["POST /voice-profiles/vp-1/versions/1/rollback"]),
    );
    await waitFor(() =>
      expect(document.activeElement?.textContent).toBe("Versions and learning"),
    );
  });

  it("returns focus to the menu when the rollback is called off", async () => {
    stubApi();
    const user = userEvent.setup();
    render(<VoiceHistory profileId="vp-1" canEdit onChanged={() => {}} />);

    const menu = await screen.findByRole("button", {
      name: "Actions for Version 1",
    });
    await user.click(menu);
    await user.click(
      screen.getByRole("button", { name: "Roll back to version 1" }),
    );
    await user.click(
      within(screen.getByRole("dialog")).getByRole("button", {
        name: "Cancel",
      }),
    );
    await waitFor(() => expect(document.activeElement).toBe(menu));
  });
});

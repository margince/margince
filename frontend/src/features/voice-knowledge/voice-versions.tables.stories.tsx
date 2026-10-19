// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import { DataTable } from "../../design-system/datatable";
import { Panel } from "../../design-system/panel";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { StoryProviders } from "../../testing/story-utils";
import { changeColumns, versionColumns } from "./voice-versions.tables";

// Every version status and every change outcome, drawn from the column sets
// with no reads behind them.

type Version = components["schemas"]["VoiceProfileVersion"];
type Delta = components["schemas"]["VoiceProfileDelta"];

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

function version(
  n: number,
  status: Version["status"],
  reason: Version["reason"],
): Version {
  return {
    id: `v-${n}`,
    profile_id: "vp-1",
    profile_version: n,
    status,
    voice_profile_md: "",
    profile_json: {},
    stats_json: {},
    source_hash: `h${n}`,
    source_count: n * 2,
    reason,
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
    created_at: `2026-09-0${n}T00:00:00Z`,
    updated_at: `2026-09-0${n}T00:00:00Z`,
    archived_at: null,
    activated_at: null,
  };
}

const VERSIONS = [
  version(5, "candidate", "automatic"),
  version(4, "rejected", "automatic"),
  version(3, "active", "manual"),
  version(2, "superseded", "rollback"),
  version(1, "superseded", "onboarding"),
];

function delta(
  to: number,
  classification: Delta["classification"],
  outcome: Delta["activation_outcome"],
  words: number,
): Delta {
  return {
    id: `d-${to}`,
    profile_id: "vp-1",
    from_version: to === 1 ? null : to - 1,
    to_version: to,
    classification,
    activation_outcome: outcome,
    words_added: words,
    sources_added: 1,
    sources_excluded: 0,
    identity_word_jaccard: null,
    signature_set_jaccard: null,
    avoid_rules_added: 0,
    avoid_rules_removed: 0,
    register_rules_removed: 0,
    created_at: `2026-09-0${to}T00:00:00Z`,
    updated_at: `2026-09-0${to}T00:00:00Z`,
    archived_at: null,
  };
}

const DELTAS = [
  delta(5, "material", "review_required", 2_140),
  delta(4, "material", "rejected", 0),
  delta(3, "routine", "manually_activated", 1),
  delta(2, "routine", "rollback", 0),
  delta(1, "material", "auto_activated", 1_840),
];

function Tables() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = viewerZone();
  return (
    <Panel title="Builds">
      <DataTable
        bleed
        fold
        label="Versions and learning"
        columns={versionColumns({
          t,
          plural,
          locale,
          zone,
          canEdit: true,
          onRestore: () => undefined,
        })}
        rows={VERSIONS}
        rowKey={(row) => row.id}
      />
      <DataTable
        bleed
        fold
        label="What changed"
        columns={changeColumns({ t, plural, locale, zone })}
        rows={DELTAS}
        rowKey={(row) => row.id}
      />
    </Panel>
  );
}

const meta: Meta = {
  title: "Settings/You/Writing voice/Build tables",
};
export default meta;
type Story = StoryObj;

export const EveryState: Story = {
  render: () => (
    <StoryProviders>
      <Tables />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const table = await canvas.findByRole("table", {
      name: "Versions and learning",
    });
    const active = within(table)
      .getAllByRole("row")
      .find((row) => row.textContent?.includes("Version 3"));
    if (active === undefined) {
      throw new Error("the active version has no row");
    }
    const marker = within(active).getByText("Active");
    await expect(marker.closest(".badge-label")).toBeNull();
  },
};

export const EveryStateAtPhoneWidth: Story = {
  ...EveryState,
  tags: ["uat-phone"],
};

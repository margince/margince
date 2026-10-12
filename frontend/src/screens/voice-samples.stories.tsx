// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { VoiceSamples } from "./voice-samples";

// The samples a voice is built from, one row each. Only a sample left out
// carries a badge.

type Source = components["schemas"]["VoiceCorpusSource"];

function sample(
  id: string,
  label: string,
  kind: Source["kind"],
  register: Source["register"],
  words: number,
  excluded?: string,
): Source {
  return {
    id,
    origin: "manual",
    kind,
    register,
    weight: 1,
    source_label: label,
    source_ref: `settings:paste:${id}`,
    word_count: words,
    included: excluded === undefined,
    exclusion_reason: excluded ?? null,
    extractor_version: "1",
    occurred_at: "2026-09-01T00:00:00Z",
    retention_until: null,
    content_erased_at: null,
    source: "manual",
    captured_by: "human:u1",
    version: 1,
    created_at: "2026-09-01T00:00:00Z",
    updated_at: "2026-09-01T00:00:00Z",
    archived_at: null,
  };
}

const SOURCES = [
  sample(
    "s1",
    "Q3 proposal for Nordwind Logistics",
    "proposal",
    "long_form",
    5_210,
  ),
  sample("s2", "Discovery call, 2 Oct", "transcript", "spoken", 3_120),
  sample(
    "s3",
    "Post on supplier audits",
    "linkedin",
    "social",
    260,
    "owner_excluded",
  ),
  sample("s4", "Renewal follow-up to Brandt", "email", "email", 1_840),
];

const SUMMARY: components["schemas"]["VoiceCorpusSummary"] = {
  total_words: 10_170,
  target_words: 30000,
  maturity: "building",
  quality_band: "good",
  source_count: 4,
  register_words: { long_form: 5_210, spoken: 3_120, email: 1_840 },
};

function story(canEdit: boolean) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ voice_profile: ["read", "update"] }),
      "GET /voice-profiles/vp-1/sources": () =>
        jsonResponse({ data: SOURCES, summary: SUMMARY }),
    });
    return (
      <StoryProviders>
        <Panel title="Writing samples">
          <VoiceSamples
            profileId="vp-1"
            canEdit={canEdit}
            onChanged={() => undefined}
          />
        </Panel>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof VoiceSamples> = {
  title: "Settings/You/Writing voice/Samples",
  component: VoiceSamples,
};
export default meta;
type Story = StoryObj<typeof VoiceSamples>;

export const Samples: Story = { render: story(true) };

export const SamplesAtPhoneWidth: Story = {
  ...Samples,
  tags: ["uat-phone"],
};

export const SamplesDark: Story = {
  ...Samples,
  globals: { theme: "dark" },
};

export const ReadOnly: Story = { render: story(false) };

// Removing the largest sample drops the voice a band, and the confirm says so.
export const RemoveDropsBand: Story = {
  render: story(true),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: "Actions for Q3 proposal for Nordwind Logistics",
      }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await userEvent.click(page.getByRole("button", { name: "Remove sample" }));
    const dialog = within(await page.findByRole("dialog"));
    const drop = dialog.getByText(
      "Removing this lowers the voice from good to thin.",
    );
    await waitFor(() => expect(drop).toBeVisible());
  },
};

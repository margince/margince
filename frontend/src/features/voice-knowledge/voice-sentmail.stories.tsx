// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { Panel, PanelBody } from "../../design-system/panel";
import { SettingList } from "../../design-system/settingrow";
import { StoryProviders } from "../../testing/story-utils";
import { LearnFromSentMail, type SentMailLearning } from "./voice-sentmail";

// The one press that reads the owner's own sent mail into their samples. The
// stories show each answer: mails learned, nothing new, no sent mail at all,
// and a press the server refused.

const SUMMARY: components["schemas"]["VoiceCorpusSummary"] = {
  total_words: 4200,
  target_words: 30000,
  maturity: "provisional",
  quality_band: "thin",
  source_count: 37,
  register_words: { email: 4200 },
};

function learned(
  counts: Partial<components["schemas"]["VoiceSentMailLearned"]>,
): SentMailLearning["outcome"] {
  return {
    kind: "learned",
    learned: {
      read: 0,
      added: 0,
      already_present: 0,
      no_own_words: 0,
      summary: SUMMARY,
      ...counts,
    },
  };
}

function story(learning: Omit<SentMailLearning, "press">) {
  return () => (
    <StoryProviders>
      <Panel title="Writing samples">
        <PanelBody>
          <SettingList>
            <LearnFromSentMail
              learning={{ ...learning, press: async () => undefined }}
            />
          </SettingList>
        </PanelBody>
      </Panel>
    </StoryProviders>
  );
}

const meta: Meta<typeof LearnFromSentMail> = {
  title: "Settings/You/Writing voice/Learn from sent mail",
  component: LearnFromSentMail,
};
export default meta;
type Story = StoryObj<typeof LearnFromSentMail>;

export const Resting: Story = {
  render: story({ reading: false, outcome: null }),
};

export const Reading: Story = {
  render: story({ reading: true, outcome: null }),
};

export const Learned: Story = {
  render: story({
    reading: false,
    outcome: learned({ read: 40, added: 36, no_own_words: 4 }),
  }),
};

export const NothingNew: Story = {
  render: story({
    reading: false,
    outcome: learned({ read: 36, already_present: 36 }),
  }),
};

export const NoSentMail: Story = {
  render: story({ reading: false, outcome: learned({}) }),
};

export const Refused: Story = {
  render: story({
    reading: false,
    outcome: { kind: "failed", problem: { title: "Not found", status: 404 } },
  }),
};

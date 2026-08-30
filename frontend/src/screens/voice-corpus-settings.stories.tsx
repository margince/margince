// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList } from "../design-system/settingrow";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";
import { VoiceCorpusIntake } from "./voice-corpus-settings";

// Where a writing sample arrives: dropped on the zone, or chosen through it.
// The two forms it takes are `first` — the control that MINTS the profile,
// which also states the word floor because there is no meter yet — and every
// one after it, where the corpus meter above says how far the floor is.
//
// A file drop cannot be performed from a story, so what is catalogued is the
// resting state of both: the zone, what teaches the voice, and the reason
// behind the ask.
function story(first: boolean) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ voice_profile: ["read", "create", "update"] }),
    });
    return (
      <StoryProviders>
        {/* In the card the row sits in a `SettingList` beside the manifest;
            here the panel stands in for that card so the row is drawn on the
            ground it actually lands on. */}
        <Panel title="Writing samples">
          <PanelBody>
            <SettingList>
              <VoiceCorpusIntake
                first={first}
                profileId={first ? null : "vp-1"}
                onChanged={() => undefined}
              />
            </SettingList>
          </PanelBody>
        </Panel>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof VoiceCorpusIntake> = {
  title: "Settings/You/Writing voice/Corpus intake",
  component: VoiceCorpusIntake,
};
export default meta;
type Story = StoryObj<typeof VoiceCorpusIntake>;

export const FirstSample: Story = { render: story(true) };

export const AnotherSample: Story = { render: story(false) };

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ComposeModal } from "./compose";
import { AiDraftBand } from "./composeaiband";
import { SelectedReply, selectedReplyRoutes } from "./composethread.stories";

const meta: Meta = {
  title: "Patterns/Compose mail/AI line",
  component: AiDraftBand,
};
export default meta;

/** The ask for a draft, one line above the words of a reply. */
export const Asking: StoryObj = {
  render: SelectedReply.render,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByRole("button", { name: "Draft reply" });
  },
};

// A reply the server drafted in German, as it read the correspondence.
function draftedIn(modelled: boolean) {
  return () => {
    installFetchStub({
      ...selectedReplyRoutes(),
      // The composer saves as the rep writes, and the server echoes the save.
      "PUT /mail-drafts": (body) =>
        jsonResponse({
          ...(typeof body === "object" && body !== null ? body : {}),
          id: "md-1",
          version: 1,
          agent_drafted: false,
          created_at: "2026-09-01T10:00:00Z",
          updated_at: "2026-09-01T10:00:00Z",
        }),
      "POST /activities/message-0/draft-email": () =>
        jsonResponse({
          subject: "Re: Delivery window",
          body: "Hallo Ada,\n\ndie Woche ab dem 14. September passt uns gut.",
          to: ["ada@example.test"],
          ai_generated: modelled,
          ai_disclosure: modelled
            ? "Drafted with AI assistance. Review before sending."
            : null,
          voice_profile_version: null,
          draft_ref: null,
          language: "de",
        }),
    });
    return (
      <StoryProviders>
        <ComposeModal
          activityId="message-0"
          entityType="contact"
          entityId="ada"
          open
          onClose={() => {}}
        />
      </StoryProviders>
    );
  };
}

async function draftAndFindLanguage() {
  const frame = await screen.findByRole("dialog");
  const dialog = within(frame);
  await userEvent.click(
    await dialog.findByRole("button", { name: "Draft reply" }),
  );
  const picker = await dialog.findByRole("combobox", {
    name: "Draft language",
  });
  await expect(picker).toHaveTextContent("Deutsch");
  await expect(frame.scrollWidth).toBeLessThanOrEqual(frame.clientWidth);
}

/** A draft stands: the line names its language, and a pick writes the words
 *  on screen again in another one. */
export const DraftLanguage: StoryObj = {
  render: draftedIn(true),
  play: draftAndFindLanguage,
};

export const DraftLanguageDark: StoryObj = {
  ...DraftLanguage,
  globals: { theme: "dark" },
};

export const DraftLanguagePhone: StoryObj = {
  ...DraftLanguage,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** No model wrote the draft, so no disclosure stands; the language is still
 *  named under the ask. */
export const DraftLanguageWithoutModel: StoryObj = {
  render: draftedIn(false),
  play: draftAndFindLanguage,
};

export const DraftLanguageWithoutModelPhone: StoryObj = {
  ...DraftLanguageWithoutModel,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

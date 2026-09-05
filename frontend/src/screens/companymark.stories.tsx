// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { CompanyMark } from "./companymark";
import { installFetchStub, StoryProviders } from "./story-utils";

// The installation's own face, at the head of the company card. The states
// that matter here are not variants of one control — they are the answers to
// "what does this company look like": the two marks it can wear, the monogram
// that stands in when it wears neither, and the row a reader who may not edit
// the company sees.

const meta: Meta<typeof CompanyMark> = {
  title: "Settings/Company logo",
  component: CompanyMark,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CompanyMark>;
type CompanyProfile = components["schemas"]["CompanyProfile"];

const ORG = "00000000-0000-4000-8000-000000000010";

const WITHOUT_MARK: CompanyProfile = {
  organization_id: ORG,
  display_name: "Brandt Automotive GmbH",
};

// Data-URI stand-ins for the stored PNGs. Storybook serves no object store, and
// the SHAPES are the case these previews exist to prove: a 4:1 wordmark in the
// wide slot and a 1:1 badge in the square one, each in a box cut for it.
const WORDMARK =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 700 200'%3E%3Crect width='700' height='200' fill='white'/%3E%3Ctext x='350' y='125' text-anchor='middle' font-family='sans-serif' font-size='92' font-weight='700' fill='%23ff6500'%3EGRADION%3C/text%3E%3C/svg%3E";
const BADGE =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 200 200'%3E%3Crect width='200' height='200' rx='36' fill='%23ff6500'/%3E%3Ctext x='100' y='142' text-anchor='middle' font-family='sans-serif' font-size='128' font-weight='700' fill='white'%3EG%3C/text%3E%3C/svg%3E";

export const NoMarkYet: Story = {
  args: { profile: WITHOUT_MARK, canEdit: true },
  render: (args) => (
    <StoryProviders>
      <CompanyMark {...args} />
    </StoryProviders>
  ),
};

// The ordinary case for an installation whose website read resolved a logo: the
// wide slot filled, the square one still an invitation. It is the state the
// collapsed sidebar falls back through, so the copy under the empty slot has to
// say that nothing is broken.
export const WideMarkOnly: Story = {
  args: { profile: { ...WITHOUT_MARK, logo_url: WORDMARK }, canEdit: true },
  render: (args) => (
    <StoryProviders>
      <CompanyMark {...args} />
    </StoryProviders>
  ),
};

export const BothMarks: Story = {
  args: {
    profile: { ...WITHOUT_MARK, logo_url: WORDMARK, logo_icon_url: BADGE },
    canEdit: true,
  },
  render: (args) => (
    <StoryProviders>
      <CompanyMark {...args} />
    </StoryProviders>
  ),
};

// The same card on the dark ground. Every derived colour is a color-mix of a
// canonical token and follows the dark accent lift, so a mark and its two verbs
// can read on light and lose contrast on dark without any test noticing.
export const BothMarksDark: Story = {
  ...BothMarks,
  globals: { theme: "dark" },
};

// Read-only: the marks are facts about the company, so they are still shown.
// What goes is the verbs, rather than being drawn disabled — a control whose
// only possible answer is a refusal is worse than no control.
export const NotYoursToChange: Story = {
  args: {
    profile: { ...WITHOUT_MARK, logo_url: WORDMARK, logo_icon_url: BADGE },
    canEdit: false,
  },
  render: (args) => (
    <StoryProviders>
      <CompanyMark {...args} />
    </StoryProviders>
  ),
};

// The server is the one that judges an image: the picker filters on media type
// and says nothing about whether the bytes behind it decode. This frame is that
// refusal, rendered where the person is standing — under the slot they used,
// and not under the other one.
export const TheServerRefusesTheImage: Story = {
  args: { profile: WITHOUT_MARK, canEdit: true },
  render: (args) => {
    installFetchStub({
      "POST /company/logo/icon": () =>
        new Response(
          JSON.stringify({
            type: "about:blank",
            title: "Unsupported media type",
            status: 415,
            code: "unsupported_media_type",
            detail:
              "the upload is not an image this server can read; PNG, JPEG, GIF, WebP, ICO and SVG work",
          }),
          {
            status: 415,
            headers: { "content-type": "application/problem+json" },
          },
        ),
    });
    return (
      <StoryProviders>
        <CompanyMark {...args} />
      </StoryProviders>
    );
  },
  // Driven to the refusal rather than posed at it: the callout appears only
  // after a file is offered and judged, so a frame that stopped at "Add a
  // square icon" would be the NoMarkYet story under a different name.
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Add a square icon" }),
    );
    const icon = within(canvas.getByRole("region", { name: "Square icon" }));
    await userEvent.upload(
      icon.getByLabelText("Square icon"),
      new File(["not an image"], "mark.png", { type: "image/png" }),
    );
    await icon.findByRole("alert");
  },
};

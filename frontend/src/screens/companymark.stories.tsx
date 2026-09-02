// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { CompanyMark } from "./companymark";
import { installFetchStub, StoryProviders } from "./story-utils";

// The installation's own face, at the head of the company card. The states
// that matter here are not variants of one control — they are three different
// answers to "what does this company look like": a mark somebody chose, the
// monogram that stands in when nobody has, and the row a reader who may not
// edit the company sees.

const meta: Meta<typeof CompanyMark> = {
  title: "Settings/Company mark",
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

// A one-pixel PNG stands in for the stored mark. Storybook serves no object
// store, and what a broken image would draw here is the monogram — which is
// the story below, so this one has to actually paint something.
const PIXEL =
  "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==";

export const NoMarkYet: Story = {
  args: { profile: WITHOUT_MARK, canEdit: true },
  render: (args) => (
    <StoryProviders>
      <CompanyMark {...args} />
    </StoryProviders>
  ),
};

export const WearingItsOwnMark: Story = {
  args: { profile: { ...WITHOUT_MARK, logo_url: PIXEL }, canEdit: true },
  render: (args) => (
    <StoryProviders>
      <CompanyMark {...args} />
    </StoryProviders>
  ),
};

// The same row on the dark ground. Every derived colour is a color-mix of a
// canonical token and follows the dark accent lift, so a mark and its two verbs
// can read on light and lose contrast on dark without any test noticing.
export const WearingItsOwnMarkDark: Story = {
  ...WearingItsOwnMark,
  globals: { theme: "dark" },
};

// Read-only: the mark is a fact about the company, so it is still shown. What
// goes is the pair of verbs, rather than being drawn disabled — a control whose
// only possible answer is a refusal is worse than no control.
export const NotYoursToChange: Story = {
  args: { profile: { ...WITHOUT_MARK, logo_url: PIXEL }, canEdit: false },
  render: (args) => (
    <StoryProviders>
      <CompanyMark {...args} />
    </StoryProviders>
  ),
};

// The server is the one that judges an image: the picker filters on media type
// and says nothing about whether the bytes behind it decode. This frame is that
// refusal, rendered where the person is standing.
export const TheServerRefusesTheImage: Story = {
  args: { profile: WITHOUT_MARK, canEdit: true },
  render: (args) => {
    installFetchStub({
      "POST /company/logo": () =>
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
  // after a file is offered and judged, so a frame that stopped at "Add a mark"
  // would be the NoMarkYet story under a different name.
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: "Add a mark" }),
    );
    await userEvent.upload(
      canvas.getByLabelText("Company mark"),
      new File(["not an image"], "mark.png", { type: "image/png" }),
    );
    await canvas.findByRole("alert");
  },
};

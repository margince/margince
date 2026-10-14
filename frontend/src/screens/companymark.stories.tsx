// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { installFetchStub, StoryProviders } from "../testing/story-utils";
import { CompanyMark } from "./companymark";

// The installation's own face, first on the company page: the two marks it can
// wear, the monogram when it wears neither, and the read-only panel.

type CompanyProfile = components["schemas"]["CompanyProfile"];

const COMPANY = "00000000-0000-4000-8000-000000000010";

const WITHOUT_MARK: CompanyProfile = {
  company_id: COMPANY,
  display_name: "Brandt Automotive GmbH",
};

// Data-URI stand-ins, because Storybook serves no object store. The shapes are
// the case: a 4:1 wordmark in the wide tile, a 1:1 badge in the square one.
const WORDMARK =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 700 200'%3E%3Crect width='700' height='200' fill='white'/%3E%3Ctext x='350' y='125' text-anchor='middle' font-family='sans-serif' font-size='92' font-weight='700' fill='%23ff6500'%3EGRADION%3C/text%3E%3C/svg%3E";
const BADGE =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 200 200'%3E%3Crect width='200' height='200' rx='36' fill='%23ff6500'/%3E%3Ctext x='100' y='142' text-anchor='middle' font-family='sans-serif' font-size='128' font-weight='700' fill='white'%3EG%3C/text%3E%3C/svg%3E";

const BOTH: CompanyProfile = {
  ...WITHOUT_MARK,
  logo_url: WORDMARK,
  logo_icon_url: BADGE,
};

const meta = {
  title: "Settings/Company/Company profile/Brand",
  component: CompanyMark,
  parameters: { layout: "padded" },
  render: (args) => (
    <StoryProviders>
      <CompanyMark {...args} />
    </StoryProviders>
  ),
} satisfies Meta<typeof CompanyMark>;
export default meta;

type Story = StoryObj<typeof meta>;

// Two empty tiles in their own shapes, each with the initials inside it.
export const NoMarkYet: Story = {
  args: { profile: WITHOUT_MARK, canEdit: true },
};

// The ordinary case for an installation whose website read resolved a logo: the
// wide tile filled, the square one still empty.
export const WideMarkOnly: Story = {
  args: { profile: { ...WITHOUT_MARK, logo_url: WORDMARK }, canEdit: true },
};

export const BothMarks: Story = {
  args: { profile: BOTH, canEdit: true },
};

// Every derived colour follows the dark token lift, so a tile can read on light
// and lose contrast on dark with no test noticing.
export const BothMarksDark: Story = {
  ...BothMarks,
  globals: { theme: "dark" },
};

// At 390px the wide tile gives up width so the pair stays on one row.
export const BothMarksPhone: Story = {
  ...BothMarks,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// Read-only: the marks are facts about the company, so they are still shown.
// The verbs and the drop target go, and the panel says why once.
export const NotYoursToChange: Story = {
  args: { profile: BOTH, canEdit: false, readOnly: true },
};

function refuseUploads(...paths: string[]) {
  const refuse = () =>
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
    );
  installFetchStub(
    Object.fromEntries(paths.map((path) => [`POST ${path}`, refuse])),
  );
}

async function refuseUpload(canvasElement: HTMLElement, slotName: string) {
  const region = await within(canvasElement).findByRole("region", {
    name: slotName,
  });
  const tile = region.querySelector<HTMLInputElement>("input[type=file]");
  if (tile === null) {
    throw new Error(`the ${slotName} tile takes no file`);
  }
  await userEvent.upload(
    tile,
    new File(["not an image"], "mark.png", { type: "image/png" }),
  );
}

// The server judges an image: the picker filters on media type only. The
// refusal sits below the tiles, so the pair stays on one row.
export const TheServerRefusesTheWideLogo: Story = {
  args: { profile: WITHOUT_MARK, canEdit: true },
  render: (args) => {
    refuseUploads("/company/logo");
    return (
      <StoryProviders>
        <CompanyMark {...args} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await refuseUpload(canvasElement, "Wide logo");
    await within(canvasElement).findByText("Wide logo not uploaded");
  },
};

export const TheServerRefusesTheSquareIcon: Story = {
  ...TheServerRefusesTheWideLogo,
  render: (args) => {
    refuseUploads("/company/logo/icon");
    return (
      <StoryProviders>
        <CompanyMark {...args} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await refuseUpload(canvasElement, "Square icon");
    await within(canvasElement).findByText("Square icon not uploaded");
  },
};

export const TheServerRefusesBothImages: Story = {
  ...TheServerRefusesTheWideLogo,
  render: (args) => {
    refuseUploads("/company/logo", "/company/logo/icon");
    return (
      <StoryProviders>
        <CompanyMark {...args} />
      </StoryProviders>
    );
  },
  play: async ({ canvasElement }) => {
    await refuseUpload(canvasElement, "Square icon");
    await refuseUpload(canvasElement, "Wide logo");
    await within(canvasElement).findByText("Wide logo not uploaded");
    await within(canvasElement).findByText("Square icon not uploaded");
  },
};

export const TheServerRefusesBothImagesPhone: Story = {
  ...TheServerRefusesBothImages,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

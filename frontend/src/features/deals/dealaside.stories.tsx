// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { Locale } from "../../i18n";
import { meFixture } from "../../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DealAside } from "./dealaside";
import type { Deal, Stage } from "./deals";

// The deal's details column. Its registration group says who recorded the
// deal; "Source" there would read as where the lead came from.
const meta = {
  title: "Records/Deal 360/Details column",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const deal: Deal = {
  id: "d1",
  name: "Fleet retrofit",
  amount_minor: 4_800_000,
  currency: "EUR",
  pipeline_id: "pl",
  stage_id: "s1",
  status: "open",
  source: "manual",
  captured_by: "human:u1",
  created_at: "2026-06-01T00:00:00Z",
  updated_at: "2026-06-01T00:00:00Z",
};

const stages: Stage[] = [
  {
    id: "s1",
    pipeline_id: "pl",
    name: "Qualify",
    position: 1,
    semantic: "open",
    win_probability: 20,
  },
];

function Column({ locale }: Readonly<{ locale: Locale }>) {
  installFetchStub({
    "GET /me": () => jsonResponse(meFixture({ allow: { deal: ["read"] } })),
  });
  return (
    <StoryProviders locale={locale}>
      <DealAside deal={deal} stages={stages} />
    </StoryProviders>
  );
}

async function expectRegistrationTerms(
  canvasElement: HTMLElement,
  group: string,
  recordedBy: string,
  source: string,
) {
  const registration = await within(canvasElement).findByRole("region", {
    name: group,
  });
  const terms = within(registration)
    .getAllByRole("term")
    .map((term) => term.textContent);
  await expect(terms).toContain(recordedBy);
  await expect(terms).not.toContain(source);
}

export const RecordedBy: Story = {
  name: "Registration names who recorded it",
  render: () => <Column locale="en" />,
  play: async ({ canvasElement }) => {
    await expectRegistrationTerms(
      canvasElement,
      "Registration",
      "Recorded by",
      "Source",
    );
  },
};

export const RecordedByGerman: Story = {
  name: "Registration names who recorded it, German",
  render: () => <Column locale="de" />,
  play: async ({ canvasElement }) => {
    await expectRegistrationTerms(
      canvasElement,
      "Registrierung",
      "Erfasst von",
      "Quelle",
    );
  },
};

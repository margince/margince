// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { Locale } from "../../i18n";
import { en as englishCopy } from "../../i18n/en";
import { listsMe } from "../../testing/lists.fixtures";
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

function Column({
  locale,
  listsOn = false,
}: Readonly<{ locale: Locale; listsOn?: boolean }>) {
  installFetchStub({
    "GET /me": listsOn
      ? listsMe(true, [], {
          allow: { deal: ["read"], list: ["read", "create", "update"] },
        })
      : () => jsonResponse(meFixture({ allow: { deal: ["read"] } })),
    "GET /records/deal/d1/lists": () =>
      jsonResponse({ data: [{ id: "s-1", name: "Launch references" }] }),
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

// On one Shortlist, for a seat that may change lists: the chip, Add to
// Shortlist and the way to every list.
export const Lists: Story = {
  name: "Lists, with Add to Shortlist",
  render: () => <Column locale="en" listsOn />,
  play: async ({ canvasElement }) => {
    const lists = within(
      await within(canvasElement).findByRole("region", { name: "Lists" }),
    );
    await expect(
      await lists.findByRole("button", {
        name: englishCopy["lists.addToShortlist"],
      }),
    ).toBeVisible();
  },
};

const lowered: Deal = {
  ...deal,
  stage_id: "s2",
  writable: true,
  version: 3,
  win_probability: 35,
  win_probability_source: "margince",
  stage_win_probability: 50,
  // The signed-in member of meFixture: only the owner reads Margince's reason.
  owner_id: "00000000-0000-4000-8000-000000000001",
};

const proposal: Stage[] = [
  {
    id: "s2",
    pipeline_id: "pl",
    name: "Proposal",
    position: 2,
    semantic: "open",
    win_probability: 50,
  },
];

function LoweredColumn() {
  installFetchStub({
    "GET /me": () =>
      jsonResponse(meFixture({ allow: { deal: ["read", "update"] } })),
    "GET /deals/d1/margince-changes": () =>
      jsonResponse({
        changes: [
          {
            id: "r1",
            kind: "deal_probability",
            summary: "Changed the win probability: 50% → 35%",
            reason: "No reply from Dana Ruiz since 2 October.",
            occurred_at: "2026-11-01T02:00:00Z",
          },
        ],
      }),
    "GET /records/deal/d1/tags": () =>
      jsonResponse({
        data: [
          {
            tag_id: "t-1",
            name: "Fleet",
            archived: false,
            assigned_at: "2026-01-02T00:00:00Z",
          },
        ],
        withheld: false,
      }),
  });
  return (
    <StoryProviders locale="en">
      <DealAside deal={lowered} stages={proposal} />
    </StoryProviders>
  );
}

// The ring and its source on one line, and the reason Margince recorded in
// the tip beside them.
export const ProbabilityReason: Story = {
  name: "Probability says why",
  render: () => <LoweredColumn />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("button", { name: "Edit About" });
    canvas
      .getByRole("button", { name: englishCopy["deal.winProbability.why"] })
      .focus();
    await expect(
      await within(document.body).findByText(/No reply from Dana Ruiz/),
    ).toBeTruthy();
  },
};

// One Edit per section for a seat that may change the deal; Value opens with
// the money and nothing else.
export const EditValue: Story = {
  name: "Edit opens the section's fields",
  render: () => <LoweredColumn />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("button", { name: "Edit About" });
    await canvas.findByRole("button", { name: "Edit Ownership" });
    await canvas.findByRole("button", { name: "Edit Registration" });
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit Value" }),
    );
    const value = within(
      await within(document.body).findByRole("dialog", { name: "Value" }),
    );
    await waitFor(() =>
      expect(value.getByRole("textbox", { name: "Value" })).toBeVisible(),
    );
    await expect(
      value.queryByRole("spinbutton", { name: "Probability" }),
    ).toBeNull();
  },
};

// The Tags row reads the tags; Registration's dialog leaves for their own editor.
export const TagsFlow: Story = {
  name: "Tags change through Registration",
  render: () => <LoweredColumn />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByText("Fleet");
    await expect(canvas.queryByRole("button", { name: "Add tag" })).toBeNull();
    await userEvent.click(
      await canvas.findByRole("button", { name: "Edit Registration" }),
    );
    const body = within(document.body);
    const registration = within(
      await body.findByRole("dialog", {
        name: englishCopy["record.registration"],
      }),
    );
    await waitFor(() => expect(registration.getByText("Fleet")).toBeVisible());
    await userEvent.click(
      registration.getByRole("button", { name: "Change Tags" }),
    );
    const editor = await body.findByRole("dialog", { name: "Tags" });
    await waitFor(() => expect(editor).toBeVisible());
    await expect(
      within(editor).getByRole("button", { name: "Add tag" }),
    ).toBeVisible();
  },
};

export const TagsFlowPhone: Story = { ...TagsFlow, tags: ["uat-phone"] };

// A seat that may only read the deal gets no Edit at all.
export const ReadOnly: Story = {
  name: "No Edit for a reader",
  render: () => <Column locale="en" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findByRole("region", { name: "About" });
    await expect(canvas.queryByRole("button", { name: /^Edit / })).toBeNull();
  },
};

export const ProbabilityReasonDark: Story = {
  ...ProbabilityReason,
  name: "Probability says why, dark",
  globals: { theme: "dark" },
};

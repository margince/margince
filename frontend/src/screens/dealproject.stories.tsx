// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DealProjectChip, StartDeliveryPrompt } from "./dealproject";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The delivery half of a deal: which project it feeds, and the one offer a won
// deal gets when its company has exactly one open project to feed.
//
// The three chip states are three different facts and they are easy to draw
// alike: a project the reader can open, no project at all, and a project that
// exists but is not theirs to see. The last one is why the chip slot survives a
// null id — drawing nothing would have said "no project", which is a claim about
// the deal rather than about the reader's grants.

const meta: Meta = {
  title: "Records/Deal/Project",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Deal = components["schemas"]["Deal"];
type Project = components["schemas"]["Project"];

const project: Project = {
  id: "pr-1",
  name: "Spare parts portal",
  phase: "planned",
  version: 2,
} as unknown as Project;

function deal(over: Partial<Deal>): Deal {
  return {
    id: "d-1",
    name: "Depot rollout",
    organization_id: "o-1",
    status: "open",
    project_id: null,
    version: 3,
    ...over,
  } as unknown as Deal;
}

function Chip({ deal: subject }: Readonly<{ deal: Deal }>) {
  installFetchStub({
    "GET /projects/pr-1": () => jsonResponse(project),
  });
  return (
    <StoryProviders>
      <DealProjectChip deal={subject} />
    </StoryProviders>
  );
}

export const ChipLinked: Story = {
  render: () => <Chip deal={deal({ project_id: "pr-1" })} />,
};

// A withheld project: the id is null and `masked_fields` names it, so the chip
// stays and carries the mask rather than disappearing into the same blank a
// deal with no project draws.
export const ChipWithheld: Story = {
  render: () => (
    <Chip deal={deal({ project_id: null, masked_fields: ["project_id"] })} />
  ),
};

// Nothing to draw, which is the correct rendering and an unexplained empty
// canvas — so the story says which of the two blanks this is.
export const ChipAbsent: Story = {
  render: () => (
    <div>
      <p className="t-caption">
        No project on this deal — the chip renders nothing.
      </p>
      <Chip deal={deal({})} />
    </div>
  ),
};

function Prompt({
  deal: subject,
  projects,
}: Readonly<{ deal: Deal; projects: Project[] }>) {
  installFetchStub({
    "GET /projects": () =>
      jsonResponse({
        data: projects,
        page: { next_cursor: null, has_more: false },
      }),
    "GET /projects/pr-1": () => jsonResponse(project),
  });
  return (
    <StoryProviders>
      <StartDeliveryPrompt deal={subject} />
    </StoryProviders>
  );
}

export const StartDeliveryOffered: Story = {
  render: () => <Prompt deal={deal({ status: "won" })} projects={[project]} />,
};

// The retry state: the deal already names the project, so only the move into
// delivery is still owed and the offer says so.
export const StartDeliveryAttached: Story = {
  render: () => (
    <Prompt
      deal={deal({ status: "won", project_id: "pr-1" })}
      projects={[project]}
    />
  ),
};

// Two open projects is a choice, not an offer: the prompt stands down and the
// reader picks on the edit form.
export const StartDeliverySilentOnTwo: Story = {
  render: () => (
    <div>
      <p className="t-caption">
        Two open projects — the offer stands down, because attaching to either
        would be a guess.
      </p>
      <Prompt
        deal={deal({ status: "won" })}
        projects={[
          project,
          { ...project, id: "pr-2", name: "Second site" } as Project,
        ]}
      />
    </div>
  ),
};

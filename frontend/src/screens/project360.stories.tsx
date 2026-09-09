// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { PageAsideProvider } from "../app/pageaside";
import { ProjectScreen } from "./project360";
import { COMPANY, project, project360 } from "./projects.fixtures";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The project page, in the states a reader can meet it in: every section
// present, two sections withheld from the reader's role, a closed project, and
// the details pane folded away. The list that leads here is the other screen
// and stays in `projects.stories.tsx`; this file is the page's only entry, so
// the catalog cannot grow two readings of one screen.
//
// Every state is drawn from `projects.fixtures.ts`, the 360 the project suites
// build from — a story that hand-rolled a second one would be a second answer
// to what a project looks like, and the two would drift.
//
// What to read under the header is the ROW: the tab strip carries the details
// switch at its end, which is where every other record page keeps it.

const meta: Meta<typeof ProjectScreen> = {
  title: "Records/Project/Screen",
  component: ProjectScreen,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProjectScreen>;

// Every read the page and its cards make, routed in one place. An unrouted path
// does not fail — `installFetchStub` answers it with an empty page — so a story
// missing a route renders green with silently empty cards, which is exactly the
// defect the catalog exists to catch.
function installPage(view: unknown) {
  installFetchStub({
    // The grants the page's verbs ask before they draw — a rep working their
    // own projects, which is the seat the fixture's `writable: true` describes.
    // Without them the page draws its refused branch, which the record suite
    // asserts and no story here is named for.
    "GET /me": meRoute({
      project: ["read", "create", "update", "delete"],
      deal: ["read", "create"],
    }),
    "GET /projects/pr-1/360": () => jsonResponse(view),
    "GET /projects/pr-1": () => jsonResponse(project()),
    "GET /field-history": () => jsonResponse(emptyPage),
    "GET /pipelines": () => jsonResponse(emptyPage),
    "GET /companies": () =>
      jsonResponse({ data: [COMPANY], page: { next_cursor: null } }),
    [`GET /companies/${COMPANY.id}`]: () => jsonResponse(COMPANY),
    "GET /users": () =>
      jsonResponse({
        data: [{ id: "u-me", display_name: "Me", status: "active" }],
        page: { next_cursor: null },
      }),
  });
}

// The pane stands open here without a provider of its own: `StoryProviders`
// mounts the shell's `RecordShell`, whose provider is already open. Only the
// folded story is about the other setting, so only it carries its own.
function page(view: unknown) {
  return () => {
    installPage(view);
    return (
      <StoryProviders>
        <ProjectScreen id="pr-1" />
      </StoryProviders>
    );
  };
}

/** Every section present, the details pane open beside the work. */
export const Page: Story = {
  render: page(
    project360({
      deals: {
        data: [
          {
            id: "d-1",
            name: "Phase one licence",
            pipeline_id: "pl",
            stage_id: "s3",
            status: "won",
            amount_minor: 450_000,
            currency: "EUR",
            source: "manual",
            captured_by: "u-me",
            created_at: "2026-06-02T09:00:00Z",
            updated_at: "2026-06-02T09:00:00Z",
          },
          {
            id: "d-2",
            name: "Phase two services",
            pipeline_id: "pl",
            stage_id: "s1",
            status: "open",
            amount_minor: 1_200_000,
            currency: "EUR",
            source: "manual",
            captured_by: "u-me",
            created_at: "2026-07-02T09:00:00Z",
            updated_at: "2026-07-02T09:00:00Z",
          },
        ],
        page: { next_cursor: null, has_more: false },
      },
      commitments: {
        data: [
          {
            activity_id: "a-1",
            subject: "Send the kickoff agenda",
            due_at: "2026-07-03T09:00:00Z",
            assignee_id: "u-me",
            assignee_name: "Me",
            overdue: true,
          },
        ],
        page: { next_cursor: null, has_more: false },
      },
    }),
  ),
};

// A reader whose role holds the project grant but not the contracts or the
// deal figures: the cards say so rather than reading as empty.
export const PageWithheld: Story = {
  render: page(
    project360({
      sections_omitted: ["contracts", "rollups", "deals"],
      contracts: undefined,
      rollups: undefined,
      deals: undefined,
    }),
  ),
};

export const PageClosed: Story = {
  render: page(
    project360({
      project: project({ phase: "closed", closed_reason: "Delivered" }),
      phase_history: {
        data: [
          {
            id: "ph-1",
            from_phase: null,
            to_phase: "initiative",
            reason: null,
            changed_at: "2026-03-01T09:00:00Z",
            changed_by: { id: "u-me", display_name: "Me" },
          },
          {
            id: "ph-2",
            from_phase: "initiative",
            to_phase: "delivering",
            reason: "Contract signed",
            changed_at: "2026-04-01T09:00:00Z",
            changed_by: { id: "u-me", display_name: "Me" },
          },
          {
            id: "ph-3",
            from_phase: "delivering",
            to_phase: "closed",
            reason: "Delivered",
            changed_at: "2026-07-01T09:00:00Z",
            changed_by: { id: "u-me", display_name: "Me" },
          },
        ],
        phase_durations: [
          { phase: "initiative", seconds: 2_678_400, current: false },
          { phase: "delivering", seconds: 7_862_400, current: false },
          { phase: "closed", seconds: 86_400, current: true },
        ],
      },
    }),
  ),
};

/** Folded away: the work takes the whole width and the switch reads "show". */
export const DetailsFolded: Story = {
  render: () => {
    installPage(project360());
    return (
      <StoryProviders>
        <PageAsideProvider open={false}>
          <ProjectScreen id="pr-1" />
        </PageAsideProvider>
      </StoryProviders>
    );
  },
};

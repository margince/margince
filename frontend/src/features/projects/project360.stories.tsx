// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { ToastProvider, ToastRegion } from "../../design-system/toast";
import { en } from "../../i18n/en";
import { COMPANY, project, project360 } from "../../testing/projects.fixtures";
import {
  emptyPage,
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ProjectScreen } from "./project360";

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
  title: "Records/Project 360/Page",
  component: ProjectScreen,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProjectScreen>;

// Every read the page and its cards make, routed in one place. An unrouted path
// does not fail — `installFetchStub` answers it with an empty page — so a story
// missing a route renders green with silently empty cards, which is exactly the
// defect the catalog exists to catch.
function installPage(view: unknown, seat?: Parameters<typeof meRoute>[0]) {
  installFetchStub({
    // The grants the page's verbs ask before they draw — a rep working their
    // own projects, which is the seat the fixture's `writable: true` describes.
    // Without them the page draws its refused branch, which the record suite
    // asserts and no story here is named for.
    "GET /me": meRoute(
      seat ?? {
        project: ["read", "create", "update", "delete"],
        deal: ["read", "create"],
      },
    ),
    "PATCH /activities/a-1": () => jsonResponse({ id: "a-1", version: 2 }),
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
      // A target end, so the head's facts strip pictures the cell that reads
      // it. The fixture's project carries none, which is the strip's OTHER
      // state and the one PageWithheld and PageClosed below still draw.
      project: project({ target_end_date: "2026-11-30" }),
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
            due_has_time: false,
            version: 1,
            overdue: true,
          },
        ],
        page: { next_cursor: null, has_more: false },
      },
    }),
  ),
};

// Snooze on the Next bar confirms with the new deadline and an Undo.
export const SnoozedWithUndo: Story = {
  render: () => {
    installPage(
      project360({
        commitments: {
          data: [
            {
              activity_id: "a-1",
              subject: "Send the kickoff agenda",
              due_at: "2027-07-03T09:00:00Z",
              assignee_id: "u-me",
              assignee_name: "Me",
              due_has_time: true,
              version: 1,
              overdue: false,
            },
          ],
          page: { next_cursor: null, has_more: false },
        },
      }),
      {
        project: ["read", "update"],
        activity: ["read", "update"],
      },
    );
    return (
      <StoryProviders>
        <ToastProvider>
          <ProjectScreen id="pr-1" />
          <ToastRegion />
        </ToastProvider>
      </StoryProviders>
    );
  },
  play: async () => {
    await userEvent.click(
      await screen.findByRole("button", { name: /^Snooze/ }),
    );
    await expect(
      await screen.findByRole("button", { name: en["common.undo"] }),
    ).toBeTruthy();
  },
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
            changed_by: {
              actor_type: "human",
              actor_id: "human:u-me",
              actor_name: "Me",
            },
          },
          {
            id: "ph-2",
            from_phase: "initiative",
            to_phase: "delivering",
            reason: "Contract signed",
            changed_at: "2026-04-01T09:00:00Z",
            changed_by: {
              actor_type: "agent",
              actor_id: "agent:p-1",
              actor_name: null,
              on_behalf_of: "u-me",
              on_behalf_of_name: "Me",
              agent_client: "Claude",
              passport_id: "p-1",
            },
          },
          {
            id: "ph-3",
            from_phase: "delivering",
            to_phase: "closed",
            reason: "Delivered",
            changed_at: "2026-07-01T09:00:00Z",
            // no changed_by: the audit link is unknown,
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

/** A project a won deal moved into delivery: its history names the deal. */
export const PageMovedByWin: Story = {
  render: page(
    project360({
      project: project({ phase: "delivering" }),
      phase_history: {
        data: [
          {
            id: "ph-1",
            from_phase: null,
            to_phase: "pursuing",
            reason: null,
            changed_at: "2026-03-01T09:00:00Z",
          },
          {
            id: "ph-2",
            from_phase: "pursuing",
            to_phase: "delivering",
            reason: null,
            changed_at: "2026-04-01T09:00:00Z",
            changed_by: {
              actor_type: "human",
              actor_id: "human:u-me",
              actor_name: "Me",
            },
            cause: {
              kind: "deal_won",
              deal_id: "d-1",
              deal_name: "Phase one licence",
            },
          },
        ],
        phase_durations: [
          { phase: "pursuing", seconds: 2_678_400, current: false },
          { phase: "delivering", seconds: 7_862_400, current: true },
        ],
      },
    }),
  ),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("link", {
      name: "Phase one licence",
    });
  },
};

/** All tasks opens the history narrowed to tasks, under its own title. */
export const AllTasksDrawer: Story = {
  render: page(
    project360({
      commitments: {
        data: [
          {
            activity_id: "a-1",
            subject: "Send the kickoff agenda",
            due_at: null,
            assignee_id: "u-me",
            assignee_name: "Me",
            due_has_time: false,
            version: 1,
            overdue: false,
          },
        ],
        page: { next_cursor: null, has_more: false },
      },
    }),
  ),
  play: async ({ canvasElement }) => {
    await userEvent.click(
      await within(canvasElement).findByRole("button", {
        name: en["project.commitments.all"],
      }),
    );
    await screen.findByRole("dialog", { name: en["project.commitments.all"] });
  },
};

// The menu's Edit project opens the project's fields in the same dialog the
// details sections save through.
export const EditProject: Story = {
  render: page(project360()),
  play: async ({ canvasElement }) => {
    const user = userEvent.setup();
    const canvas = within(canvasElement);
    await user.click(
      await canvas.findByRole("button", { name: en["record.moreActions"] }),
    );
    await user.click(
      await screen.findByRole("button", { name: en["project.edit"] }),
    );
    const dialog = await screen.findByRole("dialog", {
      name: en["project.edit"],
    });
    await waitFor(() => expect(dialog).toBeVisible());
    await expect(
      within(dialog).getByRole("textbox", { name: /^Project name/ }),
    ).toHaveValue("CRM rollout");
  },
};

// The server refuses an archived project's 360, so the page draws from the row:
// the archived sentence, and the history the shell hands in (none here).
function refusedPage(row: unknown) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({ project: ["read", "update"] }),
      "GET /projects/pr-1/360": () =>
        jsonResponse({ title: "Not found", status: 404 }, 404),
      "GET /projects/pr-1": () =>
        row
          ? jsonResponse(row)
          : jsonResponse({ title: "Not found", status: 404 }, 404),
    });
    return (
      <StoryProviders>
        <ProjectScreen id="pr-1" />
      </StoryProviders>
    );
  };
}

/** An archived project, opened from Show archived on the list. */
export const Archived: Story = {
  render: refusedPage(project({ archived_at: "2026-11-01T09:00:00Z" })),
};

/** An address that names no project the reader can see. */
export const NotFound: Story = { render: refusedPage(null) };

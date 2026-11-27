// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import type { components } from "../../api/schema";
import {
  TaskCompleteCheck,
  TaskQuickActions,
  useTaskUpdate,
} from "../../components/taskactions";
import { taskWriteKeys } from "../../lib/activitykeys";
import { company360, emptySection } from "../../testing/company.fixtures";
import { recordNamesRoutes } from "../../testing/recordnames.testkit";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { CompanyTaskList } from "./companytasklist";

// The company's Tasks tab as one list: the open tasks, what Margince
// suggests, and the signals. Check both themes and the phone story: the
// suggestion verbs move under their claim at the fold.

const meta: Meta<typeof CompanyTaskList> = {
  title: "Records/Company 360/Task list",
  component: CompanyTaskList,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof CompanyTaskList>;

type Company360 = components["schemas"]["Company360"];
type Signal = components["schemas"]["Signal"];

const busy: Company360 = {
  ...company360,
  as_of: "2026-08-13T09:00:00Z",
  next_steps: {
    ...emptySection,
    data: [
      {
        activity_id: "a-1",
        subject: "Send Dana the signed renewal",
        body: "Dana asked for the signed renewal before their buying round on 15 August.",
        due_at: "2026-08-01T12:00:00Z",
        overdue: true,
        task_type: "reach_out",
        linked_contact_id: "p-1",
        linked_deal_id: "d-1",
      },
      {
        activity_id: "a-2",
        subject: "Book the autumn range review",
        body: "On the 3 August call, Dana agreed to look at the autumn samples together once they arrive.",
        due_at: "2026-08-15T12:00:00Z",
        overdue: false,
        task_type: "meet",
      },
    ],
  },
  suggestions: [
    {
      kind: "no_next_step",
      title: 'Agree the next step on "Fleet retrofit 2026"',
      reason:
        '"Fleet retrofit 2026" has been in Proposal for 41 days and nothing is planned after the price list.',
      fingerprint: "fp-1",
      evidence: [{ entity_type: "deal", entity_id: "d-1" }],
      action: {
        kind: "add_task",
        deal_id: "d-1",
        task: {
          subject: 'Agree the next step on "Fleet retrofit 2026"',
          source: "manual",
          links: [{ entity_type: "deal", entity_id: "d-1" }],
        },
      },
    },
  ],
};

const quiet: Company360 = {
  ...company360,
  as_of: "2026-08-13T09:00:00Z",
  next_steps: { ...emptySection, data: [] },
  suggestions: [
    {
      kind: "gone_dormant",
      title: "Get back in touch with Brandt Automotive",
      reason: "Nothing has happened with this company for 199 days.",
      fingerprint: "fp-2",
      evidence: [{ entity_type: "company", entity_id: "o-1" }],
      action: {
        kind: "add_task",
        task: {
          subject: "Get back in touch with Brandt Automotive",
          source: "manual",
          links: [{ entity_type: "company", entity_id: "o-1" }],
        },
      },
    },
  ],
};

const signals: Signal[] = [
  {
    id: "sg-1",
    kind: "commitment_made",
    source_channel: "derived",
    resolution_state: "resolved",
    severity: "warn",
    summary:
      "Dana promised to confirm the depot slot once facilities sign off.",
    detected_at: "2026-08-10T09:00:00Z",
    entity_type: "company",
    entity_id: "o-1",
    status: "open",
    evidence: [],
    source: "derived",
    captured_by: "system",
    created_at: "2026-08-10T09:00:00Z",
    updated_at: "2026-08-10T09:00:00Z",
  },
];

function List({
  view,
  openSignals,
}: Readonly<{ view: Company360; openSignals: Signal[] }>) {
  installFetchStub({
    "GET /me": meRoute({ company: ["read", "update"] }),
    "GET /signals": () =>
      jsonResponse({ data: openSignals, page: emptySection.page }),
    "GET /deal-suggestions": () =>
      jsonResponse({ data: [], page: emptySection.page }),
    ...recordNamesRoutes({
      contact: { "p-1": "Dana Ronkin" },
      deal: { "d-1": "Fleet retrofit 2026" },
    }),
  });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 880 }}>
        <WithVerbs view={view} />
      </div>
    </StoryProviders>
  );
}

// The tick and the row verbs the Tasks tab hands in, inside the providers
// the update hook reads.
function WithVerbs({ view }: Readonly<{ view: Company360 }>) {
  const update = useTaskUpdate(taskWriteKeys("company", "o-1"));
  return (
    <CompanyTaskList
      companyId="o-1"
      view={view}
      onOpenTask={() => undefined}
      renderTick={(step) => (
        <TaskCompleteCheck
          activityId={step.activity_id}
          subject={step.subject}
          version={step.version}
          update={update}
        />
      )}
      renderAction={(step) => (
        <TaskQuickActions
          activityId={step.activity_id}
          version={step.version}
          dueAt={step.due_at}
          update={update}
          showComplete={false}
        />
      )}
      onPrepareMeeting={() => undefined}
      onDraftTo={() => undefined}
      onOpenRecord={() => undefined}
      onOpenReceipt={() => undefined}
      onOpenEmail={() => undefined}
      onPerform={() => undefined}
    />
  );
}

export const BusyAccount: Story = {
  render: () => <List view={busy} openSignals={signals} />,
};

export const BusyAccountDark: Story = {
  ...BusyAccount,
  globals: { theme: "dark" },
};

// Nothing open and no signal: each group says so in one line, and the one
// suggestion stands alone under its heading.
export const QuietAccount: Story = {
  render: () => <List view={quiet} openSignals={[]} />,
};

export const BusyAccountOnPhone: Story = {
  ...BusyAccount,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

// A suggestion read off a call logged by its date alone. The byline names
// that date and no clock, so its stored noon never reads as the hour it
// happened.
const DATED = 'Agree the next step on "PIM rollout Phase 2"';
export const SuggestionDatedByADayEntry: Story = {
  render: () => (
    <List
      openSignals={[]}
      view={{
        ...quiet,
        suggestions: [
          {
            kind: "no_next_step",
            fingerprint: "fp-dated",
            title: DATED,
            reason:
              '"PIM rollout Phase 2" is open and no task says what happens next.',
            evidence: [{ entity_type: "deal", entity_id: "d-1" }],
            due_at: "2026-08-12T12:00:00Z",
            due_precision: "day",
          },
        ],
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const ask = await within(canvasElement).findByText(DATED);
    const row = ask.closest(".co-move") ?? canvasElement;
    await expect(row.textContent).toMatch(/12/);
    await expect(row.textContent).not.toMatch(/\d:\d\d/);
  },
};

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, within } from "storybook/test";
import { installFetchStub, jsonResponse, StoryProviders } from "../story-utils";
import { PersonMeetingBrief } from "./drawer";
import {
  briefEmpty,
  briefManager,
  briefModel,
  briefModelPlan,
  briefOmitted,
  briefReady,
  briefScoped,
  briefWithPlan,
  meetingFacts,
  preparedFor,
} from "./fixtures";

const ROUTE = "GET /activities/a-1/meeting-brief";

// The projects a meeting filed under none can be prepared for.
const PROJECTS = [
  {
    project_id: "3f7c1a90-0000-4000-8000-00000000e001",
    name: "Depot retrofit 2026",
    key: "RETRO",
    phase: "delivering" as const,
  },
];

// The drawer is a Modal, portalled to document.body, so `#storybook-root` holds
// the preview decorator and nothing else however well the brief renders — and
// declaring `layout: "fullscreen"` removes even that, which left the render gate
// watching an empty root and reporting all eighteen frames as components that
// never rendered. The frame the decorator draws sits behind a fixed overlay and
// changes no capture.
//
// The root filling is then only evidence that the DECORATOR ran, so the meta
// carries a play every frame inherits: the dialog, and the title that says which
// dialog it is. Stated once here because it is true of every state the drawer
// can be in — assembling, failed, empty and prepared alike — and a per-story
// copy would be the same two lines eighteen times.
const meta: Meta<typeof PersonMeetingBrief> = {
  title: "Records/Person record/Meeting brief",
  component: PersonMeetingBrief,
  play: async () => {
    const drawer = within(await screen.findByRole("dialog"));
    await drawer.findByRole("heading", { name: "Meeting brief" });
  },
};

export default meta;
type Story = StoryObj<typeof PersonMeetingBrief>;

// One helper rather than a stub literal per story: what varies between these
// stories is the ANSWER, and spelling the route out nine times invites the
// nine to drift apart.
function drawer(
  answer: () => Response,
  extra?: { projects?: typeof PROJECTS },
) {
  installFetchStub({ [ROUTE]: answer });
  return (
    <StoryProviders>
      <PersonMeetingBrief
        activityId="a-1"
        open
        onClose={() => {}}
        meeting={meetingFacts}
        preparedFor={preparedFor}
        projects={extra?.projects}
      />
    </StoryProviders>
  );
}

// The everyday brief, assembled without a model.
export const Ready: Story = {
  render: () => drawer(() => jsonResponse(briefReady)),
};

// The same facts with a model lane behind them. Every panel turns indigo and
// the lead's head carries the disclosure badge; nothing else moves.
export const ModelWritten: Story = {
  render: () => drawer(() => jsonResponse(briefModel)),
};

// The whole PLAN written by a model: the objective, the arc, the close, the
// unknowns and the ranked asks all indigo, with the badge on the objective
// alone. Read beside `WithPlan` — the same panels and the same facts, composed
// rather than written — this is the only place the claim can be checked.
export const ModelWrittenPlan: Story = {
  render: () => drawer(() => jsonResponse(briefModelPlan)),
};

export const ModelWrittenPlanDark: Story = {
  render: () => drawer(() => jsonResponse(briefModelPlan)),
  globals: { theme: "dark" },
};

// What a reader sees in the seconds before the brief arrives.
export const Loading: Story = {
  render: () => drawer(() => new Response(null, { status: 200 })),
};

// The read failed. The generic sentence carries a retry; the server's own
// sentence sits under it.
export const Failed: Story = {
  render: () =>
    drawer(() =>
      jsonResponse(
        {
          type: "about:blank",
          title: "Not found",
          status: 404,
          code: "not_found",
          detail: "That meeting is filed under a different engagement.",
        },
        404,
      ),
    ),
};

// A cold record. Not an error: nothing has been captured yet.
export const Empty: Story = {
  render: () => drawer(() => jsonResponse(briefEmpty)),
};

// A reader whose grants keep a source out of the brief.
export const Omitted: Story = {
  render: () => drawer(() => jsonResponse(briefOmitted)),
};

// A meeting filed under a project: the scope line states it and the picker
// stands down.
export const Scoped: Story = {
  render: () => drawer(() => jsonResponse(briefScoped)),
};

// A meeting filed under none, with projects to choose from.
export const Unscoped: Story = {
  render: () => drawer(() => jsonResponse(briefReady), { projects: PROJECTS }),
};

// The deterministic preparation plan, added above the sections rather than in
// place of them: an `outline` plan is not yet worth leading with, and the cited
// summary a reader already had stays in front of them.
export const WithPlan: Story = {
  render: () => drawer(() => jsonResponse(briefWithPlan)),
};

export const WithPlanDark: Story = {
  render: () => drawer(() => jsonResponse(briefWithPlan)),
  globals: { theme: "dark" },
};

export const WithPlanPhone: Story = {
  render: () => drawer(() => jsonResponse(briefWithPlan)),
  globals: { viewport: { value: "mobile1" } },
};

// What a lead sees: the same brief, with one layer on top.
export const ManagerCoaching: Story = {
  render: () => drawer(() => jsonResponse(briefManager)),
};

export const ManagerCoachingDark: Story = {
  render: () => drawer(() => jsonResponse(briefManager)),
  globals: { theme: "dark" },
};

export const ManagerCoachingPhone: Story = {
  render: () => drawer(() => jsonResponse(briefManager)),
  globals: { viewport: { value: "mobile1" } },
};

export const Dark: Story = {
  render: () => drawer(() => jsonResponse(briefReady)),
  globals: { theme: "dark" },
};

// The phone sheet: full width, its three bands still paying their own padding.
export const Phone: Story = {
  render: () => drawer(() => jsonResponse(briefReady)),
  globals: { viewport: { value: "mobile1" } },
};

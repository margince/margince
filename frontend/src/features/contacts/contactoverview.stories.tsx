import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { MoveButton } from "../records/movebutton";
import { ContactOverview } from "./contactoverview";
import { ContactPageV2 } from "./contactpage";
import { ContactRail } from "./contactrail";
import "./contact360.css";

const meta: Meta = {
  title: "Records/Contact 360/Clarity",
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj;
type View = components["schemas"]["Contact360"];
const sparse: View = {
  as_of: "2026-09-14T09:00:00Z",
  contact: {
    id: "p-1",
    full_name: "Giacomo Example",
    title: "Operations manager",
    source: "import",
    captured_by: "human:u-1",
    created_at: "2026-09-14T08:00:00Z",
    updated_at: "2026-09-14T08:00:00Z",
    writable: true,
    primary_email: "giacomo@example.test",
    emails: [
      {
        id: "e-1",
        contact_id: "p-1",
        email: "giacomo@example.test",
        email_type: "work",
        is_primary: true,
        position: 0,
        source: "manual",
        captured_by: "human:u-1",
      },
    ],
  },
  sections_omitted: [],
  activities: { data: [], page: { has_more: false } },
  next_steps: { data: [], page: { has_more: false } },
  network: { colleagues: [] },
  commercial: { role: null, committee: [] },
  claims: [],
  conversation_memory: [],
  moment: {
    rule: "thin_relationship",
    headline: "No interactions recorded",
    why_now: "No interactions or colleagues found in the records you can see.",
    confidence: "observed_fact",
    claim_key: "empty",
    evidence_fingerprint: "empty",
    evidence: [],
    recommended_action: {
      kind: "log_activity",
      label: "Log an interaction",
      state: "available",
      destination: { surface: "activity_log" },
    },
  },
};
function install(view: View, failed = false) {
  installFetchStub({
    "GET /me": meRoute({
      contact: ["read", "update"],
      activity: ["read", "create"],
    }),
    "GET /contacts/p-1/360": () => jsonResponse(view),
    "GET /contacts/p-1/consent/guard": () =>
      failed
        ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
        : jsonResponse({
            contact_id: "p-1",
            entries: [
              {
                purpose_key: "business",
                purpose_label: "Business correspondence",
                purpose_class: "business_correspondence",
                channel: "email",
                verdict: "unknown",
                reason: "No basis for business correspondence is recorded.",
              },
              {
                purpose_key: "transactional",
                purpose_label: "Transactional email",
                purpose_class: "transactional",
                channel: "email",
                verdict: "allowed",
                reason: "Account and service notices only.",
              },
              {
                purpose_key: "marketing",
                purpose_label: "Marketing email",
                purpose_class: "marketing",
                channel: "email",
                verdict: "unknown",
                reason: "No consent recorded.",
              },
            ],
          }),
    "GET /consent-purposes": () =>
      jsonResponse({
        data: [
          {
            id: "p1",
            key: "business",
            label: "Business correspondence",
            requires_double_opt_in: false,
          },
        ],
        page: { has_more: false },
      }),
    "GET /contacts/p-1/consent": () =>
      jsonResponse({
        state: [
          { purpose_id: "p1", purpose_key: "business", state: "unknown" },
        ],
        events: [],
      }),
  });
}
export const FreshContact: Story = {
  render: () => {
    install(sparse);
    return (
      <StoryProviders>
        <ContactPageV2 id="p-1" tab="overview" />
      </StoryProviders>
    );
  },
};
export const FailedReads: Story = {
  render: () => {
    install(sparse, true);
    return (
      <StoryProviders>
        <ContactPageV2 id="p-1" tab="overview" />
      </StoryProviders>
    );
  },
};
export const ProfileOnly: Story = {
  render: () => {
    install(sparse);
    return (
      <StoryProviders>
        <ContactOverview
          view={sparse}
          onAction={() => undefined}
          onOpenEmail={() => undefined}
          onOpenTab={() => undefined}
        />
      </StoryProviders>
    );
  },
};
export const OpenTask: Story = {
  render: () => {
    const view: View = {
      ...sparse,
      next_steps: {
        data: [
          {
            occurred_precision: "instant",
            offers_email: false,
            id: "a-1",
            kind: "task",
            subject: "Send the pricing sheet Giacomo requested",
            occurred_at: sparse.as_of,
            due_at: "2026-09-14T12:00:00Z",
            is_done: false,
            source: "manual",
            captured_by: "human:u-1",
            created_at: sparse.as_of,
            updated_at: sparse.as_of,
          },
        ],
        page: { has_more: false },
      },
    };
    install(view);
    return (
      <StoryProviders>
        <ContactOverview
          view={view}
          onAction={() => undefined}
          onOpenEmail={() => undefined}
          onOpenTab={() => undefined}
        />
      </StoryProviders>
    );
  },
};

export const FreshContactPhone: Story = {
  ...FreshContact,
  tags: ["uat-phone"],
};
export const FreshContactDark: Story = {
  ...FreshContact,
  globals: { theme: "dark" },
};

// The left column while the consent guard is out, and after it failed.
function ColumnWhileGuard(
  state: Readonly<{ loading: boolean; failed: boolean }>,
) {
  return (
    <ContactRail
      view={sparse}
      consent={{ guard: undefined, onRetry: () => undefined, ...state }}
      writeRefusalId="write-refused"
    />
  );
}

export const PermissionsLoading: Story = {
  render: () => {
    install(sparse);
    return (
      <StoryProviders>
        <ColumnWhileGuard loading failed={false} />
      </StoryProviders>
    );
  },
};
export const PermissionsFailed: Story = {
  render: () => {
    install(sparse);
    return (
      <StoryProviders>
        <ColumnWhileGuard loading={false} failed />
      </StoryProviders>
    );
  },
};
export const OpenExistingTask: Story = {
  render: () => {
    install(sparse);
    return (
      <StoryProviders>
        <MoveButton
          contactId="p-1"
          move={{ action: "open_task", arguments: { activity_id: "a-1" } }}
        />
      </StoryProviders>
    );
  },
};
export const RestrictedOverview: Story = {
  render: () => {
    const view: View = {
      ...sparse,
      moment: undefined,
      activities: undefined,
      next_steps: undefined,
      sections_omitted: [
        "activities",
        "conversation_memory",
        "last_touch",
        "claims",
        "commercial",
        "next_meeting",
        "moments",
        "next_steps",
      ],
    };
    install(view);
    return (
      <StoryProviders>
        <ContactOverview
          view={view}
          onAction={() => undefined}
          onOpenEmail={() => undefined}
          onOpenTab={() => undefined}
        />
      </StoryProviders>
    );
  },
};
// A relationship with something on it. A promise slipped, a strong band has a
// change behind it, a colleague is in touch and a meeting is booked ahead.
const promised: View = {
  ...sparse,
  as_of: "2026-08-13T09:00:00Z",
  last_inbound_at: "2026-08-01T09:00:00Z",
  last_outbound_at: "2026-07-20T09:00:00Z",
  strength: {
    score: 64,
    bucket: "strong",
    factors: {
      recency: 0.75,
      frequency: 0.6,
      reciprocity: 0.8,
      direction: 0.5,
    },
    inbound_90d: 4,
    outbound_90d: 6,
  },
  relationship_changes: [
    {
      kind: "warmed",
      at: "2026-07-20T09:00:00Z",
      from_bucket: "moderate",
      to_bucket: "strong",
    },
  ],
  network: {
    colleagues: [
      {
        user_id: "u-2",
        display_name: "Lena Fischer",
        strength_bucket: "strong",
        interactions_90d: 5,
      },
    ],
  },
  claims: [
    {
      id: "c-1",
      kind: "commitment_ours",
      body: "Send the updated retrofit quote",
      source_activity_id: "a-1",
      occurred_precision: "instant",
      source_quote: "We will send the updated quote by the 8th.",
      status: "open",
      needs_review: false,
      due_at: "2026-06-08T09:00:00Z",
    },
  ],
  next_meeting: {
    activity_id: "m-2",
    starts_at: "2026-08-20T09:00:00Z",
    occurred_precision: "instant",
    subject: "Walkthrough",
  },
  moment: {
    rule: "overdue_promise",
    headline: "Send Giacomo the updated retrofit quote",
    why_now: "Promised on the 3 Jun kickoff call, for a date 66 days ago.",
    confidence: "observed_fact",
    claim_key: "promise:c-1",
    evidence_fingerprint: "c-1",
    evidence: [{ type: "activity", id: "a-1", label: "Kickoff call" }],
    recommended_action: {
      kind: "draft_reply",
      label: "Write the quote email",
      state: "available",
      destination: { surface: "composer" },
    },
  },
};
export const PromiseOverdue: Story = {
  render: () => {
    install(promised);
    return (
      <StoryProviders>
        <ContactOverview
          view={promised}
          onAction={() => undefined}
          onOpenEmail={() => undefined}
          onOpenTab={() => undefined}
        />
      </StoryProviders>
    );
  },
};

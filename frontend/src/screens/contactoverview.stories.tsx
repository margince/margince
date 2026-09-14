import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { ConsentAndChannels } from "./contactconsentpanel";
import { ContactOverview } from "./contactoverview";
import { ContactPageV2 } from "./contactpage";
import { MoveButton } from "./movebutton";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import "./contact360.css";

const meta: Meta = {
  title: "Records/Contact record/Clarity",
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
    why_now:
      "No interactions or colleague connections were found in the records available to you.",
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
const brief: components["schemas"]["ContactBrief"] = {
  contact_id: "p-1",
  generated_at: sparse.as_of,
  generated_by: "deterministic",
  sentences: [
    {
      text: "Giacomo is an operations manager. His contact was added from an import; there are no captured conversations yet.",
      evidence: [{ entity_type: "contact", entity_id: "p-1" }],
    },
  ],
};
function install(view: View, failed = false) {
  installFetchStub({
    "GET /me": meRoute({
      contact: ["read", "update"],
      activity: ["read", "create"],
    }),
    "GET /contacts/p-1/360": () => jsonResponse(view),
    "GET /contacts/p-1/brief": () =>
      failed
        ? jsonResponse({ title: "Unavailable", status: 503 }, 503)
        : jsonResponse(brief),
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
          brief={undefined}
          briefLoading={false}
          briefFailed={false}
          onRetryBrief={() => undefined}
          onAction={() => undefined}
          onOpenEmail={() => undefined}
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
          brief={brief}
          briefLoading={false}
          briefFailed={false}
          onRetryBrief={() => undefined}
          onAction={() => undefined}
          onOpenEmail={() => undefined}
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

export const PermissionsLoading: Story = {
  render: () => {
    install(sparse);
    return (
      <StoryProviders>
        <ConsentAndChannels view={sparse} guard={undefined} loading />
      </StoryProviders>
    );
  },
};
export const PermissionsFailed: Story = {
  render: () => {
    install(sparse);
    return (
      <StoryProviders>
        <ConsentAndChannels
          view={sparse}
          guard={undefined}
          failed
          onRetry={() => undefined}
        />
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
          brief={brief}
          briefLoading={false}
          briefFailed={false}
          onRetryBrief={() => undefined}
          onAction={() => undefined}
          onOpenEmail={() => undefined}
        />
      </StoryProviders>
    );
  },
};

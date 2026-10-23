// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import type { Locale } from "../../i18n";
import { de } from "../../i18n/de";
import {
  approvalRowSubject as approval,
  ApprovalRowStory as Row,
} from "../../testing/approval-row-story";
import { commercialApproval } from "../../testing/commercial-approval";
import { messageApproval } from "../../testing/message-approval";
import { jsonResponse } from "../../testing/story-utils";

// One staged proposal as a decidable row — the canonical affordance for
// anything an agent proposes and a human decides. Several surfaces draw this
// row (the workspace queue, Brief, the company record), so it is the one place
// the states below are worth reading side by side.
//
// The two error states are the ones that matter and they are NOT the same
// answer: a version skew says the world moved under the proposal and it must be
// re-staged, while an already-decided says somebody else got there first and the
// row itself is stale. Offering a retry on the second would invite a second
// decision on a settled question.

const meta: Meta = {
  title: "Patterns/Decision row",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;
type Approval = components["schemas"]["Approval"];

export const CompleteMessage: Story = {
  render: () => <Row subject={messageApproval()} />,
};

export const CompleteMessageGerman: Story = {
  render: () => <Row subject={messageApproval()} locale="de" />,
};

export const CompleteMessageVietnamese: Story = {
  render: () => <Row subject={messageApproval()} locale="vi" />,
};

function promotionApproval(): Approval {
  return approval({
    kind: "promote_lead",
    summary: "Qualify Dana Buyer and open Example Buyer",
    target_entity_type: "lead",
    target_entity_id: "11111111-1111-4111-8111-111111111111",
    proposed_change: { trigger: "human_qualify" },
    preview: {
      version: 2,
      state: "ready",
      current_version: 3,
      reviewer_reason: "on_behalf_of",
      changes: [],
      details: [
        {
          field: "promotion_trigger",
          value: { kind: "text", text: "human_qualify" },
        },
        {
          field: "promotion_note",
          value: { kind: "text", text: "Buyer confirmed the delivery scope." },
        },
        {
          field: "promotion_contact_name",
          value: { kind: "text", text: "Dana Buyer" },
        },
        {
          field: "promotion_contact_email",
          value: { kind: "text", text: "dana@example.test" },
        },
        {
          field: "promotion_withdrawal_address",
          value: { kind: "text", text: "dana@example.test" },
        },
        {
          field: "promotion_restriction_purpose",
          value: { kind: "text", text: "Product updates" },
        },
        {
          field: "promotion_link_issued",
          value: { kind: "date", text: "2026-08-20T09:00:00Z" },
        },
        {
          field: "promotion_link_expires",
          value: { kind: "date", text: "2028-08-20T09:00:00Z" },
        },
        {
          field: "promotion_record_revoked",
          value: { kind: "boolean", text: "true" },
        },
        {
          field: "promotion_contact_owner",
          value: { kind: "empty", text: "" },
        },
        {
          field: "promotion_deal_name",
          value: { kind: "text", text: "Example Buyer" },
        },
        { field: "pipeline_id", value: { kind: "reference", text: "Sales" } },
        { field: "stage_id", value: { kind: "reference", text: "Discovery" } },
        {
          field: "promotion_deal_amount",
          value: { kind: "money", text: "250000", currency: "USD" },
        },
      ],
      effects: [
        "promote_lead_archive_source",
        "promote_create_contact",
        "promote_open_deal_seat_contact",
      ],
    },
  });
}

export const LeadQualification: Story = {
  render: () => <Row subject={promotionApproval()} />,
};
export const LeadQualificationGerman: Story = {
  render: () => <Row subject={promotionApproval()} locale="de" />,
};
export const LeadQualificationVietnamese: Story = {
  render: () => <Row subject={promotionApproval()} locale="vi" />,
};

export const Pending: Story = {
  render: () => <Row subject={approval({})} />,
};

// A countdown is drawn only while the row is live and the proposal expires, so
// this is the state where the row ticks.
export const Expiring: Story = {
  render: () => (
    <Row subject={approval({ expires_at: "2099-01-01T00:00:00Z" })} />
  ),
};

// A decided row is read-only: the history of what was decided, with no verbs.
export const Decided: Story = {
  render: () => <Row subject={approval({ status: "approved" })} decided />,
};

export const RejectHeld: Story = {
  render: () => <Row subject={approval({})} locale="de" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: de["decision.reject"] }),
    );
    await expect(
      await canvas.findByText(de["decision.status.rejected"]),
    ).toHaveFocus();
    await expect(
      canvas.getByRole("button", { name: de["common.undo"] }),
    ).toBeVisible();
  },
};

// The world moved under the proposal. The honest offer is to re-stage, not to
// try the same write again against a version that no longer exists.
export const VersionSkew: Story = {
  render: () => (
    <Row
      subject={approval({})}
      decide={() =>
        jsonResponse(
          {
            type: "about:blank",
            title: "The record changed",
            status: 409,
            code: "version_skew",
            detail: "This deal was updated after the proposal was staged.",
          },
          409,
        )
      }
    />
  ),
};

// Somebody else decided it first. The row is stale rather than wrong, so it
// drops out instead of offering a retry.
export const AlreadyDecided: Story = {
  render: () => (
    <Row
      subject={approval({})}
      decide={() =>
        jsonResponse(
          {
            type: "about:blank",
            title: "Already decided",
            status: 409,
            code: "already_decided",
            detail: "This proposal was approved by Mira Voss.",
          },
          409,
        )
      }
    />
  ),
};

export const ReassignedCommitment: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        requested_decider_id: "assignee",
        summary: "Send revised pricing",
        proposed_change: {
          summary: "Send revised pricing",
          party: "Theo",
          promiser_seat_id: "original",
          seat_id: "assignee",
          due_date: "2026-10-23",
        },
      })}
    />
  ),
};

export const InactivePromiser: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        summary: "Send revised pricing",
        proposed_change: {
          summary: "Send revised pricing",
          party: "Theo",
          promiser_seat_id: "original",
          due_date: "2026-10-23",
        },
      })}
    />
  ),
};

export const InactivePromiserDark: Story = {
  ...InactivePromiser,
  globals: { theme: "dark" },
};

export const UnresolvedPromiser: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        summary: "Confirm the delivery date",
        proposed_change: {
          summary: "Confirm the delivery date",
          party: "Mira",
          promiser_unresolved: true,
        },
      })}
    />
  ),
};

function CommitmentReading({ locale }: Readonly<{ locale: Locale }>) {
  return (
    <Row
      locale={locale}
      subject={approval({
        kind: "commitment_task",
        summary: "Send the revised scope for the fleet renewal",
        target_entity_type: "activity",
        target_entity_id: "22222222-2222-4222-8222-222222222222",
        proposed_change: {
          summary: "Send the revised scope for the fleet renewal",
          party:
            "Alexandra Beck · Regional account development and service delivery",
          promiser_unresolved: true,
        },
      })}
    />
  );
}
export const CommitmentEnglish: Story = {
  render: () => <CommitmentReading locale="en" />,
};
export const CommitmentGerman: Story = {
  render: () => <CommitmentReading locale="de" />,
};
export const CommitmentVietnamese: Story = {
  render: () => <CommitmentReading locale="vi" />,
};
export const CommitmentGermanDark: Story = {
  ...CommitmentGerman,
  globals: { theme: "dark" },
};

export const InactiveAssignee: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        summary: "Send the revised scope",
        assignee_inactive: true,
        proposed_change: {
          summary: "Send the revised scope",
          party: "Alex Beck",
          seat_id: "assignee",
        },
      })}
    />
  ),
};

export const CommercialDecision: Story = {
  render: () => <Row subject={commercialApproval()} />,
};

export const CommercialDecisionGerman: Story = {
  render: () => <Row locale="de" subject={commercialApproval()} />,
};

export const CommercialDecisionVietnamese: Story = {
  render: () => <Row locale="vi" subject={commercialApproval()} />,
};

export const CommercialDecisionDark: Story = {
  ...CommercialDecision,
  globals: { theme: "dark" },
};

export const StaleCommercialDecision: Story = {
  render: () => (
    <Row
      subject={commercialApproval({
        preview: {
          version: 1,
          state: "stale",
          current_version: 4,
          reviewer_reason: "requested_decider",
          changes: [],
        },
      })}
    />
  ),
};

export const AdditionalRecipient: Story = {
  render: () => (
    <Row
      subject={approval({
        kind: "commitment_task",
        summary: "Send the recording",
        proposed_change: {
          summary: "Send the recording",
          existing_task_id: "33333333-3333-4333-8333-333333333333",
        },
        preview: {
          version: 1,
          state: "ready",
          reviewer_reason: "record_authority",
          effects: ["attach_task_recipients"],
          changes: [
            {
              field: "existing_task_id",
              before: {
                kind: "reference",
                text: "Human wording for the recording",
                entity_type: "activity",
                entity_id: "33333333-3333-4333-8333-333333333333",
              },
              after: {
                kind: "reference",
                text: "Human wording for the recording",
                entity_type: "activity",
                entity_id: "33333333-3333-4333-8333-333333333333",
              },
            },
            {
              field: "recipients",
              before: { kind: "empty", text: "" },
              after: { kind: "text", text: "Pat Buyer" },
            },
          ],
        },
      })}
    />
  ),
};
export const AdditionalRecipientDark: Story = {
  ...AdditionalRecipient,
  globals: { theme: "dark" },
};

function lifecycleDecision(blocked: boolean): Approval {
  return approval({
    kind: "lifecycle_change",
    target_entity_type: "company",
    target_entity_id: "company-one",
    target_label: "Cedar Labs",
    summary: "Review the end of the account relationship",
    proposed_change: {
      company_id: "company-one",
      proposed_lifecycle: "former_customer",
    },
    preview: {
      version: 1,
      state: blocked ? "stale" : "ready",
      reviewer_reason: "requested_decider",
      lifecycle_evidence: {
        scope: "company",
        occurred_at: "2026-07-31T12:00:00Z",
      },
      changes: [
        {
          field: "lifecycle",
          before: { kind: "lifecycle", text: "customer" },
          after: { kind: "lifecycle", text: "former_customer" },
        },
      ],
      conflicts: blocked
        ? [
            {
              kind: "active_contract",
              summary: "An agreement remains active.",
              blocks_approval: true,
            },
          ]
        : [],
    },
  });
}
export const AccountTermination: Story = {
  render: () => <Row subject={lifecycleDecision(false)} />,
};
export const ContinuingAgreement: Story = {
  render: () => <Row subject={lifecycleDecision(true)} />,
};
export const ContinuingAgreementDark: Story = {
  ...ContinuingAgreement,
  globals: { theme: "dark" },
};

function relinkApproval(): Approval {
  return approval({
    kind: "relink_activity",
    summary: "File buyer discussion under Delivery project",
    target_entity_type: "activity",
    target_entity_id: "11111111-1111-4111-8111-111111111111",
    proposed_change: {
      entity_type: "project",
      entity_id: "22222222-2222-4222-8222-222222222222",
      replace_existing_of_type: true,
    },
    preview: {
      version: 2,
      state: "ready",
      current_version: 3,
      reviewer_reason: "record_authority",
      changes: [],
      details: [
        {
          field: "relink_destination",
          value: { kind: "text", text: "Delivery project" },
        },
        {
          field: "relink_activity",
          value: { kind: "text", text: "Buyer discussion" },
        },
        {
          field: "relink_removed_link",
          value: { kind: "text", text: "Earlier project" },
        },
      ],
      effects: ["relink_preserve_other_links", "relink_project_retention"],
    },
  });
}

export const ProjectRelink: Story = {
  render: () => <Row subject={relinkApproval()} />,
};
export const ProjectRelinkGerman: Story = {
  render: () => <Row subject={relinkApproval()} locale="de" />,
};
export const ProjectRelinkVietnamese: Story = {
  render: () => <Row subject={relinkApproval()} locale="vi" />,
};

function leadCreationApproval(): Approval {
  return approval({
    kind: "create_record",
    summary: "Create New Buyer as a lead",
    target_entity_type: "lead",
    target_entity_id: undefined,
    target_version: undefined,
    proposed_change: {
      record_type: "lead",
      fields: { full_name: "New Buyer", email: "buyer@example.test" },
    },
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      details: [
        { field: "full_name", value: { kind: "text", text: "New Buyer" } },
        { field: "email", value: { kind: "text", text: "buyer@example.test" } },
        { field: "lead_status", value: { kind: "text", text: "new" } },
        { field: "owner_id", value: { kind: "empty", text: "" } },
      ],
      effects: ["create_lead_record"],
    },
  });
}
export const LeadCreation: Story = {
  render: () => <Row subject={leadCreationApproval()} />,
};
export const LeadCreationGerman: Story = {
  render: () => <Row subject={leadCreationApproval()} locale="de" />,
};
export const LeadCreationVietnamese: Story = {
  render: () => <Row subject={leadCreationApproval()} locale="vi" />,
};

function companyCreationApproval(): Approval {
  return approval({
    kind: "create_record",
    summary: "Create Approval Buyer as a company",
    target_entity_type: "company",
    target_entity_id: undefined,
    target_version: undefined,
    proposed_change: {
      record_type: "company",
      fields: { display_name: "Approval Buyer" },
    },
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      details: [
        {
          field: "display_name",
          value: { kind: "text", text: "Approval Buyer" },
        },
        {
          field: "creation_primary_domain",
          value: { kind: "text", text: "buyer.example.test" },
        },
        { field: "address_city", value: { kind: "text", text: "Berlin" } },
        { field: "lifecycle", value: { kind: "lifecycle", text: "unknown" } },
      ],
      effects: [
        "create_company_record",
        "create_workspace_visible",
        "geocode_if_enabled",
      ],
    },
  });
}
export const CompanyCreation: Story = {
  render: () => <Row subject={companyCreationApproval()} />,
};
export const CompanyCreationGerman: Story = {
  render: () => <Row subject={companyCreationApproval()} locale="de" />,
};
export const CompanyCreationVietnamese: Story = {
  render: () => <Row subject={companyCreationApproval()} locale="vi" />,
};

function contactCreationApproval(): Approval {
  return approval({
    kind: "create_record",
    summary: "Create Confirmed Buyer as a contact",
    target_entity_type: "contact",
    target_entity_id: undefined,
    target_version: undefined,
    proposed_change: {
      record_type: "contact",
      fields: { full_name: "Confirmed Buyer" },
    },
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      details: [
        {
          field: "full_name",
          value: { kind: "text", text: "Confirmed Buyer" },
        },
        {
          field: "creation_acquisition",
          value: { kind: "text", text: "unknown_legacy" },
        },
        { field: "email", value: { kind: "text", text: "buyer@example.test" } },
        { field: "creation_email_type", value: { kind: "text", text: "work" } },
        { field: "creation_primary", value: { kind: "boolean", text: "true" } },
        {
          field: "creation_promoted_lead",
          value: { kind: "text", text: "Earlier Buyer" },
        },
        { field: "title", value: { kind: "text", text: "Director" } },
        {
          field: "creation_source_activity",
          value: { kind: "text", text: "Buyer discussion" },
        },
        {
          field: "creation_source_consent",
          value: { kind: "text", text: "withdrawn" },
        },
      ],
      effects: [
        "create_contact_record",
        "create_workspace_visible",
        "record_contact_acquisition",
        "create_promote_matching_leads",
      ],
    },
  });
}
export const ContactCreation: Story = {
  render: () => <Row subject={contactCreationApproval()} />,
};
export const ContactCreationGerman: Story = {
  render: () => <Row subject={contactCreationApproval()} locale="de" />,
};
export const ContactCreationVietnamese: Story = {
  render: () => <Row subject={contactCreationApproval()} locale="vi" />,
};

function commercialCreationApproval(recordType: "deal" | "project"): Approval {
  const project = recordType === "project";
  return approval({
    kind: "create_record",
    summary: project ? "Create Buyer rollout" : "Create Buyer expansion",
    target_entity_type: recordType,
    target_entity_id: undefined,
    target_version: undefined,
    proposed_change: { record_type: recordType, fields: { name: "Buyer" } },
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      details: project
        ? [
            { field: "name", value: { kind: "text", text: "Buyer rollout" } },
            {
              field: "company_id",
              value: { kind: "text", text: "Project Buyer" },
            },
            { field: "owner_id", value: { kind: "text", text: "Dana Seller" } },
            { field: "phase", value: { kind: "text", text: "initiative" } },
            {
              field: "started_at",
              value: { kind: "date", text: "2030-01-02" },
            },
            {
              field: "target_end_date",
              value: { kind: "date", text: "2030-06-30" },
            },
          ]
        : [
            { field: "name", value: { kind: "text", text: "Buyer expansion" } },
            {
              field: "pipeline_id",
              value: { kind: "text", text: "Enterprise sales" },
            },
            { field: "stage_id", value: { kind: "text", text: "Discovery" } },
            { field: "owner_id", value: { kind: "text", text: "Dana Seller" } },
            {
              field: "amount_minor",
              value: { kind: "money", text: "250000", currency: "EUR" },
            },
            {
              field: "expected_arr_minor",
              value: { kind: "money", text: "120000", currency: "EUR" },
            },
          ],
      effects: project
        ? ["create_project_record", "assign_project_key"]
        : ["create_deal_record"],
    },
  });
}
export const DealCreation: Story = {
  render: () => <Row subject={commercialCreationApproval("deal")} />,
};
export const DealCreationGerman: Story = {
  render: () => (
    <Row subject={commercialCreationApproval("deal")} locale="de" />
  ),
};
export const DealCreationVietnamese: Story = {
  render: () => (
    <Row subject={commercialCreationApproval("deal")} locale="vi" />
  ),
};
export const ProjectCreation: Story = {
  render: () => <Row subject={commercialCreationApproval("project")} />,
};
export const ProjectCreationGerman: Story = {
  render: () => (
    <Row subject={commercialCreationApproval("project")} locale="de" />
  ),
};
export const ProjectCreationVietnamese: Story = {
  render: () => (
    <Row subject={commercialCreationApproval("project")} locale="vi" />
  ),
};

function productCreationApproval(): Approval {
  return approval({
    kind: "create_record",
    summary: "Create Support subscription",
    target_entity_type: "product",
    target_entity_id: undefined,
    target_version: undefined,
    proposed_change: {
      record_type: "product",
      fields: {
        name: "Support subscription",
        unit_price_minor: 25000,
        currency: "EUR",
      },
    },
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      details: [
        {
          field: "name",
          value: { kind: "text", text: "Support subscription" },
        },
        { field: "sku", value: { kind: "text", text: "SUPPORT" } },
        { field: "unit", value: { kind: "text", text: "piece" } },
        {
          field: "unit_price_minor",
          value: { kind: "money", text: "25000", currency: "EUR" },
        },
        { field: "default_tax_rate", value: { kind: "number", text: "19.12" } },
        { field: "active", value: { kind: "boolean", text: "true" } },
        { field: "billing_model", value: { kind: "text", text: "recurring" } },
        {
          field: "billing_interval_months",
          value: { kind: "number", text: "3" },
        },
      ],
      effects: ["create_product_record"],
    },
  });
}
export const ProductCreation: Story = {
  render: () => <Row subject={productCreationApproval()} />,
};
export const ProductCreationGerman: Story = {
  render: () => <Row subject={productCreationApproval()} locale="de" />,
};
export const ProductCreationVietnamese: Story = {
  render: () => <Row subject={productCreationApproval()} locale="vi" />,
};

function templateCreationApproval(): Approval {
  return approval({
    kind: "create_record",
    summary: "Create Standard offer template",
    target_entity_type: "offer_template",
    target_entity_id: undefined,
    target_version: undefined,
    proposed_change: {
      record_type: "offer_template",
      fields: {
        name: "Standard offer",
        layout: { header: "Example Services" },
      },
    },
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      details: [
        { field: "name", value: { kind: "text", text: "Standard offer" } },
        { field: "locale", value: { kind: "text", text: "de-DE" } },
        { field: "is_default", value: { kind: "boolean", text: "true" } },
        {
          field: "template_header",
          value: {
            kind: "text",
            text: "Example Services\nEngineering and delivery\n12 Example Street, Berlin",
          },
        },
        {
          field: "template_footer",
          value: {
            kind: "text",
            text: "Example Services GmbH · Registered in Berlin\nPlease include the offer reference in all correspondence.\nThis letterhead identifies the issuer; payment and delivery terms are set on each offer.",
          },
        },
      ],
      effects: ["create_offer_template", "set_template_locale_default"],
    },
  });
}
export const TemplateCreation: Story = {
  render: () => <Row subject={templateCreationApproval()} />,
};
export const TemplateCreationGerman: Story = {
  render: () => <Row subject={templateCreationApproval()} locale="de" />,
};
export const TemplateCreationVietnamese: Story = {
  render: () => <Row subject={templateCreationApproval()} locale="vi" />,
};

// A LinkedIn match offers the export's position as the job title and the company
// the import placed the connection at as the employer. A box is staged ticked
// only when the contact has nothing there yet.
const LINKEDIN_COMPANY = "33333333-3333-4333-8333-333333333333";
const LINKEDIN_CONTACT = "44444444-4444-4444-8444-444444444444";

function linkedInApproval(change: Record<string, unknown>): Approval {
  return approval({
    kind: "linkedin_match",
    proposed_by: "system:linkedin",
    summary: "Andreas Müller at Acme GmbH looks like Andreas Muller",
    target_entity_type: "contact",
    target_entity_id: LINKEDIN_CONTACT,
    proposed_change: {
      connection_id: "55555555-5555-4555-8555-555555555555",
      contact_id: LINKEDIN_CONTACT,
      connection_name: "Andreas Müller",
      connection_company: "Acme GmbH",
      connection_position: "Head of Sales",
      contact_name: "Andreas Muller",
      employer_company_id: LINKEDIN_COMPANY,
      ...change,
    },
  });
}

function linkedInRoutes(employer?: string) {
  return {
    [`GET /companies/${LINKEDIN_COMPANY}`]: () =>
      jsonResponse({ id: LINKEDIN_COMPANY, display_name: "Acme GmbH" }),
    [`GET /contacts/${LINKEDIN_CONTACT}`]: () =>
      jsonResponse({
        id: LINKEDIN_CONTACT,
        full_name: "Andreas Muller",
        ...(employer
          ? {
              employer: {
                company_id: "66666666-6666-4666-8666-666666666666",
                company_name: employer,
              },
            }
          : {}),
      }),
  };
}

const linkedInGrants = {
  relationship: ["create", "update"],
  company: ["read"],
  contact: ["read", "update"],
} as const;

export const LinkedInMatchEmptyContact: Story = {
  render: () => (
    <Row
      subject={linkedInApproval({ use_as_title: true, use_as_employer: true })}
      allow={linkedInGrants}
      routes={linkedInRoutes()}
    />
  ),
};

export const LinkedInMatchContactHasValues: Story = {
  render: () => (
    <Row
      subject={linkedInApproval({
        contact_title: "Sales",
        employer_relationship_id: "77777777-7777-4777-8777-777777777777",
        use_as_title: false,
        use_as_employer: false,
      })}
      allow={linkedInGrants}
      routes={linkedInRoutes("Old Employer GmbH")}
    />
  ),
};

export const LinkedInMatchContactHasValuesGerman: Story = {
  render: () => (
    <Row
      subject={linkedInApproval({
        contact_title: "Vertrieb",
        employer_relationship_id: "77777777-7777-4777-8777-777777777777",
        use_as_title: false,
        use_as_employer: false,
      })}
      locale="de"
      allow={linkedInGrants}
      routes={linkedInRoutes("Old Employer GmbH")}
    />
  ),
};

export const LinkedInMatchWithoutEmploymentGrant: Story = {
  render: () => (
    <Row
      subject={linkedInApproval({ use_as_title: true, use_as_employer: true })}
      allow={{ company: ["read"], contact: ["read", "update"] }}
      routes={linkedInRoutes()}
    />
  ),
};

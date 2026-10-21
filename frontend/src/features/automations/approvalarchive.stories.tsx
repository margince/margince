import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  approvalRowSubject as approval,
  ApprovalRowStory as Row,
} from "../../testing/approval-row-story";

const meta: Meta = {
  title: "Patterns/Archive approval",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
type Approval = components["schemas"]["Approval"];

function recordArchiveApproval(
  kind: "deal" | "project" | "contact" | "company" = "deal",
): Approval {
  const name =
    kind === "contact"
      ? "Dana Buyer"
      : kind === "company"
        ? "Example account"
        : "Regional expansion";
  return approval({
    kind: "archive_record",
    summary: `Archive ${name}`,
    target_entity_type: kind,
    target_entity_id: "11111111-1111-4111-8111-111111111111",
    target_label: name,
    requested_decider_id: "u-1",
    proposed_change: {
      record_type: kind,
      id: "11111111-1111-4111-8111-111111111111",
    },
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "requested_decider",
      changes: [],
      details:
        kind === "company"
          ? [
              { field: "display_name", value: { kind: "text", text: name } },
              {
                field: "archive_domain",
                value: { kind: "text", text: "example.test" },
              },
              {
                field: "archive_company_type",
                value: { kind: "text", text: "partner" },
              },
              { field: "archive_partner", value: { kind: "text", text: name } },
              {
                field: "relationship_kind",
                value: { kind: "text", text: "employment" },
              },
              {
                field: "contact_id",
                value: { kind: "reference", text: "Dana Buyer" },
              },
              {
                field: "archive_list",
                value: { kind: "reference", text: "Partner accounts" },
              },
              {
                field: "archive_tag",
                value: { kind: "reference", text: "Delivery" },
              },
            ]
          : kind === "contact"
            ? [
                { field: "full_name", value: { kind: "text", text: name } },
                {
                  field: "archive_email",
                  value: { kind: "text", text: "buyer@example.test" },
                },
                {
                  field: "archive_phone",
                  value: { kind: "text", text: "+493012345678" },
                },
                {
                  field: "archive_channel",
                  value: { kind: "text", text: "telegram" },
                },
                {
                  field: "relationship_kind",
                  value: { kind: "text", text: "employment" },
                },
                {
                  field: "company_id",
                  value: { kind: "reference", text: "Example account" },
                },
              ]
            : [
                {
                  field: "name",
                  value: { kind: "text", text: "Regional expansion" },
                },
                {
                  field: "archive_stakeholder",
                  value: { kind: "reference", text: "Dana Buyer" },
                },
                {
                  field: "archive_role",
                  value: { kind: "text", text: "Decision maker" },
                },
                {
                  field: "archive_stakeholder",
                  value: { kind: "reference", text: "Alex Sponsor" },
                },
                {
                  field: "archive_role",
                  value: { kind: "text", text: "Sponsor" },
                },
                {
                  field: "archive_list",
                  value: { kind: "reference", text: "This quarter's renewals" },
                },
                {
                  field: "archive_tag",
                  value: { kind: "reference", text: "Expansion" },
                },
              ],
      effects: [`archive_${kind}_detach_scope`],
    },
  });
}

export const DealArchive: Story = {
  render: () => <Row subject={recordArchiveApproval()} />,
};
export const DealArchiveGerman: Story = {
  render: () => <Row subject={recordArchiveApproval()} locale="de" />,
};
export const DealArchiveVietnamese: Story = {
  render: () => <Row subject={recordArchiveApproval()} locale="vi" />,
};

export const ProjectArchive: Story = {
  render: () => <Row subject={recordArchiveApproval("project")} />,
};
export const ProjectArchiveGerman: Story = {
  render: () => <Row subject={recordArchiveApproval("project")} locale="de" />,
};
export const ProjectArchiveVietnamese: Story = {
  render: () => <Row subject={recordArchiveApproval("project")} locale="vi" />,
};

export const ContactArchive: Story = {
  render: () => <Row subject={recordArchiveApproval("contact")} />,
};
export const ContactArchiveGerman: Story = {
  render: () => <Row subject={recordArchiveApproval("contact")} locale="de" />,
};
export const ContactArchiveVietnamese: Story = {
  render: () => <Row subject={recordArchiveApproval("contact")} locale="vi" />,
};

function tagMergeApproval(): Approval {
  return approval({
    kind: "merge_tags",
    summary: "Merge the duplicate customer category into Customer",
    target_entity_type: "tag",
    target_entity_id: "11111111-1111-4111-8111-111111111111",
    target_label: "Customers",
    requested_decider_id: "u-1",
    proposed_change: {
      tag_id: "11111111-1111-4111-8111-111111111111",
      into_tag_id: "33333333-3333-4333-8333-333333333333",
    },
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "requested_decider",
      changes: [],
      details: [
        {
          field: "merge_tag_source",
          value: { kind: "text", text: "Customers" },
        },
        {
          field: "merge_tag_target",
          value: { kind: "text", text: "Customer" },
        },
        {
          field: "merge_tag_move",
          value: { kind: "reference", text: "Example Engineering" },
        },
        {
          field: "merge_tag_collapse",
          value: { kind: "reference", text: "Example Logistics" },
        },
      ],
      effects: ["merge_tags_release_source"],
    },
  });
}
export const TagMerge: Story = {
  render: () => <Row subject={tagMergeApproval()} />,
};
export const TagMergeGerman: Story = {
  render: () => <Row subject={tagMergeApproval()} locale="de" />,
};
export const TagMergeVietnamese: Story = {
  render: () => <Row subject={tagMergeApproval()} locale="vi" />,
};

function relationshipArchiveApproval(): Approval {
  return approval({
    kind: "archive_record",
    summary: "Remove the former employment link",
    target_entity_type: "relationship",
    target_entity_id: "11111111-1111-4111-8111-111111111111",
    target_label: "Dana Engineer · Previous employer",
    requested_decider_id: "u-1",
    proposed_change: {
      record_type: "relationship",
      id: "11111111-1111-4111-8111-111111111111",
    },
    preview: {
      version: 2,
      state: "ready",
      reviewer_reason: "requested_decider",
      changes: [],
      details: [
        {
          field: "relationship_kind",
          value: { kind: "text", text: "employment" },
        },
        {
          field: "contact_id",
          value: { kind: "reference", text: "Dana Engineer" },
        },
        {
          field: "company_id",
          value: { kind: "reference", text: "Previous employer" },
        },
        {
          field: "archive_role",
          value: { kind: "text", text: "Engineering lead" },
        },
        {
          field: "relationship_started",
          value: { kind: "date", text: "2024-01-01" },
        },
        {
          field: "relationship_start_precision",
          value: { kind: "text", text: "month" },
        },
        {
          field: "archive_primary_employer",
          value: { kind: "reference", text: "Remaining employer" },
        },
      ],
      effects: [
        "archive_relationship_preserve_records",
        "archive_relationship_promote_employer",
      ],
    },
  });
}
export const RelationshipArchive: Story = {
  render: () => <Row subject={relationshipArchiveApproval()} />,
};
export const RelationshipArchiveGerman: Story = {
  render: () => <Row subject={relationshipArchiveApproval()} locale="de" />,
};
export const RelationshipArchiveVietnamese: Story = {
  render: () => <Row subject={relationshipArchiveApproval()} locale="vi" />,
};

export const CompanyArchive: Story = {
  render: () => <Row subject={recordArchiveApproval("company")} />,
};
export const CompanyArchiveGerman: Story = {
  render: () => <Row subject={recordArchiveApproval("company")} locale="de" />,
};
export const CompanyArchiveVietnamese: Story = {
  render: () => <Row subject={recordArchiveApproval("company")} locale="vi" />,
};

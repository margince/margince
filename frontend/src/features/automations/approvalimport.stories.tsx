import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import {
  ApprovalRowStory,
  approvalRowSubject,
} from "../../testing/approval-row-story";

const meta: Meta = {
  title: "Patterns/Decision row",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

const subject = approvalRowSubject({
  kind: "commit_import",
  summary: "Import the reviewed account and deal rows",
  target_entity_type: "import_run",
  target_label: "Accounts and opportunities.csv",
  proposed_change: { run_id: "11111111-1111-4111-8111-111111111111" },
  preview: {
    version: 2,
    state: "ready",
    reviewer_reason: "on_behalf_of",
    changes: [],
    import_rows: [
      {
        object: "company",
        source_key: "account-one",
        line: 2,
        outcome: "create",
        changes: [],
        details: [
          {
            field: "display_name",
            value: { kind: "text", text: "Example Works" },
          },
          { field: "domain", value: { kind: "text", text: "example.test" } },
        ],
        effects: ["create_company_record"],
      },
      {
        object: "deal",
        source_key: "deal-one",
        line: 2,
        outcome: "update",
        target: { kind: "reference", text: "Warehouse expansion" },
        changes: [
          {
            field: "amount_minor",
            before: { kind: "money", text: "120000", currency: "EUR" },
            after: { kind: "money", text: "180000", currency: "EUR" },
          },
          {
            field: "import_link_row_company",
            before: { kind: "empty", text: "" },
            after: { kind: "text", text: "account-one" },
          },
        ],
        details: [],
        effects: [],
      },
    ],
  } satisfies components["schemas"]["ApprovalPreview"],
});

export const ReviewedImport: Story = {
  render: () => <ApprovalRowStory subject={subject} />,
};
export const ReviewedImportGerman: Story = {
  render: () => <ApprovalRowStory subject={subject} locale="de" />,
};
export const ReviewedImportVietnamese: Story = {
  render: () => <ApprovalRowStory subject={subject} locale="vi" />,
};

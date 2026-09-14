import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { type FieldRecord, RecordFields } from "./recordfields";
import { StoryProviders } from "./story-utils";

const meta: Meta = {
  title: "Records/Details fields",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
function Example({
  readOnly = false,
  failed = false,
}: {
  readOnly?: boolean;
  failed?: boolean;
}) {
  const [record, setRecord] = useState<FieldRecord>({
    id: "sample",
    version: 1,
    name: "Brandt Automotive",
    description: "",
    currency: "EUR",
    amount: "15000",
    emails: [{ email: "mira@example.test" }],
  });
  return (
    <StoryProviders>
      <RecordFields
        title="Details"
        kind="company"
        record={record}
        canEdit={!readOnly}
        fields={[
          { key: "name", labelText: "Name", required: true },
          { key: "description", labelText: "Description", type: "textarea" },
          {
            key: "currency",
            labelText: "Currency",
            type: "select",
            options: [
              { value: "EUR", label: "EUR" },
              { value: "USD", label: "USD" },
            ],
          },
          { key: "amount", labelText: "Amount", type: "number", step: "any" },
          {
            key: "emails",
            labelText: "Email addresses",
            type: "repeatable",
            rowFields: [{ key: "email", label: "create.email", type: "email" }],
            addLabel: "field.addEmail",
          },
        ]}
        groups={[{ label: "Value", keys: ["amount", "currency"] }]}
        save={async (values, rows) => {
          if (failed) throw new Error("Unavailable");
          setRecord((current) => ({
            ...current,
            ...values,
            ...rows,
            version: (current.version ?? 0) + 1,
          }));
        }}
      />
    </StoryProviders>
  );
}
export const Editable: Story = { render: () => <Example /> };
export const ReadOnly: Story = { render: () => <Example readOnly /> };
export const SaveFailure: Story = { render: () => <Example failed /> };

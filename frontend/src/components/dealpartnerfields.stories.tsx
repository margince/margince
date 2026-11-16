import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider, useT } from "../i18n";
import { RecordFormBody } from "./create.form";
import { partnerFields } from "./dealpartnerfields";

const meta: Meta = {
  title: "Patterns/Create record/Deal partner",
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj;

function PartnerForm({ noneYet = false }: { noneYet?: boolean }) {
  const t = useT();
  const [values, setValues] = useState<Record<string, string>>({
    partner_company_id: noneYet ? "" : "partner-1",
    partner_attribution: "influenced",
  });
  return (
    <RecordFormBody
      fields={partnerFields(
        t,
        noneYet ? [] : [{ value: "partner-1", label: "Referral partner" }],
        { noneYet },
      )}
      values={values}
      setValues={setValues}
      rows={{}}
      setRows={() => {}}
      pending={false}
      error={null}
      intent="create"
      onSubmit={() => {}}
      onClose={() => {}}
    />
  );
}
export const Attributed: Story = { render: () => <PartnerForm /> };
export const NoPartnersYet: Story = { render: () => <PartnerForm noneYet /> };

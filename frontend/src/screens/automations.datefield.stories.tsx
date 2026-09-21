import type { Meta, StoryObj } from "@storybook/react-vite";
import { Field } from "../design-system/atoms";
import { DateFieldSelect } from "./automations.datefield";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The picker that names a workspace's own date column, drawn on its own.
//
// It is always the control of a `Field` — the label belongs to the form that
// declares the parameter — so every frame here renders it through one, which is
// also what puts the picker's hint under the control rather than beside it.
//
// The three frames are the three answers this control can give: the columns the
// workspace actually keeps, the honest empty state for an object that keeps
// none, and the state before an object has been chosen at all — which is the
// one a reader meets first and the one a bare disabled dropdown explains worst.

const DATE_COLUMNS = {
  data: [
    {
      id: "cf-1",
      object: "deal",
      key: "cf_contract_end_date",
      label: "Contract end date",
      type: "date",
      status: "active",
    },
    {
      id: "cf-2",
      object: "deal",
      key: "cf_renewal_notice_by",
      label: "Renewal notice by",
      type: "date",
      status: "active",
    },
  ],
  page: { next_cursor: null },
};

const meta: Meta<typeof DateFieldSelect> = {
  title: "Settings/AI/Automations/Date field picker",
  component: DateFieldSelect,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof DateFieldSelect>;

function picker(object: string, columns: typeof DATE_COLUMNS) {
  installFetchStub({ "GET /custom-fields": () => jsonResponse(columns) });
  return (
    <StoryProviders>
      <Field label="Date field">
        {(control) => (
          <DateFieldSelect
            object={object}
            value=""
            onChange={() => undefined}
            control={control}
          />
        )}
      </Field>
    </StoryProviders>
  );
}

/** The workspace keeps two date columns on deals, so the dial offers both. */
export const OffersTheWorkspacesOwnColumns: Story = {
  render: () => picker("deal", DATE_COLUMNS),
};

/** No date column on this object: the dial has nothing to offer and says so
 *  under itself rather than standing empty. */
export const NoDateColumnOnTheObject: Story = {
  render: () => picker("deal", { data: [], page: { next_cursor: null } }),
};

/** Before an object is chosen there is no list to ask for — the parameter above
 *  this one decides which columns exist. */
export const BeforeAnObjectIsChosen: Story = {
  render: () => picker("", DATE_COLUMNS),
};

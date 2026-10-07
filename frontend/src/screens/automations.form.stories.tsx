import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { AutomationDialog } from "./automations.form";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The dialog both automation verbs open, drawn open. Create and edit differ only
// in what they seed it with and what their submit verb is called; the count of
// fields decides whether it is a form dialog or a drawer.

type CatalogEntry = components["schemas"]["AutomationCatalogEntry"];

const meta: Meta<typeof AutomationDialog> = {
  title: "Settings/AI/Automations/Automation form",
  component: AutomationDialog,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof AutomationDialog>;

// The commonest shape: one integer parameter with a default and a range.
const nudge: CatalogEntry = {
  key: "stalled_deal_nudge",
  name: "Stalled-deal nudge",
  description: "Stages a follow-up when a deal stalls.",
  trigger: "deal.stalled",
  action: "send_email",
  tier: "confirmation_required",
  params_schema: {
    type: "object",
    properties: {
      due_in_days: { type: "integer", minimum: 1, maximum: 30, default: 3 },
    },
    required: ["due_in_days"],
  },
};

// Every kind `ParamFieldControl` can draw, in one entry: the date-field picker
// (a string property the catalogue names `date_field`), an integer with bounds,
// a closed enum, a boolean, and a free string. The catalogue's own
// renewal_reminder is the first four; `note` is here so the fifth control is
// pictured too rather than only described.
const renewal: CatalogEntry = {
  key: "renewal_reminder",
  name: "Renewal reminder",
  description: "Raises a task before a date the workspace keeps itself.",
  trigger: "clock.daily",
  action: "create_task",
  tier: "auto_execute",
  params_schema: {
    type: "object",
    properties: {
      date_field: { type: "string" },
      days_before: { type: "integer", minimum: 1, maximum: 365, default: 30 },
      object: { type: "string", enum: ["contact", "company", "deal"] },
      recurs_yearly: { type: "boolean", default: false },
      note: { type: "string" },
    },
  },
};

// The date picker offers the workspace's own active date columns for whichever
// object the form is currently pointing at — so a story that wants it populated
// has to answer that list rather than leaving it empty and hinted.
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

function form(props: Partial<Parameters<typeof AutomationDialog>[0]> = {}) {
  installFetchStub({ "GET /custom-fields": () => jsonResponse(DATE_COLUMNS) });
  return (
    <StoryProviders>
      <AutomationDialog
        open
        entry={nudge}
        initialName={nudge.name}
        submitLabel="Create"
        pending={false}
        onSubmit={() => undefined}
        onClose={() => undefined}
        {...props}
      />
    </StoryProviders>
  );
}

/** A NEW automation: the name is the template's, and every parameter starts at
 *  the schema's own default rather than empty. */
export const NewAutomation: Story = { render: () => form() };

/** The EDIT flow: the same form seeded from an instance somebody already
 *  configured, so the values are the running ones rather than the defaults, and
 *  the verb says Save. */
export const SeededFromAnExisting: Story = {
  render: () =>
    form({
      initialName: "Nudge after three quiet days",
      initialParams: { due_in_days: 7 },
      submitLabel: "Save",
    }),
};

/** Every parameter kind the catalogue can declare, in one form: the date-field
 *  picker, a bounded integer, a closed enum, a checkbox and a free string. */
export const EveryParameterKind: Story = {
  render: () =>
    form({
      entry: renewal,
      initialName: renewal.name,
      initialParams: { object: "deal", days_before: 45, recurs_yearly: true },
      submitLabel: "Save",
    }),
};

/** The write is out. The submit says so and is held, so a second press cannot
 *  send the same definition twice. */
export const Saving: Story = {
  render: () => form({ pending: true, submitLabel: "Save" }),
};

// Past the form dialog's field limit the definition is worked through in a drawer.
const sequence: CatalogEntry = {
  ...renewal,
  key: "follow_up_sequence",
  name: "Follow-up sequence",
  params_schema: {
    type: "object",
    properties: {
      date_field: { type: "string" },
      days_before: { type: "integer", minimum: 1, maximum: 365, default: 30 },
      object: { type: "string", enum: ["contact", "company", "deal"] },
      recurs_yearly: { type: "boolean", default: false },
      note: { type: "string" },
      first_after_days: { type: "integer", minimum: 1, default: 2 },
      second_after_days: { type: "integer", minimum: 1, default: 7 },
    },
  },
};

/** Eight fields: the same form in the drawer. */
export const LongDefinitionInADrawer: Story = {
  render: () => form({ entry: sequence, initialName: sequence.name }),
};

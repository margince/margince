import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, screen, userEvent, waitFor, within } from "storybook/test";
import { Field, Textarea, TextInput } from "./atoms";
import { ChoiceList } from "./choicelist";
import { ConfirmModal } from "./confirmmodal";
import { useFieldFaults } from "./savepage";

const meta: Meta = {
  title: "Components/Overlays and layering/Confirm modal",
  component: ConfirmModal,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

function ConfirmDemo({
  tier,
  error,
  cancelLabel,
}: Readonly<{
  tier?: "confirm";
  error?: string | null;
  cancelLabel?: string;
}>) {
  const [open, setOpen] = useState(true);
  return (
    <ConfirmModal
      open={open}
      onClose={() => setOpen(false)}
      title="Send this offer?"
      tier={tier}
      confirmLabel="Send"
      cancelLabel={cancelLabel}
      onConfirm={() => setOpen(false)}
      error={error}
    >
      <p>The buyer will receive the offer by email.</p>
    </ConfirmModal>
  );
}

const opensDialog = async () => {
  await screen.findByRole("dialog");
};

export const Default: Story = {
  render: () => <ConfirmDemo />,
  play: opensDialog,
};

export const ConfirmTier: Story = {
  render: () => <ConfirmDemo tier="confirm" />,
  play: opensDialog,
};

export const WithError: Story = {
  render: () => <ConfirmDemo error="A currency conversion rate is missing." />,
  play: opensDialog,
};

// Every row one stack step apart, the refusal included.
function ConfirmFormDemo({
  error,
  intent,
}: Readonly<{ error?: string; intent?: "drawer" }>) {
  const [open, setOpen] = useState(true);
  const [reason, setReason] = useState("price");
  return (
    <ConfirmModal
      open={open}
      onClose={() => setOpen(false)}
      title="Mark this deal lost?"
      confirmLabel="Mark lost"
      confirmVariant="danger"
      onConfirm={() => setOpen(false)}
      error={error}
      intent={intent}
    >
      <p>The deal leaves the pipeline and its open tasks close.</p>
      <ChoiceList
        legend="Why was it lost?"
        value={reason}
        choices={[
          { value: "price", label: "Price" },
          { value: "timing", label: "Timing" },
          { value: "competitor", label: "Went with a competitor" },
        ]}
        onChange={setReason}
      />
      <Field label="Competitor">
        {(control) => <TextInput {...control} defaultValue="Northwind" />}
      </Field>
      <Field label="Note" hint="Read by whoever picks the account up next.">
        {(control) => <Textarea {...control} rows={3} />}
      </Field>
    </ConfirmModal>
  );
}

const opensWithFields = async () => {
  const dialog = within(await screen.findByRole("dialog"));
  await expect(await dialog.findByLabelText("Competitor")).toBeInTheDocument();
  await expect(await dialog.findByLabelText("Note")).toBeInTheDocument();
};

export const WithFields: Story = {
  render: () => <ConfirmFormDemo />,
  play: opensWithFields,
};

export const WithFieldsAndError: Story = {
  render: () => (
    <ConfirmFormDemo error="The deal changed while you were here." />
  ),
  play: opensWithFields,
};

export const WithFieldsInDrawer: Story = {
  render: () => <ConfirmFormDemo intent="drawer" />,
  play: opensWithFields,
};

/** The way back named for what it keeps, where Cancel would not say it. */
export const NamedCancel: Story = {
  render: () => <ConfirmDemo cancelLabel="Keep the draft" />,
};

// A confirm pressed with a field empty: the sentence sits on the field and
// focus lands there, found through `bodyRef`.
function RefusedFieldDemo() {
  const [faults, faultsRoot] = useFieldFaults<HTMLDivElement>();
  const [competitor, setCompetitor] = useState("");
  return (
    <ConfirmModal
      open
      onClose={() => undefined}
      title="Mark this deal lost?"
      confirmLabel="Mark lost"
      confirmVariant="danger"
      bodyRef={faultsRoot}
      onConfirm={() => {
        faults.show(
          competitor.trim() === ""
            ? new Map([["competitor", "Competitor is required."]])
            : new Map(),
        );
      }}
    >
      <Field label="Competitor" error={faults.of("competitor")}>
        {(control) => (
          <TextInput
            {...control}
            value={competitor}
            onChange={(event) => {
              setCompetitor(event.target.value);
              faults.edited("competitor");
            }}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}

export const WithRefusedField: Story = {
  render: () => <RefusedFieldDemo />,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.click(dialog.getByRole("button", { name: "Mark lost" }));
    const sentence = await dialog.findByText("Competitor is required.");
    // The dialog fades in, and a line read during that fade is not yet visible.
    await waitFor(() => expect(sentence).toBeVisible());
    await expect(dialog.getByLabelText("Competitor")).toHaveFocus();
  },
};

function EnterConfirmsDemo() {
  const [name, setName] = useState("");
  const [saved, setSaved] = useState<string | null>(null);
  return (
    <>
      {saved !== null && <p>Saved “{saved}”</p>}
      <ConfirmModal
        open={saved === null}
        onClose={() => setSaved("")}
        title="Add tag"
        intent="form"
        confirmLabel="Add"
        confirmDisabled={name.trim() === ""}
        onConfirm={() => setSaved(name.trim())}
      >
        <Field label="Name">
          {(control) => (
            <TextInput
              {...control}
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
    </>
  );
}

/** Enter in a single-line field of a form presses the confirm. */
export const EnterConfirms: Story = {
  render: () => <EnterConfirmsDemo />,
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await userEvent.type(dialog.getByLabelText("Name"), "Renewal{Enter}");
    await expect(await screen.findByText("Saved “Renewal”")).toBeVisible();
  },
};

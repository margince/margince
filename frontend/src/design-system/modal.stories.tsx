import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useId, useState } from "react";
import { expect, screen, userEvent, within } from "storybook/test";
import {
  Button,
  Field,
  Modal,
  SectionHeader,
  Textarea,
  TextInput,
} from "./atoms";
import { ConfirmModal } from "./confirmmodal";
import { DrawerBody, DrawerFoot, DrawerHead } from "./drawerbands";
import { Heading } from "./heading";
import type { DrawerSpan, ModalIntent } from "./modal";
import { Select } from "./select";

// fe-uat (frontend/scripts/fe-uat.mjs) maps modal.tsx → modal.stories.tsx and
// fails a change to modal.tsx whose stories here do not render clean.
//
// Every one of these opens ON MOUNT, because a dialog rendered closed
// screenshots as an empty canvas. The trigger stays so the reader can reopen it
// after dismissing, which is also the only way to see the surface arrive.
const meta: Meta = {
  title: "Components/Overlays and layering/Modal",
  component: Modal,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The record a dialog opens over, so the scrim has a page to darken and a
// drawer has a page to sit beside.
function Behind({ onOpen }: Readonly<{ onOpen: () => void }>) {
  return (
    <>
      <SectionHeader title="Globex GmbH" />
      <p className="t-body">
        Anna Brandt replied on Tuesday and is waiting on pricing. Nobody has
        written since.
      </p>
      <Button variant="primary" onClick={onOpen}>
        Open
      </Button>
    </>
  );
}

function IntentDemo({
  intent,
  title,
  verb,
  danger = false,
  children,
}: Readonly<{
  intent: ModalIntent;
  title: string;
  verb: string;
  danger?: boolean;
  children: ReactNode;
}>) {
  const [open, setOpen] = useState(true);
  const titleId = useId();
  return (
    <>
      <Behind onOpen={() => setOpen(true)} />
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        intent={intent}
      >
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {title}
        </Heading>
        <div className="form-stack">{children}</div>
        <div className="actions">
          <Button onClick={() => setOpen(false)}>Cancel</Button>
          <Button
            variant={danger ? "danger" : "primary"}
            onClick={() => setOpen(false)}
          >
            {verb}
          </Button>
        </div>
      </Modal>
    </>
  );
}

const opens = (name: string) => async () => {
  const dialog = within(await screen.findByRole("dialog", { name }));
  await expect(
    dialog.getByRole("button", { name: "Close" }),
  ).toBeInTheDocument();
};

/** A yes/no before something irreversible: 440px, and a card on a phone. */
export const Confirm: Story = {
  render: () => (
    <IntentDemo
      intent="confirm"
      title="Merge these companies?"
      verb="Merge"
      danger
    >
      <p className="t-caption">
        Globex GmbH keeps its record; the duplicate's activities, deals and
        contacts move onto it. This cannot be undone.
      </p>
    </IntentDemo>
  ),
  play: opens("Merge these companies?"),
};

function ContactFields({ more = false }: Readonly<{ more?: boolean }>) {
  const names = more
    ? ["First name", "Last name", "Email", "Phone", "Job title", "Company"]
    : ["First name", "Last name", "Email", "Phone"];
  return (
    <>
      {names.map((label) => (
        <Field key={label} label={label}>
          {(control) => <TextInput {...control} />}
        </Field>
      ))}
      <Field label="Note" hint="Read by whoever picks the account up next.">
        {(control) => <Textarea {...control} rows={more ? 6 : 3} />}
      </Field>
    </>
  );
}

/** A short form a reader fills and leaves: 600px, a sheet on a phone. */
export const Form: Story = {
  render: () => (
    <IntentDemo intent="form" title="New contact at Globex" verb="Save contact">
      <ContactFields />
    </IntentDemo>
  ),
  play: opens("New contact at Globex"),
};

/** Only the body scrolls; the title and the action row stay pinned. */
export const FormLongerThanTheScreen: Story = {
  render: () => (
    <IntentDemo intent="form" title="Edit Anna Brandt" verb="Save changes">
      <ContactFields more />
      <Field label="Assistant">{(control) => <TextInput {...control} />}</Field>
      <Field label="Background">
        {(control) => <Textarea {...control} rows={10} />}
      </Field>
    </IntentDemo>
  ),
  play: async () => {
    const dialog = await screen.findByRole("dialog", {
      name: "Edit Anna Brandt",
    });
    const save = within(dialog).getByRole("button", { name: "Save changes" });
    await expect(save.getBoundingClientRect().bottom).toBeLessThanOrEqual(
      dialog.getBoundingClientRect().bottom,
    );
  },
};

/** Work beside the record: 560px on the trailing edge. */
export const Drawer: Story = {
  render: () => (
    <IntentDemo intent="drawer" title="Log a call with Anna" verb="Log call">
      <Field label="Outcome">{(control) => <TextInput {...control} />}</Field>
      <Field label="What was said">
        {(control) => <Textarea {...control} rows={8} />}
      </Field>
    </IntentDemo>
  ),
  play: opens("Log a call with Anna"),
};

function ReadingDemo({ expanded }: Readonly<{ expanded?: boolean }>) {
  const [open, setOpen] = useState(true);
  const [span, setSpan] = useState<DrawerSpan>("window");
  const titleId = useId();
  return (
    <>
      <Behind onOpen={() => setOpen(true)} />
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        intent="drawer-reading"
        span={expanded ? span : undefined}
        onSpanChange={expanded ? setSpan : undefined}
      >
        <DrawerHead>
          <Heading size="large" id={titleId} className="t-h2">
            Before the room with Anna Brandt
          </Heading>
          <p className="t-caption">Tuesday 14:00 · 40 minutes · Munich</p>
        </DrawerHead>
        <DrawerBody>
          {[
            "The objective: leave with a signed pilot scope.",
            "The risk: procurement has not seen the security pack.",
            "The response: send it Monday, named to Anna's counsel.",
            "The close plan: pilot in March, rollout in June, review in September.",
          ].map((line) => (
            <p key={line} className="t-body">
              {line}
            </p>
          ))}
        </DrawerBody>
        <DrawerFoot>
          <Button onClick={() => setOpen(false)}>Discard</Button>
          <Button variant="primary" onClick={() => setOpen(false)}>
            Send the brief
          </Button>
        </DrawerFoot>
      </Modal>
    </>
  );
}

/** Something read at length beside the record: 880px, banded. */
export const DrawerReading: Story = {
  render: () => <ReadingDemo />,
  play: opens("Before the room with Anna Brandt"),
};

/** The same drawer, expandable and opened at the window less its gutter.
 * Collapse, beside the X, takes it back to 880px. */
export const DrawerReadingExpanded: Story = {
  render: () => <ReadingDemo expanded />,
  play: async () => {
    const dialog = await screen.findByRole("dialog", {
      name: "Before the room with Anna Brandt",
    });
    await expect(dialog).toHaveClass("modal-drawer-window");
    await userEvent.click(
      within(dialog).getByRole("button", { name: "Collapse" }),
    );
    await expect(dialog).not.toHaveClass("modal-drawer-window");
    await expect(
      within(dialog).getByRole("button", { name: "Expand" }),
    ).toBeInTheDocument();
  },
};

function WorkspaceDemo() {
  const [open, setOpen] = useState(true);
  const [project, setProject] = useState("");
  return (
    <>
      <Behind onOpen={() => setOpen(true)} />
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        intent="workspace"
        eyebrow="Reply"
        title="Budget questions"
        headAside={
          <Field label="Project" labelHidden>
            {(control) => (
              <Select
                {...control}
                options={[
                  { value: "", label: "No project" },
                  { value: "gf12", label: "GF-12 · Globex rollout" },
                ]}
                value={project}
                onChange={setProject}
              />
            )}
          </Field>
        }
        confirmLabel="Send"
        onConfirm={() => setOpen(false)}
      >
        <Field label="Message">
          {(control) => <Textarea {...control} rows={16} />}
        </Field>
      </ConfirmModal>
    </>
  );
}

/** A centred dialog worked in at length. The head and foot are bands, the body
 * scrolls between them, and the head holds a control about the whole dialog. */
export const Workspace: Story = {
  render: () => <WorkspaceDemo />,
  play: opens("Budget questions"),
};

function FullDemo() {
  const [open, setOpen] = useState(true);
  const titleId = useId();
  return (
    <>
      <Behind onOpen={() => setOpen(true)} />
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={titleId}
        intent="full"
      >
        <div className="file-preview">
          <div className="file-preview-head">
            <Heading
              size="large"
              id={titleId}
              className="t-h3 file-preview-name"
            >
              Globex master services agreement.pdf
            </Heading>
          </div>
          <div className="file-preview-stage">
            <p className="t-body">
              1. Scope. The supplier provides the services described in each
              order form signed by both parties.
            </p>
          </div>
        </div>
      </Modal>
    </>
  );
}

/** A stored file read at the size of the screen, framed by the scrim. */
export const Full: Story = {
  render: () => <FullDemo />,
  play: opens("Globex master services agreement.pdf"),
};

/** Captured at 390px by `uat-phone`: a confirm stays a card, every other
 * intent becomes the sheet. */
export const PhoneConfirm: Story = {
  ...Confirm,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const PhoneForm: Story = {
  ...Form,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** A phone draws every drawer as the full sheet, so it offers no Collapse. */
export const PhoneDrawerReadingExpanded: Story = {
  render: () => <ReadingDemo expanded />,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async () => {
    const dialog = await screen.findByRole("dialog", {
      name: "Before the room with Anna Brandt",
    });
    await expect(
      within(dialog).queryByRole("button", { name: "Collapse" }),
    ).toBeNull();
  },
};

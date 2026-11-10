// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { FormEvent, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { useShortcutDispatcher } from "../lib/shortcuts";
import { Button, Modal, TextInput } from "./atoms";
import { Heading } from "./heading";
import type { ModalIntent } from "./modal";
import { RecordFormDialog } from "./recordformdialog";

afterEach(cleanup);

function Dispatcher({ children }: Readonly<{ children: ReactNode }>) {
  useShortcutDispatcher();
  return <LocaleProvider initial="en">{children}</LocaleProvider>;
}

function commit(target: Element, modifier: "metaKey" | "ctrlKey" = "metaKey") {
  fireEvent.keyDown(target, { key: "Enter", [modifier]: true });
}

const submitted = () => vi.fn((event: FormEvent) => event.preventDefault());

function FormDialog({
  onSubmit,
  intent = "form",
  title = "Edit company",
  actions,
  children,
}: Readonly<{
  onSubmit: (event: FormEvent) => void;
  intent?: Exclude<ModalIntent, "drawer" | "drawer-reading">;
  title?: string;
  actions: ReactNode;
  children?: ReactNode;
}>) {
  const formId = `${title}-form`;
  return (
    <Modal open onClose={() => {}} labelledBy={title} intent={intent}>
      <Heading size="large" id={title}>
        {title}
      </Heading>
      <form id={formId} onSubmit={onSubmit}>
        <TextInput aria-label={`${title} name`} defaultValue="Acme" />
        {children}
      </form>
      <div className="actions">{actions}</div>
    </Modal>
  );
}

const save = (form: string, props: Record<string, unknown> = {}) => (
  <Button variant="primary" type="submit" form={`${form}-form`} {...props}>
    Save
  </Button>
);

describe("⌘/Ctrl+Enter in a form dialog", () => {
  it.each(["metaKey", "ctrlKey"] as const)(
    "presses the submit from a field with %s",
    (modifier) => {
      const onSubmit = submitted();
      render(
        <Dispatcher>
          <FormDialog onSubmit={onSubmit} actions={save("Edit company")} />
        </Dispatcher>,
      );
      commit(screen.getByLabelText("Edit company name"), modifier);
      expect(onSubmit).toHaveBeenCalledTimes(1);
    },
  );

  it("leaves a refused submit alone", () => {
    const onSubmit = submitted();
    render(
      <Dispatcher>
        <FormDialog
          onSubmit={onSubmit}
          actions={save("Edit company", { disabled: true })}
        />
      </Dispatcher>,
    );
    commit(screen.getByLabelText("Edit company name"));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("leaves a submit whose write is out alone", () => {
    const onSubmit = submitted();
    render(
      <Dispatcher>
        <FormDialog
          onSubmit={onSubmit}
          actions={save("Edit company", { pending: true })}
        />
      </Dispatcher>,
    );
    commit(screen.getByLabelText("Edit company name"));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("never presses a destructive submit", () => {
    const onSubmit = submitted();
    render(
      <Dispatcher>
        <FormDialog
          onSubmit={onSubmit}
          actions={
            <Button variant="danger" type="submit" form="Edit company-form">
              Delete
            </Button>
          }
        />
      </Dispatcher>,
    );
    commit(screen.getByLabelText("Edit company name"));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("presses the primary of two submits", () => {
    const pressed: string[] = [];
    render(
      <Dispatcher>
        <FormDialog
          onSubmit={(event) => {
            event.preventDefault();
            const submitter = (event.nativeEvent as SubmitEvent).submitter;
            pressed.push(submitter?.textContent ?? "");
          }}
          actions={
            <>
              <Button type="submit" form="Edit company-form">
                Save draft
              </Button>
              {save("Edit company")}
            </>
          }
        />
      </Dispatcher>,
    );
    commit(screen.getByLabelText("Edit company name"));
    expect(pressed).toEqual(["Save"]);
  });

  it("holds the form's own validation", () => {
    const onSubmit = submitted();
    render(
      <Dispatcher>
        <FormDialog onSubmit={onSubmit} actions={save("Edit company")}>
          <TextInput aria-label="Domain" required defaultValue="" />
        </FormDialog>
      </Dispatcher>,
    );
    commit(screen.getByLabelText("Domain"));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("leaves ⌘↵ to a field that answers it itself", () => {
    const onSubmit = submitted();
    const ownCommit = vi.fn();
    render(
      <Dispatcher>
        <FormDialog onSubmit={onSubmit} actions={save("Edit company")}>
          <textarea
            aria-label="Note"
            onKeyDown={(event) => {
              if (event.key === "Enter" && event.metaKey) {
                event.preventDefault();
                ownCommit();
              }
            }}
          />
        </FormDialog>
      </Dispatcher>,
    );
    commit(screen.getByLabelText("Note"));
    expect(ownCommit).toHaveBeenCalledTimes(1);
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("does nothing in a dialog that holds no form", () => {
    const onConfirm = vi.fn();
    render(
      <Dispatcher>
        <Modal open onClose={() => {}} labelledBy="ask" intent="form">
          <Heading size="large" id="ask">
            Change status
          </Heading>
          <Button variant="primary" onClick={onConfirm}>
            Change
          </Button>
        </Modal>
      </Dispatcher>,
    );
    commit(screen.getByRole("dialog"));
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("answers only in the dialog on top", () => {
    const below = submitted();
    const above = submitted();
    render(
      <Dispatcher>
        <FormDialog onSubmit={below} actions={save("Edit company")} />
        <FormDialog
          onSubmit={above}
          title="New contact"
          actions={save("New contact")}
        />
      </Dispatcher>,
    );
    commit(screen.getByLabelText("New contact name"));
    expect(above).toHaveBeenCalledTimes(1);
    expect(below).not.toHaveBeenCalled();
  });

  it("does nothing while a question covers the form", () => {
    const onSubmit = submitted();
    render(
      <Dispatcher>
        <FormDialog onSubmit={onSubmit} actions={save("Edit company")} />
        <Modal open onClose={() => {}} labelledBy="leave" intent="confirm">
          <Heading size="large" id="leave">
            Discard changes?
          </Heading>
        </Modal>
      </Dispatcher>,
    );
    commit(screen.getByRole("heading", { name: "Discard changes?" }));
    expect(onSubmit).not.toHaveBeenCalled();
  });
});

describe("⌘/Ctrl+Enter in a drawer", () => {
  function drawer(
    commitSubmits: boolean | undefined,
    onSubmit: (event: FormEvent) => void,
  ) {
    return (
      <Modal
        open
        onClose={() => {}}
        labelledBy="d"
        intent="drawer"
        commitSubmits={commitSubmits}
      >
        <Heading size="large" id="d">
          Webhook
        </Heading>
        <form id="d-form" onSubmit={onSubmit}>
          <TextInput aria-label="Address" />
        </form>
        <Button variant="primary" type="submit" form="d-form">
          Save
        </Button>
      </Modal>
    );
  }

  it("does nothing unless the drawer asks for it", () => {
    const onSubmit = submitted();
    render(<Dispatcher>{drawer(undefined, onSubmit)}</Dispatcher>);
    commit(screen.getByLabelText("Address"));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("submits a record form that runs long enough to be a drawer", () => {
    const onSubmit = submitted();
    render(
      <Dispatcher>
        <RecordFormDialog
          open
          title="New contact"
          onClose={() => {}}
          fieldCount={9}
          form={
            <form id="record-form" onSubmit={onSubmit}>
              <TextInput aria-label="Full name" />
            </form>
          }
          actions={
            <Button variant="primary" type="submit" form="record-form">
              Create
            </Button>
          }
        />
      </Dispatcher>,
    );
    expect(screen.getByRole("dialog").className).toContain("modal-drawer");
    commit(screen.getByLabelText("Full name"));
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });
});

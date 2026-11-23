/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import {
  cleanup,
  fireEvent,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { type FormEvent, type ReactNode, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ConfirmModal } from "./confirmmodal";
import { DateInput, type ISODate } from "./dateinput";
import { Field } from "./field";
import type { ModalIntent } from "./modal";

// ConfirmModal is the extracted state-driven confirm-dialog shape that used
// to live duplicated inline in the deals.tsx terminal-stage advance confirm
// and archive.tsx's ArchiveAction. These specs pin the shared behaviour both
// call sites relied on: a Cancel/Confirm button pair, an optional autonomy
// dot before the title, an inline (not thrown) mutation error, and the two
// different ways the pair refuses a press while a mutation is out.

afterEach(cleanup);

describe("ConfirmModal", () => {
  it("renders nothing while closed", () => {
    rtlRender(
      <ConfirmModal
        open={false}
        onClose={vi.fn()}
        title="Archive this contact?"
        confirmLabel="Archive"
        onConfirm={vi.fn()}
      >
        <p>Body copy</p>
      </ConfirmModal>,
    );
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("renders the title and body without a dot when tier is omitted", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Archive this contact?"
        confirmLabel="Archive"
        onConfirm={vi.fn()}
      >
        <p>This cannot be undone.</p>
      </ConfirmModal>,
    );
    expect(screen.getByText("Archive this contact?")).toBeTruthy();
    expect(screen.getByText("This cannot be undone.")).toBeTruthy();
    expect(document.querySelector(".dot")).toBeNull();
  });

  it("renders an autonomy dot before the title when tier is set", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Move to Won?"
        tier="confirm"
        confirmLabel="Confirm"
        onConfirm={vi.fn()}
      >
        <p>Moving this deal to a terminal stage.</p>
      </ConfirmModal>,
    );
    expect(document.querySelector(".dot-confirm")).toBeTruthy();
  });

  it("names the dialog by its title alone, not by the dot before it", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Move to Lost?"
        tier="confirm"
        confirmLabel="Confirm"
        onConfirm={vi.fn()}
      >
        <p>Moving this deal to a terminal stage.</p>
      </ConfirmModal>,
    );
    expect(screen.getByRole("dialog", { name: "Move to Lost?" })).toBeTruthy();
  });

  it("draws the tier it is given, an automatic one included", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Move to Won?"
        tier="auto"
        confirmLabel="Confirm"
        onConfirm={vi.fn()}
      >
        <p>Moving this deal to a terminal stage.</p>
      </ConfirmModal>,
    );
    expect(
      within(screen.getByRole("heading", { name: /Move to Won/ })).getByRole(
        "img",
        { name: "Automatic" },
      ),
    ).toBeTruthy();
    expect(document.querySelector(".dot-confirm")).toBeNull();
  });

  it("fires onConfirm when the confirm button is clicked", async () => {
    const onConfirm = vi.fn();
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Archive this contact?"
        confirmLabel="Archive"
        onConfirm={onConfirm}
      >
        <p>Body copy</p>
      </ConfirmModal>,
    );
    await userEvent.click(screen.getByText("Archive"));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("confirms on Ctrl+Enter in a note, and only while Confirm can be pressed", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    const dialog = (reason?: string) => (
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Disqualify this lead?"
        confirmLabel="Disqualify"
        confirmReason={reason}
        onConfirm={onConfirm}
      >
        <textarea aria-label="Note" />
      </ConfirmModal>
    );
    const { rerender } = rtlRender(dialog("Choose a reason first."));
    await user.click(screen.getByRole("textbox", { name: "Note" }));
    await user.keyboard("{Control>}{Enter}{/Control}");
    expect(onConfirm).not.toHaveBeenCalled();

    rerender(dialog());
    // A Ctrl+Enter that picks an input-method candidate is not a confirm.
    fireEvent.keyDown(screen.getByRole("textbox", { name: "Note" }), {
      key: "Enter",
      ctrlKey: true,
      isComposing: true,
    });
    expect(onConfirm).not.toHaveBeenCalled();
    await user.keyboard("Moved{Enter}");
    expect(onConfirm).not.toHaveBeenCalled();
    await user.keyboard("{Control>}{Enter}{/Control}");
    expect(onConfirm).toHaveBeenCalledTimes(1);
    await user.keyboard("{Meta>}{Enter}{/Meta}");
    expect(onConfirm).toHaveBeenCalledTimes(2);
    expect(screen.getByRole("textbox", { name: "Note" })).toHaveValue(
      "Moved\n",
    );
  });

  it("fires onClose when the cancel button is clicked", async () => {
    const onClose = vi.fn();
    rtlRender(
      <ConfirmModal
        open
        onClose={onClose}
        title="Archive this contact?"
        confirmLabel="Archive"
        onConfirm={vi.fn()}
      >
        <p>Body copy</p>
      </ConfirmModal>,
    );
    await userEvent.click(screen.getByText("Cancel"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("names the way back when the caller gives it a name", async () => {
    const onClose = vi.fn();
    rtlRender(
      <ConfirmModal
        open
        onClose={onClose}
        title="Turn off Continue with Google?"
        confirmLabel="Turn off Google sign-in"
        cancelLabel="Keep it on"
        onConfirm={vi.fn()}
      >
        <p>Body copy</p>
      </ConfirmModal>,
    );
    expect(screen.queryByText("Cancel")).toBeNull();
    await userEvent.click(screen.getByText("Keep it on"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("announces the error message when error is set", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Archive this contact?"
        confirmLabel="Archive"
        onConfirm={vi.fn()}
        error="archive failed"
      >
        <p>Body copy</p>
      </ConfirmModal>,
    );
    expect(screen.getByRole("alert").textContent).toBe("archive failed");
  });

  it("renders no error paragraph when error is null", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Archive this contact?"
        confirmLabel="Archive"
        onConfirm={vi.fn()}
        error={null}
      >
        <p>Body copy</p>
      </ConfirmModal>,
    );
    expect(screen.queryByRole("alert")).toBeNull();
  });

  // Both buttons refuse the press while the act is in flight, and they refuse
  // it in two different ways because they are two different facts. Confirm
  // started the write and stays focusable so the reader keeps their place;
  // Cancel started nothing and is simply not available, since backing out of
  // something already on its way to the server would tell the reader they
  // stopped it when they did not.
  it("keeps the confirm focusable and busy, and takes Cancel away", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Archive this contact?"
        confirmLabel="Archive"
        onConfirm={vi.fn()}
        pending
      >
        <p>Body copy</p>
      </ConfirmModal>,
    );
    expect(screen.getByRole("button", { name: "Cancel" })).toBeDisabled();
    const confirm = screen.getByRole("button", { name: "Archive" });
    expect(confirm).toBeEnabled();
    expect(confirm).toHaveAttribute("aria-disabled", "true");
    expect(confirm).toHaveAttribute("aria-busy", "true");
  });

  it("does not confirm a second time while the first is still out", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Archive this contact?"
        confirmLabel="Archive"
        onConfirm={onConfirm}
        pending
      >
        <p>Body copy</p>
      </ConfirmModal>,
    );
    await user.click(screen.getByText("Archive"));
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("holds Escape, the backdrop and the corner X until the write settles", async () => {
    const user = userEvent.setup();
    function Archiving({ pending }: Readonly<{ pending: boolean }>) {
      const [open, setOpen] = useState(true);
      return (
        <ConfirmModal
          open={open}
          onClose={() => setOpen(false)}
          title="Archive this contact?"
          confirmLabel="Archive"
          onConfirm={vi.fn()}
          pending={pending}
        >
          <p>Body copy</p>
        </ConfirmModal>
      );
    }
    const { rerender } = rtlRender(<Archiving pending />);
    const backdrop = document.querySelector(".overlay");
    if (!backdrop) throw new Error("the dialog drew no backdrop");
    await user.keyboard("{Escape}");
    await user.click(backdrop);
    const corner = screen.getByRole("button", { name: "Close" });
    expect(corner).toBeDisabled();
    await user.click(corner);
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    rerender(<Archiving pending={false} />);
    expect(screen.getByRole("button", { name: "Close" })).toBeEnabled();
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("leaves both buttons enabled when not pending", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Archive this contact?"
        confirmLabel="Archive"
        onConfirm={vi.fn()}
      >
        <p>Body copy</p>
      </ConfirmModal>,
    );
    expect((screen.getByText("Cancel") as HTMLButtonElement).disabled).toBe(
      false,
    );
    expect((screen.getByText("Archive") as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("lets the caller gate the confirm while its own precondition is unmet", async () => {
    const onConfirm = vi.fn();
    rtlRender(
      <ConfirmModal
        open
        onClose={() => undefined}
        title="Fulfill erasure request"
        confirmLabel="Erase and suppress"
        confirmVariant="danger"
        confirmDisabled
        onConfirm={onConfirm}
      >
        <p>Type ERASE to confirm.</p>
      </ConfirmModal>,
    );

    const confirm = screen.getByRole("button", { name: "Erase and suppress" });
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
    // An unmet precondition is not a write in flight. The two used to share
    // one `disabled` on this control, so "type ERASE first" was drawn exactly
    // like "your erasure is going through".
    expect(confirm.hasAttribute("aria-busy")).toBe(false);
    await userEvent.click(confirm);
    expect(onConfirm).not.toHaveBeenCalled();
  });

  it("leaves the confirm enabled when the caller sets no gate", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={() => undefined}
        title="Revoke"
        confirmLabel="Revoke"
        onConfirm={() => undefined}
      >
        <p>body</p>
      </ConfirmModal>,
    );
    expect(
      (screen.getByRole("button", { name: "Revoke" }) as HTMLButtonElement)
        .disabled,
    ).toBe(false);
  });

  it("stacks the body and its error in one form stack", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Mark lost?"
        confirmLabel="Mark lost"
        onConfirm={vi.fn()}
        error="The deal changed."
      >
        <p>Body copy</p>
        <p>More copy</p>
      </ConfirmModal>,
    );
    const stack = screen.getByRole("dialog").querySelector(".form-stack");
    expect(
      [...(stack?.children ?? [])].map((child) => child.textContent),
    ).toEqual(["Body copy", "More copy", "The deal changed."]);
  });

  it("draws no empty stack for a body that holds nothing", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Sign out everywhere?"
        confirmLabel="Sign out"
        onConfirm={vi.fn()}
      >
        {false}
      </ConfirmModal>,
    );
    expect(screen.getByRole("dialog").querySelector(".form-stack")).toBeNull();
  });
});

describe("the box a confirm sits in", () => {
  function boxOf(box: { intent?: Exclude<ModalIntent, "full"> }) {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Reject this offer?"
        confirmLabel="Reject"
        onConfirm={vi.fn()}
        {...box}
      >
        <p>The buyer is told.</p>
      </ConfirmModal>,
    );
    return screen.getByRole("dialog", { name: "Reject this offer?" }).className;
  }

  it("is the confirm card unless the caller names another", () => {
    expect(boxOf({})).toBe("modal modal-confirm");
  });

  it("takes the intent the caller names", () => {
    expect(boxOf({ intent: "form" })).toBe("modal modal-form");
  });

  it("refuses the lightbox, which clips a body past the viewport", () => {
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Full"
        confirmLabel="Go"
        onConfirm={vi.fn()}
        // @ts-expect-error a confirm never takes the full intent
        intent="full"
      >
        <p>Body</p>
      </ConfirmModal>,
    );
    expect(screen.getByRole("dialog", { name: "Full" })).toBeInTheDocument();
  });

  it("hands its body, the element around the fields, to bodyRef", () => {
    let body: HTMLDivElement | null = null;
    rtlRender(
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Add to Shortlist"
        confirmLabel="Add"
        onConfirm={vi.fn()}
        bodyRef={(element) => {
          body = element;
        }}
      >
        <input aria-label="Note" />
      </ConfirmModal>,
    );
    const note = screen.getByRole("textbox", { name: "Note" });
    expect(body).not.toBeNull();
    expect(body).toBe(note.parentElement);
    expect(note.closest(".form-stack")).toBe(body);
  });
});

describe("Enter in a form dialog", () => {
  type Shape = Partial<
    Pick<
      Parameters<typeof ConfirmModal>[0],
      | "intent"
      | "enterConfirms"
      | "confirmDisabled"
      | "confirmReason"
      | "pending"
    >
  >;

  function NamedTag({
    onSave,
    shape = { intent: "form" },
    field,
  }: Readonly<{
    onSave: (name: string) => void;
    shape?: Shape;
    field?: (name: string, setName: (next: string) => void) => ReactNode;
  }>) {
    const [name, setName] = useState("");
    return (
      <ConfirmModal
        open
        onClose={vi.fn()}
        title="Add tag"
        confirmLabel="Add"
        onConfirm={() => onSave(name)}
        {...shape}
      >
        {field ? (
          field(name, setName)
        ) : (
          <input
            aria-label="Name"
            value={name}
            onChange={(event) => setName(event.target.value)}
          />
        )}
      </ConfirmModal>
    );
  }

  const nameBox = () => screen.getByRole("textbox", { name: "Name" });

  it("presses Confirm with what was typed", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    rtlRender(<NamedTag onSave={onSave} />);
    await user.type(nameBox(), "Renewal{Enter}");
    expect(onSave).toHaveBeenCalledExactlyOnceWith("Renewal");
  });

  it.each<[string, Shape]>([
    ["disabled", { intent: "form", confirmDisabled: true }],
    ["refused with a reason", { intent: "form", confirmReason: "Name it." }],
    ["busy with the write", { intent: "form", pending: true }],
    ["switched off", { intent: "form", enterConfirms: false }],
    ["a plain confirm", {}],
  ])("does nothing while Confirm is %s", async (_state, shape) => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    rtlRender(<NamedTag onSave={onSave} shape={shape} />);
    await user.type(nameBox(), "Renewal{Enter}");
    expect(onSave).not.toHaveBeenCalled();
    expect(nameBox()).toHaveValue("Renewal");
  });

  it("leaves a modified Enter and an input method's Enter to the field", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    rtlRender(<NamedTag onSave={onSave} />);
    await user.type(nameBox(), "Renewal{Shift>}{Enter}{/Shift}");
    await user.keyboard("{Alt>}{Enter}{/Alt}");
    fireEvent.keyDown(nameBox(), { key: "Enter", isComposing: true });
    fireEvent.keyDown(nameBox(), { key: "Enter", keyCode: 229 });
    expect(onSave).not.toHaveBeenCalled();
  });

  it.each<
    [string, (name: string, setName: (next: string) => void) => ReactNode]
  >([
    [
      "a combobox",
      (name, setName) => (
        <input
          aria-label="Name"
          role="combobox"
          aria-expanded={false}
          aria-controls="tag-rows"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      ),
    ],
    [
      "a field that suggests",
      (name, setName) => (
        <input
          aria-label="Name"
          aria-autocomplete="list"
          value={name}
          onChange={(event) => setName(event.target.value)}
        />
      ),
    ],
    [
      "a field that takes its own Enter",
      (name, setName) => (
        <input
          aria-label="Name"
          value={name}
          onChange={(event) => setName(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter") event.preventDefault();
          }}
        />
      ),
    ],
  ])("leaves Enter to %s", async (_owner, field) => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    rtlRender(<NamedTag onSave={onSave} field={field} />);
    const box = screen.getByLabelText("Name");
    await user.type(box, "Renewal{Enter}");
    expect(onSave).not.toHaveBeenCalled();
    expect(box).toHaveValue("Renewal");
  });

  it("leaves a checkbox's Enter to the checkbox", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    rtlRender(
      <NamedTag
        onSave={onSave}
        field={() => <input type="checkbox" aria-label="Pinned" />}
      />,
    );
    screen.getByRole("checkbox", { name: "Pinned" }).focus();
    await user.keyboard("{Enter}");
    expect(onSave).not.toHaveBeenCalled();
  });

  it("lets a form inside the body submit once, without a second save", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    const onSubmit = vi.fn((event: FormEvent) => event.preventDefault());
    rtlRender(
      <NamedTag
        onSave={onSave}
        field={(name, setName) => (
          <form onSubmit={onSubmit}>
            <input
              aria-label="Name"
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </form>
        )}
      />,
    );
    await user.type(nameBox(), "Renewal{Enter}");
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSave).not.toHaveBeenCalled();
  });

  it("saves the day a date field settles on that same Enter", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    rtlRender(
      <NamedTag
        onSave={onSave}
        field={(day, setDay) => (
          <Field label="Due">
            {(control) => (
              <DateInput
                {...control}
                today="2026-05-20"
                value={day as ISODate | ""}
                onChange={(event) => setDay(event.target.value)}
              />
            )}
          </Field>
        )}
      />,
    );
    await user.type(
      screen.getByRole("textbox", { name: "Due" }),
      "21/05/2026{Enter}",
    );
    expect(onSave).toHaveBeenCalledExactlyOnceWith("2026-05-21");
  });

  it("refuses as the click does while a date field holds text it cannot read", async () => {
    const user = userEvent.setup();
    const onSave = vi.fn();
    rtlRender(
      <NamedTag
        onSave={onSave}
        field={(day, setDay) => (
          <Field label="Due">
            {(control) => (
              <DateInput
                {...control}
                today="2026-05-20"
                value={day as ISODate | ""}
                onChange={(event) => setDay(event.target.value)}
              />
            )}
          </Field>
        )}
      />,
    );
    const due = screen.getByRole("textbox", { name: "Due" });
    await user.type(due, "31/02/2026{Enter}");
    expect(onSave).not.toHaveBeenCalled();
    expect(due).toHaveFocus();
  });
});

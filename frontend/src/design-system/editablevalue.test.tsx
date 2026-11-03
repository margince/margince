/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { EditableValue } from "./editablevalue";

afterEach(cleanup);

describe("EditableValue", () => {
  it("opens the editor from a press on the value", async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    render(
      <EditableValue editLabel="Change Industry" onEdit={onEdit}>
        Software
      </EditableValue>,
    );
    await user.click(screen.getByText("Software"));
    expect(onEdit).toHaveBeenCalledTimes(1);
  });

  it("opens the editor from a press on the bubble", async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    const { container } = render(
      <EditableValue editLabel="Change Industry" onEdit={onEdit}>
        Software
      </EditableValue>,
    );
    const bubble = container.querySelector(".editable-value-bubble");
    if (!bubble) throw new Error("no bubble drawn");
    await user.click(bubble);
    expect(onEdit).toHaveBeenCalledTimes(1);
  });

  it("is one tab stop, named by the verb and the value, opened by Enter or Space", async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    render(
      <EditableValue editLabel="Change Industry" onEdit={onEdit}>
        Software
      </EditableValue>,
    );
    await user.tab();
    const pill = screen.getByRole("button", {
      name: "Change Industry Software",
    });
    expect(pill).toHaveFocus();
    expect(pill).toHaveAttribute("aria-haspopup", "dialog");
    await user.keyboard("{Enter}");
    await user.keyboard(" ");
    expect(onEdit).toHaveBeenCalledTimes(2);
    await user.tab();
    expect(document.body).toHaveFocus();
  });

  it("renders a read-only value plain", () => {
    const { container } = render(
      <EditableValue
        editLabel="Change Industry"
        onEdit={() => undefined}
        editable={false}
      >
        Software
      </EditableValue>,
    );
    expect(screen.getByText("Software")).toBeInTheDocument();
    expect(screen.queryByRole("button")).toBeNull();
    expect(container.querySelector(".editable-value-bubble")).toBeNull();
    expect(container.querySelector(".editable-value")).toBeNull();
  });

  it("shows the placeholder in the pill while the value is empty", () => {
    render(
      <EditableValue
        editLabel="Change Industry"
        placeholder="Add industry"
        onEdit={() => undefined}
      />,
    );
    expect(
      screen.getByRole("button", { name: "Change Industry Add industry" }),
    ).toBeInTheDocument();
  });

  it("keeps a link value's own action and edits from the bubble", async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    const followed = vi.fn((event: React.MouseEvent) => event.preventDefault());
    render(
      <EditableValue editLabel="Change Email" onEdit={onEdit} link>
        <a href="mailto:ana@example.com" onClick={followed}>
          ana@example.com
        </a>
      </EditableValue>,
    );
    await user.click(screen.getByRole("link", { name: "ana@example.com" }));
    expect(followed).toHaveBeenCalledTimes(1);
    expect(onEdit).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Change Email" }));
    expect(onEdit).toHaveBeenCalledTimes(1);
  });

  it("edits from a press on a link value's padding, with focus on the edit button", async () => {
    const user = userEvent.setup();
    const onEdit = vi.fn();
    const { container } = render(
      <EditableValue editLabel="Change Email" onEdit={onEdit} link>
        <a href="mailto:ana@example.com">ana@example.com</a>
      </EditableValue>,
    );
    const face = container.querySelector(".editable-value-face");
    if (!face) throw new Error("no face drawn");
    await user.click(face);
    expect(onEdit).toHaveBeenCalledTimes(1);
    expect(screen.getByRole("button", { name: "Change Email" })).toHaveFocus();
  });

  it("nests no control inside another in a link value", () => {
    const { container } = render(
      <EditableValue editLabel="Change Email" onEdit={() => undefined} link>
        <a href="mailto:ana@example.com">ana@example.com</a>
      </EditableValue>,
    );
    expect(
      container.querySelector("button a, a button, button button"),
    ).toBeNull();
  });

  it("keeps the focused button when a first value turns the pill into a link", () => {
    const pill = (link: boolean) => (
      <EditableValue
        editLabel="Change Email"
        placeholder="Add email"
        onEdit={() => undefined}
        link={link}
      >
        {link ? <a href="mailto:ana@example.com">ana@example.com</a> : null}
      </EditableValue>
    );
    const view = render(pill(false));
    const opener = screen.getByRole("button", {
      name: "Change Email Add email",
    });
    opener.focus();
    view.rerender(pill(true));
    expect(screen.getByRole("button", { name: "Change Email" })).toBe(opener);
    expect(opener).toHaveFocus();
  });
});

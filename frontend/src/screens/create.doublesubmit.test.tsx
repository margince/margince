/** @vitest-environment happy-dom */
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type CreateField, CreateRecordModal } from "./create";

afterEach(cleanup);

const fields: CreateField[] = [
  { key: "name", labelText: "Name", type: "text" },
];

function modal(
  props: { pending: boolean; error: string | null },
  onSubmit: () => void,
) {
  return (
    <LocaleProvider initial="en">
      <CreateRecordModal
        open
        onClose={() => {}}
        title="New thing"
        fields={fields}
        onSubmit={onSubmit}
        {...props}
      />
    </LocaleProvider>
  );
}

function typeName() {
  fireEvent.change(screen.getByRole("textbox", { name: /Name/ }), {
    target: { value: "Acme" },
  });
}

function save() {
  return screen.getByRole("button", { name: "Create" });
}

describe("a second press while the first save is still starting", () => {
  it("sends one request even though the parent's pending flag has not flipped yet", async () => {
    const onSubmit = vi.fn();
    render(modal({ pending: false, error: null }, onSubmit));
    typeName();
    fireEvent.click(save());
    // The button's own latch has let go by now; only `pending` is still late.
    await act(async () => {});
    fireEvent.click(save());
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("lets the user save again once the first save has ended", () => {
    const onSubmit = vi.fn();
    const view = render(modal({ pending: false, error: null }, onSubmit));
    typeName();
    fireEvent.click(save());
    view.rerender(modal({ pending: true, error: null }, onSubmit));
    view.rerender(modal({ pending: false, error: "Refused" }, onSubmit));
    fireEvent.click(save());
    expect(onSubmit).toHaveBeenCalledTimes(2);
  });

  it("lets the user save again when the parent refused without ever going pending", () => {
    const onSubmit = vi.fn();
    const view = render(modal({ pending: false, error: null }, onSubmit));
    typeName();
    fireEvent.click(save());
    view.rerender(modal({ pending: false, error: "Refused" }, onSubmit));
    fireEvent.click(save());
    expect(onSubmit).toHaveBeenCalledTimes(2);
  });
});

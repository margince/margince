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

function modal(pending: boolean, onSubmit: () => void) {
  return (
    <LocaleProvider initial="en">
      <CreateRecordModal
        open
        onClose={() => {}}
        title="New thing"
        fields={fields}
        pending={pending}
        error={null}
        onSubmit={onSubmit}
      />
    </LocaleProvider>
  );
}

function typeName() {
  fireEvent.change(screen.getByRole("textbox", { name: /Name/ }), {
    target: { value: "Acme" },
  });
}

const save = () => screen.getByRole("button", { name: "Create" });
const form = () => document.querySelector("form") as HTMLFormElement;

describe("a second press while the first save is not yet drawn pending", () => {
  it("sends one request for two clicks a commit apart", async () => {
    const onSubmit = vi.fn();
    render(modal(false, onSubmit));
    typeName();
    fireEvent.click(save());
    await act(async () => {
      await Promise.resolve();
    });
    fireEvent.click(save());
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("sends one request for Enter pressed twice", () => {
    const onSubmit = vi.fn();
    render(modal(false, onSubmit));
    typeName();
    fireEvent.submit(form());
    fireEvent.submit(form());
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it("lets the user save again once the first save has ended", () => {
    const onSubmit = vi.fn();
    const view = render(modal(false, onSubmit));
    typeName();
    fireEvent.click(save());
    view.rerender(modal(true, onSubmit));
    view.rerender(modal(false, onSubmit));
    fireEvent.click(save());
    expect(onSubmit).toHaveBeenCalledTimes(2);
  });

  it("lets the user save again when the parent never went pending", async () => {
    vi.useFakeTimers();
    try {
      const onSubmit = vi.fn();
      render(modal(false, onSubmit));
      typeName();
      fireEvent.click(save());
      await act(async () => {
        await vi.advanceTimersByTimeAsync(1);
      });
      fireEvent.click(save());
      expect(onSubmit).toHaveBeenCalledTimes(2);
    } finally {
      vi.useRealTimers();
    }
  });
});

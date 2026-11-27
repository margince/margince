/** @vitest-environment happy-dom */
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { type CreateField, CreateRecordModal } from "./create";

afterEach(cleanup);

function modal(suggest: (text: string) => Promise<string>, onSubmit = vi.fn()) {
  const fields: CreateField[] = [
    { key: "name", labelText: "Name", type: "text" },
    {
      key: "name_vi",
      labelText: "Name in Vietnamese",
      fill: {
        label: "translation.suggest",
        source: (values) => values.name ?? "",
        suggest,
      },
    },
  ];
  render(
    <LocaleProvider initial="en">
      <CreateRecordModal
        open
        onClose={() => {}}
        title="New stage"
        fields={fields}
        pending={false}
        error={null}
        onSubmit={onSubmit}
      />
    </LocaleProvider>,
  );
  return onSubmit;
}

const suggest = () =>
  screen.getByRole("button", { name: /Suggest translation/ });
const vietnamese = () =>
  screen.getByRole("textbox", {
    name: /Name in Vietnamese/,
  }) as HTMLInputElement;

describe("Suggest translation under a name field", () => {
  it("waits for the name it translates", () => {
    modal(async () => "x");
    expect(suggest()).toHaveProperty("disabled", true);
  });

  it("fills the field and leaves saving to the admin", async () => {
    const onSubmit = modal(async () => "Đã gửi báo giá");
    fireEvent.change(screen.getByRole("textbox", { name: /^Name\s*$/ }), {
      target: { value: "Proposal sent" },
    });
    fireEvent.click(suggest());
    await waitFor(() => expect(vietnamese().value).toBe("Đã gửi báo giá"));
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it("keeps what was typed while the model ran, and drops an answer for a changed name", async () => {
    let answer: (text: string) => void = () => {};
    modal(() => new Promise((resolve) => (answer = resolve)));
    const name = screen.getByRole("textbox", { name: /^Name\s*$/ });
    fireEvent.change(name, { target: { value: "Proposal sent" } });
    fireEvent.click(suggest());
    fireEvent.change(name, { target: { value: "Offer sent" } });
    answer("Đã gửi báo giá");
    await waitFor(() => expect(suggest()).toHaveProperty("disabled", false));
    expect(vietnamese().value).toBe("");
    expect((name as HTMLInputElement).value).toBe("Offer sent");
  });

  it("says so when no suggestion comes back, and keeps the field as typed", async () => {
    modal(async () => {
      throw new Error("assistant_unavailable");
    });
    fireEvent.change(screen.getByRole("textbox", { name: /^Name\s*$/ }), {
      target: { value: "Proposal sent" },
    });
    fireEvent.click(suggest());
    await screen.findByText(/No suggestion came back/);
    expect(vietnamese().value).toBe("");
  });
});

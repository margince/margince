/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import {
  PageAsideProvider,
  PageAsideToggle,
  usePageAside,
} from "../app/pageaside";
import { UnsavedGuard, useUnsavedGuard } from "../app/unsaved";
import { LocaleProvider } from "../i18n";
import { type RecordFieldSave, RecordFields } from "./recordfields";

afterEach(cleanup);
function wrapper({ children }: { children: ReactNode }) {
  return (
    <QueryClientProvider
      client={
        new QueryClient({
          defaultOptions: {
            queries: { retry: false },
            mutations: { retry: false },
          },
        })
      }
    >
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}
const fields = [
  { key: "name", labelText: "Name" },
  { key: "title", labelText: "Title" },
];
const record = {
  id: "record-1",
  version: 1,
  name: "Original",
  title: "Director",
};
function editor(save: RecordFieldSave, current = record) {
  return (
    <RecordFields
      title="Details"
      kind="contact"
      fields={fields}
      record={current}
      canEdit
      save={save}
    />
  );
}

describe("record fields", () => {
  it("saves only the changed field against the version the user started editing", async () => {
    const save = vi.fn<RecordFieldSave>(async () => undefined);
    const user = userEvent.setup();
    const view = render(editor(save), { wrapper });
    await user.click(screen.getByRole("button", { name: "Change Name" }));
    await user.clear(screen.getByRole("textbox", { name: "Name" }));
    await user.type(
      screen.getByRole("textbox", { name: "Name" }),
      "My correction",
    );
    view.rerender(editor(save, { ...record, version: 2, title: "CEO" }));
    await user.keyboard("{Enter}");
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith({ name: "My correction" }, {}, record),
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
  it("retains a failed draft and permits cancelling without another write", async () => {
    const save = vi.fn<RecordFieldSave>(async () => {
      throw new Error("offline");
    });
    const user = userEvent.setup();
    render(editor(save), { wrapper });
    await user.click(screen.getByRole("button", { name: "Change Name" }));
    const input = screen.getByRole("textbox", { name: "Name" });
    await user.clear(input);
    await user.type(input, "Keep my work{Enter}");
    await screen.findByRole("alert");
    expect(input).toHaveValue("Keep my work");
    await user.keyboard("{Escape}");
    expect(
      screen.queryByRole("textbox", { name: "Name" }),
    ).not.toBeInTheDocument();
    expect(save).toHaveBeenCalledTimes(1);
  });
  it("renders unset fields, and does not write an unchanged value", async () => {
    const save = vi.fn<RecordFieldSave>(async () => undefined);
    const user = userEvent.setup();
    render(editor(save, { ...record, title: "" }), { wrapper });
    expect(
      screen.getByRole("button", { name: "Change Title" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Change Name" }));
    await user.keyboard("{Enter}");
    expect(save).not.toHaveBeenCalled();
  });
  it("keeps a coupled edit local until Save and preserves all submitted rows", async () => {
    const save = vi.fn<RecordFieldSave>(async () => undefined);
    const user = userEvent.setup();
    render(
      <RecordFields
        title="Details"
        kind="contact"
        record={{
          id: "c-1",
          version: 3,
          emails: [
            { email: "first@example.test" },
            { email: "second@example.test" },
          ],
        }}
        fields={[
          {
            key: "emails",
            labelText: "Email addresses",
            type: "repeatable",
            rowFields: [{ key: "email", label: "create.email", type: "email" }],
            addLabel: "field.addEmail",
          },
        ]}
        canEdit
        save={save}
      />,
      { wrapper },
    );
    await user.click(
      screen.getByRole("button", { name: "Change Email addresses" }),
    );
    const inputs = screen.getAllByRole("textbox", { name: /Email/ });
    await user.clear(inputs[0]);
    await user.type(inputs[0], "corrected@example.test");
    expect(save).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith(
        {},
        {
          emails: [
            { email: "corrected@example.test" },
            { email: "second@example.test" },
          ],
        },
        expect.objectContaining({ version: 3 }),
      ),
    );
  });
  it("does not offer edits to a reader without permission", () => {
    render(
      <RecordFields
        title="Details"
        kind="contact"
        fields={fields}
        record={record}
        canEdit={false}
        save={async () => undefined}
      />,
      { wrapper },
    );
    expect(screen.getByText("Original")).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Change/ }),
    ).not.toBeInTheDocument();
  });
});

function ProtectedRecord({ elsewhere = false }: { elsewhere?: boolean }) {
  const { open } = usePageAside();
  useUnsavedGuard(elsewhere);
  return (
    <>
      <PageAsideToggle />
      {open && editor(async () => undefined)}
    </>
  );
}
it("protects changed Details drafts without blocking an unchanged editor or another draft", async () => {
  const user = userEvent.setup();
  render(
    <UnsavedGuard address="record" onKeep={() => undefined}>
      {() => (
        <PageAsideProvider open>
          <ProtectedRecord elsewhere />
        </PageAsideProvider>
      )}
    </UnsavedGuard>,
    { wrapper },
  );
  const toggle = await screen.findByRole("button", { name: "Hide details" });
  await user.click(screen.getByRole("button", { name: "Change Name" }));
  expect(toggle).toBeEnabled();
  await user.type(screen.getByRole("textbox", { name: "Name" }), " changed");
  expect(screen.getByRole("button", { name: "Hide details" })).toBeDisabled();
  await user.keyboard("{Escape}");
  expect(screen.getByRole("button", { name: "Hide details" })).toBeEnabled();
});
it("focuses a refused email draft after blur so Escape can cancel it", async () => {
  const user = userEvent.setup();
  const save = vi.fn<RecordFieldSave>(async () => undefined);
  render(
    <RecordFields
      title="Details"
      kind="lead"
      fields={[{ key: "email", labelText: "Email", type: "email" }]}
      record={{ id: "l1", email: "valid@example.test" }}
      canEdit
      save={save}
    />,
    { wrapper },
  );
  await user.click(screen.getByRole("button", { name: "Change Email" }));
  const input = screen.getByRole("textbox", { name: "Email" });
  await user.clear(input);
  await user.type(input, "invalid");
  await user.tab();
  expect(input).toHaveFocus();
  await user.keyboard("{Escape}");
  expect(screen.queryByRole("textbox")).toBeNull();
  expect(save).not.toHaveBeenCalled();
});
it("connects the reference label to its search input", async () => {
  const user = userEvent.setup();
  render(
    <RecordFields
      title="Details"
      kind="company"
      fields={[
        {
          key: "parent_id",
          labelText: "Parent",
          searchTargets: async () => [],
        },
      ]}
      record={{ id: "c1" }}
      canEdit
      save={async () => undefined}
    />,
    { wrapper },
  );
  await user.click(screen.getByRole("button", { name: "Change Parent" }));
  const label = document.querySelector("label");
  if (!label) throw new Error("Reference field has no label");
  await user.click(label);
  expect(screen.getByRole("searchbox", { name: "Parent" })).toHaveFocus();
});

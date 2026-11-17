/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";
import { Button } from "../design-system/atoms";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import type { CreateField } from "./create";
import type { RailEdit } from "./railfield";
import type { RecordFieldSave } from "./recordfieldtypes";
import { type SectionEditOptions, useSectionEdit } from "./sectionedit";

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

const FIELDS: CreateField[] = [
  { key: "name", labelText: "Name", required: true },
  { key: "title", labelText: "Title" },
  {
    key: "stage",
    labelText: "Stage",
    type: "select",
    required: true,
    options: [
      { value: "qualify", label: "Qualify" },
      { value: "proposal", label: "Proposal" },
    ],
  },
];
const RECORD = {
  id: "deal-1",
  version: 4,
  name: "Fleet renewal",
  title: "Director",
  stage: "proposal",
};

function railEdit(save: RecordFieldSave, canEdit = true): RailEdit {
  return { kind: "deal", record: RECORD, fields: FIELDS, canEdit, save };
}

function Section({
  edit,
  section,
}: Readonly<{ edit: RailEdit; section: SectionEditOptions }>) {
  const { edit: sectionEdit, element } = useSectionEdit(edit, section);
  return (
    <>
      {sectionEdit && (
        <Button variant="link" onClick={sectionEdit.onEdit}>
          Edit About
        </Button>
      )}
      {element}
    </>
  );
}

const noSave: RecordFieldSave = async () => undefined;

describe("section edit", () => {
  it("opens with only the section's fields and saves them against the opened version", async () => {
    const save = vi.fn<RecordFieldSave>(async () => undefined);
    const user = userEvent.setup();
    render(
      <Section
        edit={railEdit(save)}
        section={{ label: "About", keys: ["title", "stage"] }}
      />,
      { wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Edit About" }));
    const dialog = await screen.findByRole("dialog", { name: "About" });
    expect(within(dialog).queryByRole("textbox", { name: "Name" })).toBeNull();
    await user.clear(within(dialog).getByRole("textbox", { name: "Title" }));
    await user.type(
      within(dialog).getByRole("textbox", { name: "Title" }),
      "CEO",
    );
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith(
        { title: "CEO", stage: "proposal" },
        {},
        RECORD,
      ),
    );
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("shows a locked field read-only with its reason, and never sends it", async () => {
    const save = vi.fn<RecordFieldSave>(async () => undefined);
    const user = userEvent.setup();
    render(
      <Section
        edit={railEdit(save)}
        section={{
          label: "About",
          keys: ["title", "stage"],
          readOnly: { stage: { reason: "Moved on the board" } },
        }}
      />,
      { wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Edit About" }));
    const dialog = await screen.findByRole("dialog", { name: "About" });
    expect(within(dialog).queryByRole("combobox")).toBeNull();
    const stage = within(dialog).getByRole("group", { name: "Stage" });
    expect(stage).toHaveTextContent("Proposal");
    expect(stage).toHaveAccessibleDescription("Moved on the board");
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(save).toHaveBeenCalledWith({ title: "Director" }, {}, RECORD),
    );
  });

  it("leaves the dialog for a row's own flow", async () => {
    const openOwner = vi.fn();
    const user = userEvent.setup();
    render(
      <Section
        edit={railEdit(noSave)}
        section={{
          label: "About",
          keys: ["title", "owner"],
          flows: {
            owner: { label: "Owner", reading: "Ana Brandt", open: openOwner },
          },
        }}
      />,
      { wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Edit About" }));
    const dialog = await screen.findByRole("dialog", { name: "About" });
    expect(
      within(dialog).getByRole("group", { name: "Owner" }),
    ).toHaveTextContent("Ana Brandt");
    await user.click(
      within(dialog).getByRole("button", { name: "Change Owner" }),
    );
    expect(openOwner).toHaveBeenCalledOnce();
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("asks before a flow drops a draft, and keeps it on Cancel", async () => {
    const openOwner = vi.fn();
    const user = userEvent.setup();
    render(
      <Section
        edit={railEdit(noSave)}
        section={{
          label: "About",
          keys: ["title", "owner"],
          flows: { owner: { label: "Owner", open: openOwner } },
        }}
      />,
      { wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Edit About" }));
    const dialog = await screen.findByRole("dialog", { name: "About" });
    await user.type(
      within(dialog).getByRole("textbox", { name: "Title" }),
      " of sales",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Change Owner" }),
    );
    const question = await screen.findByRole("dialog", {
      name: en["unsaved.title"],
    });
    await user.click(
      within(question).getByRole("button", { name: en["create.cancel"] }),
    );
    expect(openOwner).not.toHaveBeenCalled();
    expect(within(dialog).getByRole("textbox", { name: "Title" })).toHaveValue(
      "Director of sales",
    );
    await user.click(
      within(dialog).getByRole("button", { name: "Change Owner" }),
    );
    await user.click(
      within(
        await screen.findByRole("dialog", { name: en["unsaved.title"] }),
      ).getByRole("button", { name: en["unsaved.discard"] }),
    );
    expect(openOwner).toHaveBeenCalledOnce();
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "About" })).toBeNull(),
    );
  });

  it("opens a section's only change, a flow, directly", async () => {
    const openOwner = vi.fn();
    const user = userEvent.setup();
    render(
      <Section
        edit={railEdit(noSave, false)}
        section={{
          label: "Ownership",
          keys: ["owner", "title"],
          flows: { owner: { label: "Owner", open: openOwner } },
        }}
      />,
      { wrapper },
    );
    await user.click(screen.getByRole("button", { name: "Edit About" }));
    expect(openOwner).toHaveBeenCalledOnce();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it.each<[string, boolean, SectionEditOptions]>([
    ["a reader who may not edit", false, { label: "About", keys: ["title"] }],
    [
      "a section whose fields are all locked",
      true,
      {
        label: "About",
        keys: ["stage"],
        readOnly: { stage: { reason: "Locked" } },
      },
    ],
    [
      "a section of fields the edit does not offer",
      true,
      {
        label: "About",
        keys: ["visible_to"],
      },
    ],
  ])("offers no Edit to %s", (_case, canEdit, section) => {
    render(<Section edit={railEdit(noSave, canEdit)} section={section} />, {
      wrapper,
    });
    expect(screen.queryByRole("button", { name: "Edit About" })).toBeNull();
  });
});

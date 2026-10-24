// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import { PageSaveBar, SavePageProvider, useSaveDraft } from "./savepage";

afterEach(cleanup);

const SAVE = translate("en", "saveBar.save");
const DISCARD = translate("en", "saveBar.discard");
const UNSAVED = translate("en", "saveBar.unsaved");

function Card({
  name,
  write = async () => undefined,
}: Readonly<{ name: string; write?: (value: string) => Promise<unknown> }>) {
  const [saved, setSaved] = useState("");
  const [draft, setDraft] = useState("");
  useSaveDraft({
    dirty: draft !== saved,
    invalid: draft === "!",
    save: async () => {
      await write(draft);
      setSaved(draft);
    },
    discard: () => setDraft(saved),
  });
  return (
    <input
      aria-label={name}
      value={draft}
      onChange={(event) => setDraft(event.target.value)}
    />
  );
}

function Page({
  writes = {},
}: Readonly<{
  writes?: Record<string, (value: string) => Promise<unknown>>;
}>) {
  return (
    <LocaleProvider initial="en">
      <SavePageProvider>
        <Card name="Name" write={writes.Name} />
        <Card name="Title" write={writes.Title} />
        <PageSaveBar />
      </SavePageProvider>
    </LocaleProvider>
  );
}

const field = (name: string) => screen.getByRole("textbox", { name });
const button = (name: string) => screen.queryByRole("button", { name });

describe("PageSaveBar", () => {
  it("is absent until a card holds a draft, and goes when it no longer does", async () => {
    const user = userEvent.setup();
    render(<Page />);
    expect(screen.queryByText(UNSAVED)).toBeNull();

    await user.type(field("Name"), "Jane");
    expect(screen.getByText(UNSAVED)).toBeVisible();
    expect(button(SAVE)).toBeVisible();

    await user.clear(field("Name"));
    expect(button(SAVE)).toBeNull();
  });

  it("writes every dirty card with one Save, and none that is clean", async () => {
    const user = userEvent.setup();
    const name = vi.fn(async () => undefined);
    const title = vi.fn(async () => undefined);
    render(<Page writes={{ Name: name, Title: title }} />);
    await user.type(field("Name"), "Jane");
    await user.type(field("Title"), "CEO");
    await user.click(screen.getByRole("button", { name: SAVE }));

    expect(name).toHaveBeenCalledWith("Jane");
    expect(title).toHaveBeenCalledWith("CEO");
    expect(button(SAVE)).toBeNull();
  });

  it("keeps the bar and every refused draft, and says why once", async () => {
    const user = userEvent.setup();
    render(
      <Page
        writes={{
          Name: async () => {
            throw new Error("The name is taken.");
          },
        }}
      />,
    );
    await user.type(field("Name"), "Jane");
    await user.type(field("Title"), "CEO");
    await user.click(screen.getByRole("button", { name: SAVE }));

    expect(await screen.findAllByRole("alert")).toHaveLength(1);
    expect(field("Name")).toHaveValue("Jane");
    expect(button(SAVE)).toBeVisible();
  });

  it("holds Discard while the save runs", async () => {
    const user = userEvent.setup();
    render(<Page writes={{ Name: () => new Promise(() => undefined) }} />);
    await user.type(field("Name"), "Jane");
    await user.click(screen.getByRole("button", { name: SAVE }));

    expect(screen.getByRole("button", { name: DISCARD })).toBeDisabled();
  });

  it("puts every dirty card back on Discard", async () => {
    const user = userEvent.setup();
    render(<Page />);
    await user.type(field("Name"), "Jane");
    await user.type(field("Title"), "CEO");
    await user.click(screen.getByRole("button", { name: DISCARD }));

    expect(field("Name")).toHaveValue("");
    expect(field("Title")).toHaveValue("");
    expect(button(SAVE)).toBeNull();
  });

  it("sends nothing while a field says its value is wrong", async () => {
    const user = userEvent.setup();
    const name = vi.fn(async () => undefined);
    render(<Page writes={{ Name: name }} />);
    await user.type(field("Name"), "!");
    await user.click(screen.getByRole("button", { name: SAVE }));

    expect(name).not.toHaveBeenCalled();
    expect(screen.getByText(translate("en", "saveBar.invalid"))).toBeVisible();
  });
});

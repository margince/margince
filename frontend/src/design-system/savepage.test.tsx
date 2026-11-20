// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import {
  focusFirstRefused,
  PageSaveBar,
  SavePageProvider,
  useFieldFaults,
  useSaveDraft,
} from "./savepage";

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

type Offer = Readonly<{ offered: boolean; offer?: string }>;

function OfferedCard({ offered, offer }: Offer) {
  useSaveDraft({
    dirty: false,
    offered,
    offer,
    save: async () => undefined,
    discard: () => undefined,
  });
  return null;
}

function OfferPage({ offered, offer }: Offer) {
  return (
    <LocaleProvider initial="en">
      <SavePageProvider>
        <OfferedCard offered={offered} offer={offer} />
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

  it("brings back an offer put aside on Discard once it is offered again", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<OfferPage offered />);
    await user.click(screen.getByRole("button", { name: DISCARD }));
    expect(button(SAVE)).toBeNull();

    rerender(<OfferPage offered={false} />);
    rerender(<OfferPage offered />);
    expect(button(SAVE)).toBeVisible();
  });

  it("offers a new value at once after Discard set the old one aside", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<OfferPage offered offer="gcal" />);
    await user.click(screen.getByRole("button", { name: DISCARD }));
    expect(button(SAVE)).toBeNull();

    rerender(<OfferPage offered offer="gcal" />);
    expect(button(SAVE)).toBeNull();
    rerender(<OfferPage offered offer="graphcal" />);
    expect(button(SAVE)).toBeVisible();
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

describe("focusFirstRefused", () => {
  it("focuses the first refused field inside its root and none outside it", () => {
    render(
      <>
        <input aria-label="Outside" aria-invalid="true" />
        <form aria-label="Form">
          <input aria-label="Valid" />
          <input aria-label="First refused" aria-invalid="true" />
          <input aria-label="Second refused" aria-invalid="true" />
        </form>
      </>,
    );
    focusFirstRefused(screen.getByRole("form"));
    expect(screen.getByLabelText("First refused")).toHaveFocus();
  });

  it("does nothing without a root or a refused field", () => {
    render(<input aria-label="Valid" />);
    focusFirstRefused(null);
    focusFirstRefused(document);
    expect(document.body).toHaveFocus();
  });
});

function TwoFields() {
  const [faults, faultsRoot] = useFieldFaults();
  return (
    <form
      ref={faultsRoot}
      onSubmit={(event) => {
        event.preventDefault();
        faults.show(
          new Map([
            ["city", "Enter a city."],
            ["zip", "Enter a postal code."],
          ]),
        );
      }}
    >
      {(["city", "zip"] as const).map((field) => (
        <p key={field}>
          <input
            aria-label={field}
            aria-invalid={faults.of(field) ? true : undefined}
            onChange={() => faults.edited(field)}
          />
          <span>{faults.of(field)}</span>
        </p>
      ))}
      <button type="submit">Save</button>
    </form>
  );
}

describe("useFieldFaults", () => {
  it("shows each sentence on its field, focuses the first, and drops one when its field is edited", async () => {
    const user = userEvent.setup();
    render(<TwoFields />);
    await user.click(screen.getByRole("button", { name: "Save" }));
    expect(await screen.findByText("Enter a city.")).toBeVisible();
    expect(screen.getByText("Enter a postal code.")).toBeVisible();
    expect(screen.getByLabelText("city")).toHaveFocus();
    await user.type(screen.getByLabelText("city"), "Graz");
    expect(screen.queryByText("Enter a city.")).toBeNull();
    expect(screen.getByText("Enter a postal code.")).toBeVisible();
  });
});

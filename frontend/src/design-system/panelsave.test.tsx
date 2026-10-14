// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../i18n";
import { Panel, PanelBody } from "./panel";
import { PanelSave } from "./panelsave";

afterEach(cleanup);

const SAVE = translate("en", "panelSave.save");
const DISCARD = translate("en", "panelSave.discard");

function renderCard(props: Partial<Parameters<typeof PanelSave>[0]> = {}) {
  const onSave = vi.fn();
  const onDiscard = vi.fn();
  const view = render(
    <LocaleProvider initial="en">
      <Panel
        title="Names"
        actions={
          <PanelSave dirty onSave={onSave} onDiscard={onDiscard} {...props} />
        }
      >
        <PanelBody>Fields</PanelBody>
      </Panel>
    </LocaleProvider>,
  );
  return { ...view, onSave, onDiscard };
}

describe("PanelSave", () => {
  it("draws no verbs while the card matches what was saved", () => {
    const { container } = renderCard({ dirty: false });
    expect(screen.queryByRole("button")).toBeNull();
    expect(container.querySelector(".panel-save")).toBeNull();
  });

  it("offers Discard on the start edge and Save as the one primary on the end", () => {
    const { container, onSave, onDiscard } = renderCard();
    const save = screen.getByRole("button", { name: SAVE });
    expect(container.querySelector(".action-row-trail")).toContainElement(save);
    fireEvent.click(save);
    fireEvent.click(screen.getByRole("button", { name: DISCARD }));
    expect(onSave).toHaveBeenCalledTimes(1);
    expect(onDiscard).toHaveBeenCalledTimes(1);
  });

  it("holds both verbs while the save runs", () => {
    renderCard({ pending: true });
    expect(screen.getByRole("button", { name: SAVE })).toHaveAttribute(
      "aria-disabled",
      "true",
    );
    expect(screen.getByRole("button", { name: DISCARD })).toBeDisabled();
  });

  it("takes the caller's verb and caption", () => {
    renderCard({ saveLabel: "Save preferences", caption: "Unsaved changes" });
    expect(
      screen.getByRole("button", { name: "Save preferences" }),
    ).toBeVisible();
    expect(screen.getByText("Unsaved changes")).toBeVisible();
  });
});

/** @vitest-environment happy-dom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { en as englishCopy } from "../../../i18n/en";
import type { ConversationState } from "./conversation-types";
import { collectingState, render, stubApi } from "./voice-act.testkit";
import { ConversationWorkbench, WorkbenchEntranceScope } from "./workbench";

beforeEach(() => {
  vi.stubGlobal("scrollTo", vi.fn());
  stubApi();
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const connectState: ConversationState = {
  ...collectingState(),
  act: "connect",
  phase: "cn.consent",
};

// Two acts in a row, each mounting its own workbench as the journey does. The
// control that moves on goes away with its act.
function TwoActs({ placesFocus = false }: Readonly<{ placesFocus?: boolean }>) {
  const [act, setAct] = useState<"voice" | "connect">("voice");
  return (
    <WorkbenchEntranceScope>
      {act === "voice" ? (
        <ConversationWorkbench
          key="voice"
          core="idle"
          status=""
          railState={collectingState()}
          title="Train your writing voice"
        >
          <button type="button" onClick={() => setAct("connect")}>
            Skip for now
          </button>
        </ConversationWorkbench>
      ) : (
        <ConversationWorkbench
          key="connect"
          core="idle"
          status=""
          railState={connectState}
          title="Connect your mail"
        >
          {placesFocus ? <PlacedField /> : <p>Accounts</p>}
        </ConversationWorkbench>
      )}
    </WorkbenchEntranceScope>
  );
}

function PlacedField() {
  return (
    <input
      aria-label="Mail address"
      ref={(node) => {
        node?.focus();
      }}
    />
  );
}

describe("focus across the journey's steps", () => {
  it("leaves focus alone when the first step arrives", async () => {
    render(<TwoActs />);
    await screen.findByRole("heading", { name: "Train your writing voice" });
    expect(document.activeElement).toBe(document.body);
  });

  it("hands the reader to the new step's title when the last one goes away", async () => {
    render(<TwoActs />);
    await userEvent.click(
      await screen.findByRole("button", {
        name: englishCopy["ob.conv.voice.skipped"],
      }),
    );
    expect(document.activeElement).toBe(
      screen.getByRole("heading", { level: 1, name: "Connect your mail" }),
    );
  });

  it("keeps focus where a step put it itself", async () => {
    render(<TwoActs placesFocus />);
    await userEvent.click(
      await screen.findByRole("button", {
        name: englishCopy["ob.conv.voice.skipped"],
      }),
    );
    expect(document.activeElement).toBe(screen.getByLabelText("Mail address"));
  });

  it("names the step with a line above the title, not a heading before it", () => {
    render(
      <ConversationWorkbench
        core="idle"
        status=""
        railState={collectingState()}
        eyebrow="Step 1 of 2 · Voice"
        title="Train your writing voice"
      >
        <p>Board</p>
      </ConversationWorkbench>,
    );
    expect(screen.getAllByRole("heading").map((h) => h.textContent)).toEqual([
      "Train your writing voice",
    ]);
    expect(screen.getByText("Step 1 of 2 · Voice")).toBeTruthy();
  });
});

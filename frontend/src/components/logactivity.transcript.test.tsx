/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../design-system/select-testing";
import { en } from "../i18n/en";
import type { ActivityDraft } from "../lib/activitybody";
import { LogActivity } from "./logactivity";
import {
  type Captured,
  createdActivity,
  jsonResponse,
  render,
  stubApi,
} from "./logactivity.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

async function mount(kind: ActivityDraft["kind"]) {
  const captured: Captured[] = [];
  stubApi(
    {
      "GET /activities/deal-filing": () =>
        jsonResponse({ choice_required: false }),
      "POST /activities": createdActivity,
    },
    captured,
  );
  render(
    <LogActivity
      entityType="contact"
      entityId="p1"
      askedKind={kind === "call" || kind === "task" ? kind : undefined}
    />,
  );
  if (kind === "meeting") {
    await pickOption(
      userEvent.setup(),
      await screen.findByLabelText(en["log.kind"]),
      en["log.kindMeeting"],
    );
  }
  return captured;
}

const kinds: { kind: ActivityDraft["kind"]; transcript: boolean }[] = [
  { kind: "call", transcript: true },
  { kind: "meeting", transcript: true },
  { kind: "note", transcript: false },
  { kind: "task", transcript: false },
];

describe("transcripts in the activity form", () => {
  it("keeps the transcript choice with the entered text when changing between call and meeting", async () => {
    await mount("call");
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", { name: en["log.asTranscript"] }),
    );
    const raw = "Anna: I will send the offer.";
    fireEvent.change(
      screen.getByRole("textbox", { name: en["log.transcriptLabel"] }),
      { target: { value: raw } },
    );
    await pickOption(
      user,
      screen.getByLabelText(en["log.kind"]),
      en["log.kindMeeting"],
    );
    expect(
      screen.getByRole("checkbox", { name: en["log.asTranscript"] }),
    ).toHaveProperty("checked", true);
    expect(
      screen.getByRole("textbox", { name: en["log.transcriptLabel"] }),
    ).toHaveProperty("value", raw);
  });

  it.each(kinds)(
    "offers a transcript for $kind: $transcript",
    async ({ kind, transcript }) => {
      await mount(kind);
      await screen.findByRole("textbox", { name: /Subject/ });
      const checkbox = screen.queryByRole("checkbox", {
        name: en["log.asTranscript"],
      });
      if (transcript) {
        expect(checkbox).not.toBeNull();
        expect(checkbox).toHaveProperty("checked", false);
      } else {
        expect(checkbox).toBeNull();
      }
    },
  );

  it("sends an opted-in call as raw transcript text", async () => {
    const captured = await mount("call");
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("checkbox", { name: en["log.asTranscript"] }),
    );
    const raw = "\nAnna: I will send the offer by Friday.\n";
    fireEvent.change(
      screen.getByRole("textbox", { name: en["log.transcriptLabel"] }),
      { target: { value: raw } },
    );
    const save = screen.getByRole("button", { name: en["log.save"] });
    await waitFor(() => expect(save).toHaveProperty("disabled", false));
    await user.click(save);
    await waitFor(() =>
      expect(captured.some((entry) => entry.key === "POST /activities")).toBe(
        true,
      ),
    );
    expect(
      captured.find((entry) => entry.key === "POST /activities")?.body,
    ).toMatchObject({ kind: "call", body: raw, source_system: "transcript" });
  });

  it("keeps an ordinary call outside transcript processing", async () => {
    const captured = await mount("call");
    const user = userEvent.setup();
    await user.type(
      await screen.findByRole("textbox", { name: /Subject/ }),
      "Call summary",
    );
    const save = screen.getByRole("button", { name: en["log.save"] });
    await waitFor(() => expect(save).toHaveProperty("disabled", false));
    await user.click(save);
    await waitFor(() =>
      expect(captured.some((entry) => entry.key === "POST /activities")).toBe(
        true,
      ),
    );
    expect(
      captured.find((entry) => entry.key === "POST /activities")?.body,
    ).not.toHaveProperty("source_system");
  });
});

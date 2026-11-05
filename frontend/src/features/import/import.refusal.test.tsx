// @vitest-environment happy-dom
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { en } from "../../i18n/en";
import { ProblemError } from "../../lib/problem";
import { StoryProviders } from "../../testing/story-utils";
import { ImportRefusal } from "./import.notices";

afterEach(cleanup);

const busy = new ProblemError({
  status: 409,
  code: "import_source_busy",
  details: { run_id: "run-1" },
});

function refusal(keepsDraft: boolean) {
  render(
    <StoryProviders>
      <ImportRefusal title="Refused" error={busy} keepsDraft={keepsDraft} />
    </StoryProviders>,
  );
}

describe("a busy source name refusal", () => {
  // A stage leaves the upload as a draft, and the reader is told where it waits.
  it("says where the draft waits when the refused call left one", () => {
    refusal(true);
    expect(screen.getByText(en["import.sourceBusy"])).toBeVisible();
    expect(screen.getByText(en["import.sourceBusyDraft"])).toBeVisible();
  });

  // Undo, cancel and approve leave no draft, so the sentence would be false.
  it("says nothing of a draft when the refused call left none", () => {
    refusal(false);
    expect(screen.getByText(en["import.sourceBusy"])).toBeVisible();
    expect(screen.queryByText(en["import.sourceBusyDraft"])).toBeNull();
  });
});

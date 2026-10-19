// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { screen, waitFor } from "@testing-library/react";
import { expect } from "vitest";

// The close dialog is up and has read the deal's open tasks, so Confirm takes
// a press.
export async function askedToClose(stage: string) {
  await waitFor(() =>
    expect(screen.getByText(`Move to ${stage}?`)).toBeTruthy(),
  );
  await waitFor(() =>
    expect(screen.queryByText("Loading the deal’s open tasks…")).toBeNull(),
  );
}

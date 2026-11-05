// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { screen } from "@testing-library/react";
import type { UserEvent } from "@testing-library/user-event";

// Test-only, apart from ai-admin.testkit.ts, whose fixtures the stories import
// too: a story bundle has no use for the testing library.

/** Opens a task row's details from its name, as a reader does. */
export async function openTaskDetails(user: UserEvent, name: string) {
  const label = `${name}: what it does`;
  await user.click(screen.getByRole("button", { name: label }));
  return screen.getByRole("region", { name: label });
}

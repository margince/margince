// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { screen } from "@testing-library/react";
import { vi } from "vitest";
import { type PhoneRow, phoneRow } from "./phonerow";

/** A window `width` pixels wide, as every `max-width` query the app asks reads it. */
export function stubWindowWidth(width: number): void {
  vi.stubGlobal("matchMedia", (query: string) => ({
    matches: width <= Number(/max-width:\s*(\d+)px/.exec(query)?.[1] ?? 0),
    media: query,
    addEventListener: () => {},
    removeEventListener: () => {},
  }));
}

/** The rendered list row that names `name`, once it shows. */
export async function phoneRowOf(name: string): Promise<PhoneRow> {
  return phoneRow(await screen.findByText(name));
}

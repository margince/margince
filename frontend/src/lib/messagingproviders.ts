// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

export type MessengerProvider = components["schemas"]["MessengerProvider"];

export const MESSENGER_PROVIDERS: ReadonlyArray<{
  value: MessengerProvider;
  label: string;
}> = [
  { value: "dispact", label: "Dispact" },
  { value: "slack", label: "Slack" },
  { value: "teams", label: "Microsoft Teams" },
];

export function isMessengerProvider(value: string): value is MessengerProvider {
  return MESSENGER_PROVIDERS.some((provider) => provider.value === value);
}

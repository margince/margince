// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** Two email addresses are one mailbox when they match trimmed and case-blind, as the server stores them. */
export function addressKey(address: string): string {
  return address.trim().toLowerCase();
}

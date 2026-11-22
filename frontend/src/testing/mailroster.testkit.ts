// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The connector roster of a rep with one mailbox connected. A composer that
// finds none blocks Send, so a suite that stubs its own routes answers with this.
export const CONNECTED_ROSTER = {
  data: [{ id: "conn-1", provider: "gmail", status: "connected", scopes: [] }],
};

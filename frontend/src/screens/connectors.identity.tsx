// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { LucideIcon } from "lucide-react";

// A connection's identity, as the left half of its row: the provider this
// build's own name for it, and the account it reads. One shape for a mailbox
// and for a bot, because a reader auditing the page reads both the same way.
export function ConnectionIdentity({
  icon: Icon,
  name,
  account,
}: Readonly<{
  icon: LucideIcon;
  name: string;
  account?: string | null;
}>) {
  return (
    <span className="connector-id">
      <Icon aria-hidden />
      <span>
        <strong>{name}</strong>
        {account && <span className="connector-account">{account}</span>}
      </span>
    </span>
  );
}

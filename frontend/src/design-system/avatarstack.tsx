// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { formatNumber } from "../format/format";
import { useLocale } from "../i18n";
import { Avatar } from "./atoms";
import "./avatarstack.css";

// AvatarStack draws a group of contacts as overlapping monograms — coverage as
// faces rather than a count, up to `max`, with the remainder folded into a
// "+N" chip so a committee of fifteen still reads as one compact shape. The
// remainder is digits and a `+`, not prose — the one count this design system
// draws without a translated sentence around it, the same reasoning that
// keeps a currency figure or a version number out of the copy gate.
export function AvatarStack({
  contacts,
  max = 5,
}: Readonly<{
  contacts: readonly { name: string }[];
  max?: number;
}>) {
  const { locale } = useLocale();
  const shown = contacts.slice(0, max);
  const rest = contacts.length - shown.length;
  return (
    <span className="avatar-stack">
      {shown.map((contact) => (
        <span className="avatar-stack-item" key={contact.name}>
          <Avatar name={contact.name} />
        </span>
      ))}
      {rest > 0 && (
        <span className="avatar-stack-more">+{formatNumber(rest, locale)}</span>
      )}
    </span>
  );
}

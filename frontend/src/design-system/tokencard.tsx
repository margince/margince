// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Avatar } from "./avatar";
import type { PeekContent } from "./peek";
import type { TokenCard } from "./tokeninput";

export function tokenCard(
  value: string,
  label: string,
  card: TokenCard | undefined,
): PeekContent {
  return {
    head: label,
    headHidden: true,
    items: [
      {
        key: value,
        lead: card && (
          <Avatar
            name={label}
            identity={card.identity}
            photo={card.photo}
            size="md"
          />
        ),
        title: label,
        sub: value,
        href: card?.href,
      },
    ],
    foot: card?.about || undefined,
  };
}

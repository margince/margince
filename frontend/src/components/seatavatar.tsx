// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ComponentProps } from "react";
import { Avatar } from "../design-system/atoms";
import { useMemberAvatar } from "../lib/membernames";

// A caller holding the seat passes its `avatar_url` as `photo` and asks
// nothing; one holding only the id lets the batched names read find it.
export function SeatAvatar({
  id,
  name,
  photo,
  size,
}: Readonly<{
  id: string | null | undefined;
  name: string;
  photo?: string | null;
  size?: ComponentProps<typeof Avatar>["size"];
}>) {
  const found = useMemberAvatar(photo === undefined ? id : null);
  return (
    <Avatar
      name={name}
      identity={id || name}
      photo={photo === undefined ? found : photo}
      size={size}
    />
  );
}

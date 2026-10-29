// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ComponentProps } from "react";
import { appHref, followAppLink } from "../lib/recordaddress";

// Keep native link actions while routing an ordinary app click without a reload.
export function AppLink({ href, onClick, ...props }: ComponentProps<"a">) {
  return (
    <a
      {...props}
      href={appHref(href)}
      onClick={(event) => {
        onClick?.(event);
        followAppLink(event);
      }}
    />
  );
}

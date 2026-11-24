// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ComponentProps } from "react";
import { appHref, followAppLink } from "../lib/recordaddress";
import { useRecordHref } from "../lib/recordhref";

// Keep native link actions while routing an ordinary app click without a reload.
// A link to a record takes the way back of the list it is drawn in.
export function AppLink({ href, onClick, ...props }: ComponentProps<"a">) {
  const recordHref = useRecordHref();
  return (
    <a
      {...props}
      href={appHref(href === undefined ? href : recordHref(href))}
      onClick={(event) => {
        onClick?.(event);
        followAppLink(event);
      }}
    />
  );
}

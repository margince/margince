// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createContext, type ReactNode, useContext } from "react";

type RecordHref = (href: string) => string;

const unchanged: RecordHref = (href) => href;

const RecordHrefContext = createContext<RecordHref>(unchanged);

/**
 * How every link under it addresses a record. A list that a record leads back
 * to marks its links here, so a shared component drawn inside marks its own.
 */
export function RecordHrefScope({
  href,
  children,
}: Readonly<{ href: RecordHref; children: ReactNode }>) {
  return (
    <RecordHrefContext.Provider value={href}>
      {children}
    </RecordHrefContext.Provider>
  );
}

/** The address for a link to a record from here; unchanged outside a scope. */
export function useRecordHref(): RecordHref {
  return useContext(RecordHrefContext);
}

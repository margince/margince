// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useUnsavedGuard } from "../../components/unsaved";
import {
  PageSaveBar,
  SavePageProvider,
  useSavePage,
} from "../../design-system/savepage";

/**
 * A settings page's drafts, its one save bar, and the question on the way out.
 * The guard lives in the shell, so the page hands it the bar's save here.
 */
export function SettingsSavePage({
  children,
}: Readonly<{ children: ReactNode }>) {
  return (
    <SavePageProvider>
      <LeaveWithSave />
      {children}
      <PageSaveBar />
    </SavePageProvider>
  );
}

function LeaveWithSave() {
  const page = useSavePage();
  useUnsavedGuard(page?.dirty ?? false, undefined, page?.save);
  return null;
}

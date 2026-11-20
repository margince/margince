// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { navigate, type Route } from "../lib/router";

// A record made from a tab of the record the reader is on: the reader stays,
// and this names what landed with a way to open it.
export function useAddedNotice(): (name: string, route: Route) => void {
  const t = useT();
  const toast = useToast();
  return (name, route) =>
    toast.show(t("create.added", { name }), {
      action: {
        kind: "open",
        label: t("create.addedOpen"),
        onAct: () => navigate(route),
      },
    });
}

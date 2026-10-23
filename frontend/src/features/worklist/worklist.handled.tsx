// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the product did on this reader's behalf. It is the same Done for you
// list Home draws, with the same window, count and status. A change is never
// missing here because some other list was meant to show it. Each line carries its own way back where it has one; a record's
// full history stays on the record.
//
// Last on the page, and open. A reader opens this page to find what to do
// next, and a list of what is finished answers a different question.

import { Panel } from "../../design-system/panel";
import { useT } from "../../i18n";
import { MagicPanel } from "../sign-in/magic";
import { AFTER_THE_DAY } from "./worklist.layout";

export function HandledForYouPanel() {
  const t = useT();
  return (
    <Panel className={AFTER_THE_DAY} title={t("magic.lane.done")}>
      <MagicPanel only="done" emptyLabel={t("worklist.handled.empty")} />
    </Panel>
  );
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { problemMessageOf } from "../screens/common";
import { Callout } from "./callout";
import { PanelBody } from "./panel";
import "./panel.css";

// Said once for the whole card rather than on each refused control.
export function PanelNotices({
  readOnly,
  refused,
}: Readonly<{
  /** The read-only posture's title, or false for a reader who may write. */
  readOnly?: string | false;
  /** The card's last refused write: its title, and the failure behind it. */
  refused?: Readonly<{ title: string; error: unknown }>;
}>) {
  const t = useT();
  if (!readOnly && refused === undefined) return null;
  return (
    <PanelBody className="panel-notices">
      {readOnly && <Callout kind="standing" title={readOnly} />}
      {refused !== undefined && (
        <Callout kind="outcome" tone="danger" title={refused.title}>
          {problemMessageOf(refused.error, t)}
        </Callout>
      )}
    </PanelBody>
  );
}

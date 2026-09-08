// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What a message is filed against, named and reachable, on the drawer's
// envelope block.
//
// The presentation has carried `links` all along and the drawer drew none of
// them: a rep reading a mail could see who it was with and not which account,
// deal or project it belongs to — the one question a message on a shared
// timeline most often raises. The link table carries ids and no names, so
// naming one is a record read, which is why this lives here rather than in the
// catalog component that draws the line.
//
// Each name is `EntityRef`, so a record referenced from a message is named,
// linked and cached exactly as it is everywhere else in the product — and a
// name the reader may not resolve degrades the same way here as there.

import { Fragment } from "react";

import type { components } from "../api/schema";
import { EntityRef } from "./entityref";

type EmailPresentation = components["schemas"]["EmailPresentation"];

export function EmailRecordLinks({
  presentation,
}: Readonly<{ presentation: EmailPresentation }>) {
  // Null, not an empty fragment: the drawer reads a nullish answer as "nothing
  // is filed here" and leaves the label off, and an empty element is truthy.
  if (presentation.links.length === 0) {
    return null;
  }
  return (
    <>
      {presentation.links.map((link, index) => (
        <Fragment key={`${link.entity_type}:${link.entity_id}`}>
          {index > 0 && ", "}
          {/* A NEW tab, because this reference sits inside a dialog: following
              it in place would close the message being read to reach a record
              the reader can also open from the page behind it. */}
          <EntityRef kind={link.entity_type} id={link.entity_id} newTab />
        </Fragment>
      ))}
    </>
  );
}

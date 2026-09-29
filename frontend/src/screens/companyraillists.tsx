// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { navigate } from "../app/router";
import { Disclosure } from "../design-system/atoms";
import { PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { SectionSummary } from "./companyrailshared";
import "./lists.css";

type List = components["schemas"]["List"];

/**
 * The Shortlists this account is on that the reader may find, each opening
 * its list. The page reads carry the section only while lists are switched
 * on; absent, the rail draws nothing rather than an empty slice.
 */
export function ListsSection({ lists }: Readonly<{ lists?: readonly List[] }>) {
  const t = useT();
  if (!lists || lists.length === 0) {
    return null;
  }
  return (
    <Disclosure
      className="co-sect"
      summary={
        <SectionSummary title={t("lists.onShortlists")} count={lists.length} />
      }
    >
      <PanelBody>
        <ul className="co-rail-lists">
          {lists.map((list) => (
            <li key={list.id}>
              <button
                type="button"
                className="link-button"
                onClick={() => navigate({ screen: "lists", id: list.id })}
              >
                {list.name}
              </button>
            </li>
          ))}
        </ul>
      </PanelBody>
    </Disclosure>
  );
}

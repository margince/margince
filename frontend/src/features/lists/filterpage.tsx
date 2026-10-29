// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// A new filter on one record type (AC-filters-and-views-1/3/4/5).
//
// It starts calm — the page's name, the record type, and two ways in — and
// grows only as the reader builds. The count and the rows arrive with the
// first complete condition; before then there is nothing to count, and the
// line under the editor says what would make there be.

import { useState } from "react";
import { useGuardedLeave } from "../../components/unsaved";
import { SegmentedControl } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { useFilterVocabulary } from "../../lib/filterdata";
import {
  isObjectTab,
  type ObjectTab,
  resourceOf,
  useObjectTabs,
  useTabWords,
} from "../../lib/filtersaddress";
import { type Group, isComplete, newGroup } from "../../lib/segmentpredicate";
import { useFilterDraft } from "./filterdraft";
import { FilterEditor } from "./filtereditor";
import { useFilterExport } from "./filterexport";
import { FilterFoot } from "./filterfoot";
import { FocusedHead } from "./filterhead";
import { FilterOutcome } from "./filtermatches";
import { usePlainWords } from "./filterpropose";
import { SaveFilterModal, useLandOnSaved } from "./filtersave";
import "./filters.css";

export function FilterPage({ tab }: Readonly<{ tab: ObjectTab }>) {
  const t = useT();
  const [draft, dispatch] = useFilterDraft(() => newGroup("and"));
  const [switchTo, setSwitchTo] = useState<ObjectTab | null>(null);
  // The tree as the reader saw it when they pressed Save: an answer landing
  // behind the dialog must not become part of what they named and saved.
  const [saving, setSaving] = useState<Group | null>(null);
  const resource = resourceOf(tab);
  const listed = useObjectTabs();
  // A kind the directory does not list still owns this page. Its unit may be
  // gone, or the seat may lack the read right. It stays selectable.
  const tabs = listed.includes(tab) ? listed : [...listed, tab];
  const wordsOf = useTabWords();
  const current = wordsOf(tab);
  const words = usePlainWords({ resource, dispatch });
  const vocabulary = useFilterVocabulary(resource);
  const exportRun = useFilterExport();
  const complete = isComplete(draft.tree);
  // A half-built condition is nothing to lose; a complete one is a filter the
  // reader could have kept. Every way off the page that already asked, or
  // kept the filter, leaves through here so the guard does not ask again.
  const leave = useGuardedLeave(complete);
  const land = useLandOnSaved(tab, leave);

  const empty = draft.tree.children.length === 0;
  const records = current.unit;
  const switchTab = (next: ObjectTab) => {
    if (next === tab) {
      return;
    }
    // A PUSH: the record type is what the reader came here for, so Back
    // returns to the last one they looked at. Conditions name one type's
    // fields, so leaving a type with some on screen asks first.
    if (empty) {
      leave({ screen: "filters", id: next });
      return;
    }
    setSwitchTo(next);
  };

  return (
    <div className="wrap filters-screen">
      <FocusedHead
        title={current.newFilter}
        control={
          <SegmentedControl<string>
            options={tabs}
            value={tab}
            onChange={(next) => {
              if (isObjectTab(next)) {
                switchTab(next);
              }
            }}
            labels={Object.fromEntries(
              tabs.map((option) => [option, wordsOf(option).tab]),
            )}
            label={t("filters.objectLabel")}
          />
        }
      />
      <Panel
        title={t("filters.find", { records })}
        // Once there is a filter to keep. Every verb here sends the tree,
        // and an unfinished one is a tree the engine refuses.
        footer={
          complete ? (
            <FilterFoot
              resource={resource}
              tree={draft.tree}
              mode={{ kind: "new" }}
              onSave={() => setSaving(draft.tree)}
              exportRun={exportRun}
            />
          ) : undefined
        }
      >
        <PanelBody>
          <FilterEditor
            draft={draft}
            dispatch={dispatch}
            vocabulary={vocabulary}
            words={words}
            records={records}
          />
        </PanelBody>
      </Panel>
      <FilterOutcome tab={tab} tree={draft.tree} vocabulary={vocabulary} />
      <SaveFilterModal
        open={saving !== null}
        onClose={() => setSaving(null)}
        tab={tab}
        tree={saving ?? draft.tree}
        onSaved={(kind, id, name) => {
          setSaving(null);
          land(kind, id, name);
        }}
      />
      <ConfirmModal
        open={switchTo !== null}
        onClose={() => setSwitchTo(null)}
        title={t("filters.switch.title", {
          records: switchTo ? wordsOf(switchTo).unit : "",
        })}
        confirmLabel={t("filters.switch.confirm")}
        onConfirm={() => {
          if (switchTo) {
            leave({ screen: "filters", id: switchTo });
          }
          setSwitchTo(null);
        }}
      >
        <p>{current.switchClears}</p>
      </ConfirmModal>
    </div>
  );
}

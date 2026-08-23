// The ONE section a record shows its projects in, and the one place a reader
// attaches or detaches one.
//
// A deal, a company and a contact all answer the same question — which bodies
// of work is this record part of — and before this they answered it three ways:
// the deal form had a picker, the company and contact pages read their projects
// only to fill a meeting brief's scope, and nothing offered attach or detach at
// all. Three spellings of one question is how the two drift until they disagree
// in front of a user.
//
// The record page supplies an ADAPTER: what is linked, how to link, how to
// unlink, and whether a second link is allowed. Everything else — the surface,
// the verbs, the empty state, the confirm, the pending and refused states — is
// here, so every record page says it the same way.

import { type ReactNode, useState } from "react";

import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { Button, EmptyState } from "./atoms";
import { ConfirmModal } from "./confirmmodal";
import { Panel, PanelBody, PanelRow } from "./panel";
import { RecordPicker, type RecordPickerCandidate } from "./recordpicker";
import "./projectlinks.css";

// One project as this section shows it: enough to name it, phase it, and open
// it. The same four fields the 360 sections already carry, so a page hands in
// what it already has rather than fetching a second shape.
export type LinkedProject = Readonly<{
  // The linked record's id. Named for the project because the projects side is
  // the common case; the project page's own mirror puts a company id here, and
  // the two rows are the same three slots either way.
  project_id: string;
  name: string;
  // Where the name points. Defaults to the project page, which is right for
  // every caller listing PROJECTS; the mirror listing companies overrides it.
  href?: string;
  key?: string | null;
  // The phase as the CALLER draws it. A node rather than the phase string,
  // because the badge that renders a phase lives with the projects screen and
  // this tier may not reach into a screen — and a second badge drawn here would
  // be exactly the duplication this component exists to end.
  phase?: ReactNode;
}>;

// What a record page has to answer for its own projects. Everything a record
// KIND differs on lives here; nothing else about this section does.
export type ProjectLinksAdapter = Readonly<{
  // The projects on this record, already read by the page.
  linked: readonly LinkedProject[];
  // Whether the reader may change them at all — a read-only page, an archived
  // record, a seat without the write grant.
  readOnly?: boolean;
  // Whether a SECOND project may be attached. A deal carries at most one, so
  // its adapter answers false once it has one, and the verb becomes "move"
  // rather than "attach" — the same section, one truthful word apart.
  allowsMany: boolean;
  // Search for a project to attach. The page decides the search space: a
  // company's page offers its own company's projects, a deal's offers the
  // deal's company's.
  search: (query: string) => Promise<RecordPickerCandidate[]>;
  // Attach the picked project. Rejecting leaves the dialog open with the pick
  // still made, so a refused write is legible rather than silent.
  attach: (projectID: string) => Promise<void>;
  // Take this project off the record. Absent means the link cannot be removed
  // from HERE — a project's own company list says so on the project page.
  detach?: (projectID: string) => Promise<void>;
  // Start a new project on this record, when the page offers it.
  onCreate?: () => void;
}>;

export function ProjectLinks({
  adapter,
  titleKey,
  emptyBody,
  searchLabel,
}: Readonly<{
  adapter: ProjectLinksAdapter;
  // The section's own heading, because a project page saying "Companies" and a
  // company page saying "Projects" are the same section pointed the other way.
  titleKey: MessageKey;
  // One line saying how a link of this kind comes to exist, shown when there
  // are none. A bare "nothing here" tells a reader what they can already see.
  emptyBody: MessageKey;
  // What the picker asks for, already translated. Defaulted rather than
  // required because the projects case is every caller but one, and a label
  // saying "search projects" on the company mirror would be wrong.
  searchLabel?: string;
}>) {
  const t = useT();
  const [picking, setPicking] = useState(false);
  const [detaching, setDetaching] = useState<LinkedProject | null>(null);
  const [busy, setBusy] = useState(false);
  const [refusal, setRefusal] = useState<string | null>(null);

  const canAdd =
    !adapter.readOnly && (adapter.allowsMany || adapter.linked.length === 0);
  // A deal that already has a project offers to MOVE it rather than to add a
  // second, because that is what the write does.
  const moving = !adapter.allowsMany && adapter.linked.length > 0;

  async function run(work: () => Promise<void>, done: () => void) {
    setBusy(true);
    setRefusal(null);
    try {
      await work();
      done();
    } catch (error) {
      // The dialog stays open carrying the refusal: a write the server refused
      // is the one moment the reader most needs to see what they asked for.
      setRefusal(error instanceof Error ? error.message : String(error));
    } finally {
      setBusy(false);
    }
  }

  return (
    <Panel
      title={t(titleKey)}
      titleAction={
        !adapter.readOnly && (
          <div className="pl-verbs">
            {adapter.onCreate && (
              <Button variant="ghost" onClick={adapter.onCreate}>
                {t("projectLinks.new")}
              </Button>
            )}
            {canAdd || moving ? (
              <Button variant="ghost" onClick={() => setPicking(true)}>
                {t(moving ? "projectLinks.move" : "projectLinks.attach")}
              </Button>
            ) : null}
          </div>
        )
      }
    >
      {adapter.linked.length === 0 ? (
        <PanelBody>
          <EmptyState title={t("projectLinks.emptyTitle")}>
            <p>{t(emptyBody)}</p>
          </EmptyState>
        </PanelBody>
      ) : (
        adapter.linked.map((project) => (
          <PanelRow key={project.project_id}>
            <div className="pl-row">
              <a
                className="pl-name"
                href={project.href ?? `#/projects/${project.project_id}`}
              >
                {project.name}
              </a>
              {project.key && (
                <span className="t-mono pl-key">{project.key}</span>
              )}
              {project.phase}
              {adapter.detach && !adapter.readOnly && (
                <Button variant="ghost" onClick={() => setDetaching(project)}>
                  {t("projectLinks.detach")}
                </Button>
              )}
            </div>
          </PanelRow>
        ))
      )}

      {/* The pick dialog is a ConfirmModal with no confirm of its own: picking
        IS the act, so a second "OK" would ask a reader to agree with what they
        just chose. Its pending and error slots are what carry a refused write,
        which is why this is not a bare Modal. */}
      {picking && (
        <ConfirmModal
          open
          onClose={() => setPicking(false)}
          title={t(moving ? "projectLinks.move" : "projectLinks.attach")}
          confirmLabel={t("projectLinks.attach")}
          confirmDisabled
          onConfirm={() => undefined}
          pending={busy}
          error={refusal}
        >
          <RecordPicker
            label={searchLabel ?? t("projectLinks.searchLabel")}
            searchTargets={adapter.search}
            disabled={busy}
            onPick={(candidate) =>
              run(
                () => adapter.attach(candidate.id),
                () => setPicking(false),
              )
            }
          />
        </ConfirmModal>
      )}

      {detaching && adapter.detach && (
        <ConfirmModal
          open
          onClose={() => setDetaching(null)}
          title={t("projectLinks.detachTitle")}
          confirmLabel={t("projectLinks.detachConfirm")}
          confirmVariant="danger"
          pending={busy}
          error={refusal}
          onConfirm={() =>
            run(
              () => adapter.detach?.(detaching.project_id) ?? Promise.resolve(),
              () => setDetaching(null),
            )
          }
        >
          <p>{t("projectLinks.detachBody", { name: detaching.name })}</p>
        </ConfirmModal>
      )}
    </Panel>
  );
}

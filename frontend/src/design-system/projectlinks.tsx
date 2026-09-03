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

import { type ReactNode, useRef, useState } from "react";

import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf } from "../screens/common";
import { Button, EmptyState, Field } from "./atoms";
import { ConfirmModal } from "./confirmmodal";
import { Panel, PanelBody, PanelGroupHead, PanelRow } from "./panel";
import { RecordPicker, type RecordPickerCandidate } from "./recordpicker";
import { Select } from "./select";
import "./projectlinks.css";

// One project as this section shows it: enough to name it, phase it, and open
// it. The same four fields the 360 sections already carry, so a page hands in
// what it already has rather than fetching a second shape.
// What a section calls the thing it links. Every string a reader or a screen
// reader meets, so the mirror cannot half-rename itself.
export type LinkWords = Readonly<{
  attach: string;
  move: string;
  detachTitle: string;
  search: string;
}>;

// One role a link may hold, with the words the reader sees.
export type LinkRole = Readonly<{ value: string; label: string }>;

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
  //
  // It is the RECORD's answer, not each linked project's: the 360 rows carry no
  // per-project writability, so a reader holding the record's write grant but
  // not one project's still sees the verb. That refusal is the server's to
  // make, and it makes it — the dialog stays open carrying the 403's own words
  // rather than the verb quietly doing nothing. Hiding the verb on a guess
  // would be worse: a reader who MAY write the project would be told they
  // cannot, with nothing to read and nowhere to appeal.
  readOnly?: boolean;
  // Whether a SECOND project may be attached. A deal carries at most one, so
  // its adapter answers false once it has one, and the verb becomes "move"
  // rather than "attach" — the same section, one truthful word apart.
  allowsMany: boolean;
  // Search for a project to attach. The page decides the search space: a
  // company's page offers its own company's projects, a deal's offers the
  // deal's company's.
  search: (query: string) => Promise<RecordPickerCandidate[]>;
  // What a link can BE, and what the picker offers. A record's place on a
  // project is a claim — a customer is not a subcontractor, a sponsor is not a
  // bystander — so the reader says which, rather than the page choosing one for
  // them and re-roling whatever they picked.
  roles: readonly LinkRole[];
  // Attach the picked project with the role the reader chose. Rejecting leaves
  // the dialog open with the pick still made, so a refused write is legible
  // rather than silent.
  attach: (projectID: string, role: string) => Promise<void>;
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
  words,
  bare = false,
}: Readonly<{
  adapter: ProjectLinksAdapter;
  // The section's own heading, because a project page saying "Companies" and a
  // company page saying "Projects" are the same section pointed the other way.
  titleKey: MessageKey;
  // One line saying how a link of this kind comes to exist, shown when there
  // are none. A bare "nothing here" tells a reader what they can already see.
  emptyBody: MessageKey;
  // The words this section uses for the thing being linked, already
  // translated. Defaulted to the projects wording because that is every caller
  // but one — the project page's own mirror links COMPANIES, and a section
  // saying "Attach project" about a company is wrong on screen and wrong in the
  // dialog's accessible name.
  words?: LinkWords;
  // Render the section as a GROUP inside a pane the caller holds — a group
  // head one level under the pane's own title, with the verbs, then the rows —
  // rather than as a pane of its own. The company glance does: the projects
  // stand under the deals in the one pane that holds the money, and a pane
  // inside a pane is two borders around one list.
  bare?: boolean;
}>) {
  const t = useT();
  const [picking, setPicking] = useState(false);
  const [detaching, setDetaching] = useState<LinkedProject | null>(null);
  const [busy, setBusy] = useState(false);
  const [refusal, setRefusal] = useState<string | null>(null);
  const [role, setRole] = useState(adapter.roles[0]?.value ?? "");
  // The projects wording, which every caller but the project page's own mirror
  // wants. Spelled once so a caller overriding one string cannot leave the
  // other three saying "project" about a company.
  const said: LinkWords = words ?? {
    attach: t("projectLinks.attach"),
    move: t("projectLinks.move"),
    detachTitle: t("projectLinks.detachTitle"),
    search: t("projectLinks.searchLabel"),
  };
  // Focus returns to the SECTION rather than to the row's own verb: a
  // successful detach removes that button, and focus restored to a node that
  // is about to unmount leaves a keyboard reader nowhere.
  const section = useRef<HTMLDivElement>(null);

  const canAdd =
    !adapter.readOnly && (adapter.allowsMany || adapter.linked.length === 0);
  // A deal that already has a project offers to MOVE it rather than to add a
  // second, because that is what the write does.
  const moving = !adapter.allowsMany && adapter.linked.length > 0;

  // A dialog cannot be dismissed while its write is in flight. Escape and the
  // backdrop both reach onClose, and closing there loses the refusal the write
  // is about to return — the caller sees nothing and believes it worked.
  function closeWhenIdle(close: () => void) {
    return () => {
      if (!busy) {
        close();
      }
    };
  }

  async function run(work: () => Promise<void>, done: () => void) {
    setBusy(true);
    setRefusal(null);
    try {
      await work();
      done();
    } catch (error) {
      // The dialog stays open carrying the refusal: a write the server refused
      // is the one moment the reader most needs to see what they asked for.
      setRefusal(problemMessageOf(error, t));
    } finally {
      setBusy(false);
    }
  }

  const verbs = !adapter.readOnly && (
    <div className="pl-verbs">
      {adapter.onCreate && (
        <Button variant="ghost" onClick={adapter.onCreate}>
          {t("projectLinks.new")}
        </Button>
      )}
      {canAdd || moving ? (
        <Button variant="ghost" onClick={() => setPicking(true)}>
          {moving ? said.move : said.attach}
        </Button>
      ) : null}
    </div>
  );
  const rows =
    adapter.linked.length === 0 ? (
      <PanelBody>
        {bare ? (
          <EmptyState plate title={t("projectLinks.emptyTitle")}>
            {t(emptyBody)}
          </EmptyState>
        ) : (
          <EmptyState title={t("projectLinks.emptyTitle")}>
            <p>{t(emptyBody)}</p>
          </EmptyState>
        )}
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
              <Button
                variant="ghost"
                onClick={() => setDetaching(project)}
                // Every row's verb reads the same to the eye and must not to
                // a screen reader: a button list of five "Detach" entries
                // cannot say which record each one takes off.
                aria-label={t("projectLinks.detachNamed", {
                  name: project.name,
                })}
              >
                {t("projectLinks.detach")}
              </Button>
            )}
          </div>
        </PanelRow>
      ))
    );
  const dialogs = (
    <>
      <AttachDialog
        open={picking}
        adapter={adapter}
        role={role}
        onRole={setRole}
        moving={moving}
        busy={busy}
        refusal={refusal}
        words={said}
        onClose={closeWhenIdle(() => setPicking(false))}
        onPick={(id) =>
          run(
            () => adapter.attach(id, role),
            () => setPicking(false),
          )
        }
      />
      <DetachDialog
        project={detaching}
        detach={adapter.detach}
        busy={busy}
        refusal={refusal}
        returnFocusTo={() => section.current}
        words={said}
        onClose={closeWhenIdle(() => setDetaching(null))}
        onConfirm={(id) =>
          run(
            () => adapter.detach?.(id) ?? Promise.resolve(),
            () => setDetaching(null),
          )
        }
      />
    </>
  );
  if (bare) {
    return (
      <div ref={section}>
        <PanelGroupHead title={t(titleKey)} level="h3" action={verbs} />
        {rows}
        {dialogs}
      </div>
    );
  }
  return (
    <div ref={section}>
      <Panel title={t(titleKey)} titleAction={verbs}>
        {rows}
        {dialogs}
      </Panel>
    </div>
  );
}

// The pick dialog, lifted out of the section: it carries a role, a search and a
// refusal, and folding all three back in is what took the section past what one
// function should ask a reader to hold at once.
//
// It confirms nothing of its own — picking IS the act — so its confirm button
// is permanently disabled, present only because ConfirmModal's pending and
// error slots are what make a refused write legible.
function AttachDialog({
  open,
  adapter,
  role,
  onRole,
  moving,
  busy,
  refusal,
  words,
  onClose,
  onPick,
}: Readonly<{
  open: boolean;
  adapter: ProjectLinksAdapter;
  role: string;
  onRole: (role: string) => void;
  moving: boolean;
  busy: boolean;
  refusal: string | null;
  words: LinkWords;
  onClose: () => void;
  onPick: (projectID: string) => void;
}>) {
  const t = useT();
  if (!open) {
    return null;
  }
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={moving ? words.move : words.attach}
      confirmLabel={words.attach}
      confirmDisabled
      onConfirm={() => undefined}
      pending={busy}
      error={refusal}
    >
      {adapter.roles.length > 1 && (
        <Field label={t("projectLinks.roleLabel")}>
          {(control) => (
            <Select
              {...control}
              name="pl-role"
              options={adapter.roles.map((one) => ({
                value: one.value,
                label: one.label,
              }))}
              value={role}
              onChange={onRole}
              disabled={busy}
            />
          )}
        </Field>
      )}
      <RecordPicker
        label={words.search}
        searchTargets={adapter.search}
        disabled={busy}
        onPick={(candidate) => onPick(candidate.id)}
      />
    </ConfirmModal>
  );
}

// The detach confirm, lifted out for the same reason.
function DetachDialog({
  project,
  detach,
  busy,
  refusal,
  returnFocusTo,
  words,
  onClose,
  onConfirm,
}: Readonly<{
  project: LinkedProject | null;
  detach?: (projectID: string) => Promise<void>;
  busy: boolean;
  refusal: string | null;
  returnFocusTo: () => HTMLElement | null;
  words: LinkWords;
  onClose: () => void;
  onConfirm: (projectID: string) => void;
}>) {
  const t = useT();
  if (!project || !detach) {
    return null;
  }
  return (
    <ConfirmModal
      open
      onClose={onClose}
      returnFocusTo={returnFocusTo}
      title={words.detachTitle}
      confirmLabel={t("projectLinks.detachConfirm")}
      confirmVariant="danger"
      pending={busy}
      error={refusal}
      onConfirm={() => onConfirm(project.project_id)}
    >
      <p>{t("projectLinks.detachBody", { name: project.name })}</p>
    </ConfirmModal>
  );
}

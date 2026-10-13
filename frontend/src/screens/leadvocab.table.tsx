import { useId, useState } from "react";
import {
  Button,
  Field,
  Modal,
  OverflowMenu,
  TextInput,
} from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { CellStack } from "../design-system/cellstack";
import { DataTable, type DataTableColumn } from "../design-system/datatable";
import { Heading } from "../design-system/heading";
import { Switch } from "../design-system/switch";
import { formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf } from "./common";

// One administered term: a lead source or a disqualification reason. Both
// lists share this table so a source row and a reason row read the same way.
export type VocabTerm = Readonly<{
  id: string;
  label: string;
  active: boolean;
  system?: boolean;
  lead_count?: number;
}>;

export type VocabRename = Readonly<{
  pending: boolean;
  error: unknown;
  reset: () => void;
  save: (term: VocabTerm, label: string, onSaved: () => void) => void;
}>;

export function VocabTable<Term extends VocabTerm>({
  label,
  nameHeader,
  terms,
  canEdit,
  canRemove,
  scoreColumn,
  keptReason,
  renameCaption,
  rename,
  onActive,
  onRemove,
}: Readonly<{
  label: string;
  nameHeader: string;
  terms: readonly Term[];
  canEdit: boolean;
  canRemove: boolean;
  // Sources weigh the score; reasons do not, so the column is the caller's.
  scoreColumn?: DataTableColumn<Term>;
  // Why this term cannot be removed, for a seat that could remove another.
  keptReason: (term: Term) => string;
  renameCaption?: (term: Term) => string;
  rename: VocabRename;
  onActive: (term: Term, active: boolean) => void;
  onRemove: (term: Term) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [renaming, setRenaming] = useState<Term | null>(null);
  const columns: DataTableColumn<Term>[] = [
    {
      key: "name",
      header: nameHeader,
      grow: true,
      render: (term) => (
        <CellStack>
          <span className="lead-vocab-name">{term.label}</span>
          {term.system === true && (
            <span className="t-caption">{t("leadSources.builtIn")}</span>
          )}
        </CellStack>
      ),
    },
    ...(scoreColumn ? [scoreColumn] : []),
    {
      key: "leads",
      header: t("leadVocab.col.leads"),
      align: "end",
      render: (term) => formatNumber(term.lead_count ?? 0, locale),
    },
    {
      key: "offered",
      header: t("leadVocab.col.offered"),
      render: (term) => (
        <Switch
          label={t("leadVocab.offerFor", { label: term.label })}
          labelHidden
          checked={term.active}
          disabled={!canEdit}
          onChange={(next) => onActive(term, next)}
        />
      ),
    },
    {
      key: "actions",
      header: "",
      render: (term) => (
        <TermMenu
          term={term}
          canEdit={canEdit}
          canRemove={canRemove}
          keptReason={keptReason}
          onRename={() => setRenaming(term)}
          onRemove={() => onRemove(term)}
        />
      ),
    },
  ];
  return (
    <div
      className={
        scoreColumn ? "lead-vocab-table lead-vocab-scored" : "lead-vocab-table"
      }
    >
      <DataTable
        label={label}
        columns={columns}
        rows={[...terms]}
        rowKey={(term) => term.id}
      />
      {/* Mounted only while open, so a half-typed name is gone the next time
          Rename is pressed. Closing clears the refusal with the form. */}
      {renaming && (
        <RenameDialog
          term={renaming}
          caption={renameCaption?.(renaming)}
          rename={rename}
          onClose={() => {
            rename.reset();
            setRenaming(null);
          }}
        />
      )}
    </div>
  );
}

// Built-ins and in-use terms are kept: the server answers 409 to the delete,
// so the menu says why instead of offering it.
function removableTerm(term: VocabTerm, canRemove: boolean): boolean {
  return canRemove && term.system !== true && (term.lead_count ?? 0) === 0;
}

function TermMenu<Term extends VocabTerm>({
  term,
  canEdit,
  canRemove,
  keptReason,
  onRename,
  onRemove,
}: Readonly<{
  term: Term;
  canEdit: boolean;
  canRemove: boolean;
  keptReason: (term: Term) => string;
  onRename: () => void;
  onRemove: () => void;
}>) {
  const t = useT();
  if (!canEdit && !canRemove) return null;
  return (
    <OverflowMenu label={t("leadVocab.moreFor", { label: term.label })}>
      {canEdit && <Button onClick={onRename}>{t("leadVocab.rename")}</Button>}
      {removableTerm(term, canRemove) ? (
        <Button variant="danger" onClick={onRemove}>
          {t("leadSources.remove")}
        </Button>
      ) : (
        canRemove && <p className="t-caption">{keptReason(term)}</p>
      )}
    </OverflowMenu>
  );
}

// A real form, so Enter commits; a refused save keeps the typed name in the
// box and says why above it, so the reader fixes it rather than retypes it.
function RenameDialog({
  term,
  caption,
  rename,
  onClose,
}: Readonly<{
  term: VocabTerm;
  caption?: string;
  rename: VocabRename;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  const [draft, setDraft] = useState(term.label);
  const next = draft.trim();
  const changed = next !== "" && next !== term.label;
  return (
    <Modal open onClose={onClose} labelledBy={titleId}>
      <form
        className="form-stack"
        onSubmit={(e) => {
          e.preventDefault();
          if (rename.pending) return;
          if (!changed) {
            onClose();
            return;
          }
          rename.save(term, next, onClose);
        }}
      >
        <Heading size="large" className="t-h3" id={titleId}>
          {t("leadVocab.renameTitle", { label: term.label })}
        </Heading>
        {rename.error !== undefined && rename.error !== null && (
          <Callout
            kind="outcome"
            tone="danger"
            title={t("leadSources.notSaved")}
          >
            {problemMessageOf(rename.error, t)}
          </Callout>
        )}
        <Field label={t("leadVocab.nameField")} hint={caption}>
          {(control) => (
            <TextInput
              {...control}
              data-testid="lead-vocab-rename"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
            />
          )}
        </Field>
        <div className="form-actions">
          <Button variant="ghost" onClick={onClose}>
            {t("deals.cancel")}
          </Button>
          <Button type="submit" variant="primary" pending={rename.pending}>
            {t("leadVocab.save")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

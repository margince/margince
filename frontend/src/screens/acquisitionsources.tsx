import { useState } from "react";
import { useCanWrite } from "../app/capability";
import { Badge, Button, Field, Modal, TextInput } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useT } from "../i18n";
import {
  type AcquisitionSource,
  useAcquisitionSources,
  useCreateAcquisitionSource,
  useUpdateAcquisitionSource,
} from "./acquisitionsources.queries";
import { problemMessageOf, QueryGate } from "./common";

/**
 * Settings → the business channels a deal may be attributed to.
 *
 * Sibling of the lead-source card and deliberately NOT the same list: that one
 * also administers how records reach Margince and what each origin is worth to
 * a lead's score. This one names how an opportunity reached the business, and
 * the two vocabularies would fight if they shared a table.
 *
 * There is no delete. A key a deal has ever carried must stay resolvable or
 * that deal stops rendering its own history, so the only withdrawal is the
 * active switch — which keeps the entry readable everywhere it is already
 * stored while removing it from every picker.
 */
export function AcquisitionSourcesCard() {
  const t = useT();
  const canCreate = useCanWrite("custom_field", "create");
  const canEdit = useCanWrite("custom_field", "update");
  const query = useAcquisitionSources();
  const create = useCreateAcquisitionSource();
  const update = useUpdateAcquisitionSource();
  const [adding, setAdding] = useState(false);
  const failure = [create, update].find((m) => m.isError);
  return (
    <Panel
      title={t("acqSources.title")}
      titleAction={
        canCreate && (
          <Button small onClick={() => setAdding(true)}>
            {t("acqSources.addOpen")}
          </Button>
        )
      }
    >
      <PanelBody>
        <p className="settings-panel-sub">{t("acqSources.sub")}</p>
        <SettingList>
          <SettingRow
            label={t("acqSources.listLabel")}
            layout="stack"
            control={
              <QueryGate query={query} pendingLabel={t("acqSources.loading")}>
                {(sources) => (
                  <ul
                    className="lead-vocab-list"
                    data-testid="acquisition-source-list"
                  >
                    {sources.map((source) => (
                      <AcquisitionSourceRow
                        key={source.id}
                        source={source}
                        canEdit={canEdit}
                        onUpdate={(body) =>
                          update.mutate({ id: source.id, body })
                        }
                      />
                    ))}
                  </ul>
                )}
              </QueryGate>
            }
          />
        </SettingList>
        {!canEdit && <p className="t-caption">{t("acqSources.readOnly")}</p>}
        {failure?.error && (
          <Callout
            tone="danger"
            live="alert"
            title={problemMessageOf(failure.error, t)}
          />
        )}
        {adding && (
          <AddAcquisitionSourceDialog
            onClose={() => setAdding(false)}
            onAdd={(label) =>
              create.mutate({ label }, { onSuccess: () => setAdding(false) })
            }
            pending={create.isPending}
          />
        )}
      </PanelBody>
    </Panel>
  );
}

function AcquisitionSourceRow({
  source,
  canEdit,
  onUpdate,
}: Readonly<{
  source: AcquisitionSource;
  canEdit: boolean;
  onUpdate: (body: { label?: string; active?: boolean }) => void;
}>) {
  const t = useT();
  const [label, setLabel] = useState(source.label);
  return (
    <li className="lead-vocab-row" data-testid={`acq-source-${source.key}`}>
      <Field label={t("acqSources.labelFor", { key: source.key })}>
        {(control) => (
          <TextInput
            {...control}
            value={label}
            disabled={!canEdit}
            onChange={(e) => setLabel(e.target.value)}
            onBlur={() => {
              const next = label.trim();
              // Only on a real change, and never on an emptied field: a blank
              // box is a half-finished edit, not an instruction to erase the
              // name every deal carrying this key renders through.
              if (next && next !== source.label) {
                onUpdate({ label: next });
              } else if (!next) {
                setLabel(source.label);
              }
            }}
          />
        )}
      </Field>
      {/* The KEY, shown because it is what deals actually store and what a
          report groups by — a reader renaming the label needs to see that the
          thing underneath does not move. */}
      <span className="t-mono t-caption lead-vocab-key">{source.key}</span>
      <span className="lead-vocab-flags">
        {source.system && <Badge>{t("acqSources.builtIn")}</Badge>}
        <Switch
          label={t("acqSources.activeFor", { label: source.label })}
          labelHidden
          checked={source.active}
          disabled={!canEdit}
          onChange={(next: boolean) => onUpdate({ active: next })}
        />
      </span>
    </li>
  );
}

function AddAcquisitionSourceDialog({
  onClose,
  onAdd,
  pending,
}: Readonly<{
  onClose: () => void;
  onAdd: (label: string) => void;
  pending: boolean;
}>) {
  const t = useT();
  const [label, setLabel] = useState("");
  return (
    <Modal open onClose={onClose} labelledBy="acq-source-add-title">
      <h2 id="acq-source-add-title" className="t-title">
        {t("acqSources.addTitle")}
      </h2>
      <Field label={t("acqSources.addLabel")} hint={t("acqSources.addHint")}>
        {(control) => (
          <TextInput
            {...control}
            value={label}
            onChange={(e) => setLabel(e.target.value)}
          />
        )}
      </Field>
      <div className="action-row">
        <Button variant="ghost" onClick={onClose}>
          {t("deals.cancel")}
        </Button>
        <Button
          disabled={!label.trim() || pending}
          onClick={() => onAdd(label.trim())}
        >
          {t("acqSources.addConfirm")}
        </Button>
      </div>
    </Modal>
  );
}

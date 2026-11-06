// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Pencil } from "lucide-react";
import { type ReactNode, useId, useRef } from "react";
import { Button } from "./button";
import { describedByIds } from "./describedby";
import { FileDropzoneControl } from "./filedropzone";
import { OverflowMenu } from "./overflowmenu";
import "./imageslot.css";

/** The words of a slot's Edit menu: the pill's verb, its full name, and the two items. */
export type ImageSlotVerbs = Readonly<{
  edit: string;
  editLabel: string;
  upload: string;
  remove: string;
}>;

/**
 * One stored picture as the hero of its row, with an Edit pill under it whose
 * menu uploads or removes it. Nothing takes a file while a write is out. A drop
 * racing a removal lands in whichever order the two answer.
 */
export function ImageSlot({
  shape,
  filled,
  preview,
  editable,
  name,
  labelledBy,
  describedBy,
  refusalId,
  emptyLabel,
  accept,
  verbs,
  uploading,
  removing,
  onPick,
  onRemove,
}: Readonly<{
  shape: "wide" | "square" | "round";
  filled: boolean;
  // What the slot holds now, or the record's monogram when it holds nothing.
  preview: ReactNode;
  editable: boolean;
  /** The slot's printed name. Leave it out where `labelledBy` names it. */
  name?: string;
  labelledBy?: string;
  describedBy?: string;
  // The caller's refusal, which the Edit pill points at while it stands.
  refusalId?: string;
  emptyLabel: string;
  accept: string;
  verbs: ImageSlotVerbs;
  uploading: boolean;
  removing: boolean;
  onPick: (file: File) => void;
  onRemove: () => void;
}>) {
  const nameId = useId();
  const inputId = useId();
  const tile = useRef<HTMLDivElement>(null);
  const busy = uploading || removing;
  // Out of the tab order, since the menu's Upload is the keyboard's way in.
  // The zone stays so a file dropped on the picture lands.
  const control = {
    id: inputId,
    "aria-labelledby": name === undefined ? labelledBy : nameId,
    "aria-describedby": describedByIds(describedBy, refusalId),
    tabIndex: -1,
  };
  const choose = () => {
    tile.current?.querySelector<HTMLInputElement>("input[type=file]")?.click();
  };

  return (
    <section
      className={`image-slot image-slot-${shape}`}
      aria-labelledby={name === undefined ? undefined : nameId}
    >
      <div ref={tile} className="image-slot-tile" aria-busy={busy || undefined}>
        {editable && (
          <FileDropzoneControl
            control={control}
            emptyLabel={emptyLabel}
            accept={accept}
            disabled={busy}
            onPick={onPick}
          />
        )}
        <div className="image-slot-mark">{preview}</div>
      </div>
      {editable && (
        <OverflowMenu
          label={verbs.editLabel}
          text={verbs.edit}
          align="center"
          icon={<Pencil aria-hidden="true" />}
          pending={busy}
        >
          <Button onClick={choose} aria-describedby={refusalId}>
            {verbs.upload}
          </Button>
          {filled && (
            <Button onClick={onRemove} aria-describedby={refusalId}>
              {verbs.remove}
            </Button>
          )}
        </OverflowMenu>
      )}
      {name !== undefined && (
        <span className="t-name" id={nameId}>
          {name}
        </span>
      )}
    </section>
  );
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useRef } from "react";
import { Button } from "./button";
import { describedByIds } from "./describedby";
import { FileDropzoneControl } from "./filedropzone";
import "./imageslot.css";

/** The three verbs of a slot, each as its printed word or its full name. */
export type ImageSlotVerbs = Readonly<{
  add: string;
  replace: string;
  remove: string;
}>;

/**
 * One stored picture in a tile cut to its shape, with the verbs that change it.
 * The tile takes nothing while a write is out: a drop racing a removal lands in
 * whichever order the two answer.
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
  words,
  names = words,
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
  // The caller's refusal, which every verb points at while it stands.
  refusalId?: string;
  emptyLabel: string;
  accept: string;
  words: ImageSlotVerbs;
  // Each verb's accessible name, where two slots on one page would otherwise
  // offer two controls with one name.
  names?: ImageSlotVerbs;
  uploading: boolean;
  removing: boolean;
  onPick: (file: File) => void;
  onRemove: () => void;
}>) {
  const nameId = useId();
  const inputId = useId();
  const tile = useRef<HTMLDivElement>(null);
  const labelId = name === undefined ? labelledBy : nameId;
  const busy = uploading || removing;
  const control = {
    id: inputId,
    "aria-labelledby": labelId,
    "aria-describedby": describedByIds(describedBy, refusalId),
    disabled: busy,
  };
  const choose = () => {
    tile.current?.querySelector<HTMLInputElement>("input[type=file]")?.click();
  };

  return (
    <section
      className={`image-slot image-slot-${shape}`}
      aria-labelledby={name === undefined ? undefined : nameId}
    >
      <div
        ref={tile}
        className={filled ? "image-slot-tile is-filled" : "image-slot-tile"}
        aria-busy={busy || undefined}
      >
        {editable && (
          <FileDropzoneControl
            control={control}
            emptyLabel={emptyLabel}
            accept={accept}
            onPick={onPick}
          />
        )}
        <div className="image-slot-mark">{preview}</div>
      </div>
      {name !== undefined && (
        <span className="t-name" id={nameId}>
          {name}
        </span>
      )}
      {editable && (
        <div className="image-slot-verbs">
          {/* Each verb is disabled by the other one's flight and pending on its
              own, so focus stays on a pressed verb and a repeat press is
              swallowed rather than racing the first request. */}
          <Button
            variant="ghost"
            aria-label={filled ? names.replace : names.add}
            onClick={choose}
            aria-describedby={refusalId}
            disabled={removing}
            pending={uploading}
          >
            {filled ? words.replace : words.add}
          </Button>
          {filled && (
            <Button
              variant="ghost"
              aria-label={names.remove}
              onClick={onRemove}
              aria-describedby={refusalId}
              disabled={uploading}
              pending={removing}
            >
              {words.remove}
            </Button>
          )}
        </div>
      )}
    </section>
  );
}

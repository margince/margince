// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { PICTURE_MAX_PIXELS } from "./picturelimits";

// Far above the square the server stores, so its own resample still has detail to keep.
const SHRUNK_EDGE = 1024;

// A phone photo has more pixels than the server decodes, so its centre square, the part
// the server keeps, is drawn smaller here first. A picture within the bound is sent as
// picked, and so is one this browser cannot draw: the server then judges it.
export async function shrinkPicture(file: File): Promise<File> {
  if (typeof createImageBitmap !== "function") {
    return file;
  }
  let bitmap: ImageBitmap;
  try {
    bitmap = await createImageBitmap(file);
  } catch {
    return file;
  }
  try {
    const { width, height } = bitmap;
    if (width * height <= PICTURE_MAX_PIXELS) {
      return file;
    }
    const side = Math.min(width, height);
    const edge = Math.min(side, SHRUNK_EDGE);
    const canvas = document.createElement("canvas");
    canvas.width = edge;
    canvas.height = edge;
    const context = canvas.getContext("2d");
    if (context === null) {
      return file;
    }
    context.drawImage(
      bitmap,
      (width - side) / 2,
      (height - side) / 2,
      side,
      side,
      0,
      0,
      edge,
      edge,
    );
    const type = file.type === "image/jpeg" ? "image/jpeg" : "image/png";
    const shrunk = await new Promise<Blob | null>((resolve) =>
      canvas.toBlob(resolve, type),
    );
    return shrunk === null ? file : new File([shrunk], file.name, { type });
  } finally {
    bitmap.close();
  }
}

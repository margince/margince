// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { inflateSync } from "node:zlib";

export interface PngSize {
  width: number;
  height: number;
}

export interface Raster extends PngSize {
  channels: number;
  samples: Buffer;
}

const SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);

const CHANNELS_BY_COLOUR_TYPE = new Map([
  [2, 3],
  [6, 4],
]);

interface Chunk {
  type: string;
  body: Buffer;
}

function readPng(file: string): { header: Buffer; chunks: Chunk[] } {
  const bytes = readFileSync(file);
  if (
    !bytes.subarray(0, 8).equals(SIGNATURE) ||
    bytes.toString("latin1", 12, 16) !== "IHDR"
  ) {
    throw new Error(`${file} is not a PNG`);
  }
  const chunks: Chunk[] = [];
  for (let at = 8; at < bytes.length; at += 12 + bytes.readUInt32BE(at)) {
    chunks.push({
      type: bytes.toString("latin1", at + 4, at + 8),
      body: bytes.subarray(at + 8, at + 8 + bytes.readUInt32BE(at)),
    });
  }
  return { header: bytes.subarray(16, 29), chunks };
}

function sizeOf(header: Buffer): PngSize {
  return { width: header.readUInt32BE(0), height: header.readUInt32BE(4) };
}

export function pngSize(file: string): PngSize {
  return sizeOf(readPng(file).header);
}

function paeth(left: number, up: number, upLeft: number): number {
  const estimate = left + up - upLeft;
  const toLeft = Math.abs(estimate - left);
  const toUp = Math.abs(estimate - up);
  const toUpLeft = Math.abs(estimate - upLeft);
  if (toLeft <= toUp && toLeft <= toUpLeft) return left;
  return toUp <= toUpLeft ? up : upLeft;
}

function predict(filter: number, left: number, up: number, upLeft: number) {
  switch (filter) {
    case 0:
      return 0;
    case 1:
      return left;
    case 2:
      return up;
    case 3:
      return (left + up) >> 1;
    case 4:
      return paeth(left, up, upLeft);
    default:
      throw new Error(`unknown PNG row filter ${filter}`);
  }
}

function channelsOf(header: Buffer, file: string): number {
  const interlaced = header.readUInt8(12) !== 0;
  const channels =
    header.readUInt8(8) === 8 && !interlaced
      ? (CHANNELS_BY_COLOUR_TYPE.get(header.readUInt8(9)) ?? 0)
      : 0;
  if (channels === 0) {
    throw new Error(`${file} is not an 8-bit, non-interlaced RGB or RGBA PNG`);
  }
  return channels;
}

function unfilter(filtered: Buffer, size: PngSize, channels: number): Buffer {
  const stride = size.width * channels;
  const samples = Buffer.alloc(size.height * stride);
  for (let y = 0; y < size.height; y++) {
    const filter = filtered.readUInt8(y * (stride + 1));
    for (let x = 0; x < stride; x++) {
      const here = y * stride + x;
      const left = x >= channels ? samples.readUInt8(here - channels) : 0;
      const up = y > 0 ? samples.readUInt8(here - stride) : 0;
      const upLeft =
        x >= channels && y > 0
          ? samples.readUInt8(here - stride - channels)
          : 0;
      const raw = filtered.readUInt8(y * (stride + 1) + 1 + x);
      samples[here] = (raw + predict(filter, left, up, upLeft)) & 0xff;
    }
  }
  return samples;
}

// Reads the 8-bit, non-interlaced RGB and RGBA that a browser screenshot writes.
export function decodePng(file: string): Raster {
  const { header, chunks } = readPng(file);
  const size = sizeOf(header);
  const channels = channelsOf(header, file);
  const compressed = chunks
    .filter((chunk) => chunk.type === "IDAT")
    .map((chunk) => chunk.body);
  const filtered = inflateSync(Buffer.concat(compressed));
  return { ...size, channels, samples: unfilter(filtered, size, channels) };
}

export function colourAt(raster: Raster, x: number, y: number) {
  const at = (y * raster.width + x) * raster.channels;
  const hex = `#${raster.samples.toString("hex", at, at + 3)}`;
  const alpha = raster.channels === 4 ? raster.samples.readUInt8(at + 3) : 255;
  return { hex, alpha };
}

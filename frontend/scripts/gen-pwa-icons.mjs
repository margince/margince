// Bakes the install PNGs in public/ from icon.svg and icon-maskable.svg; run
// `node scripts/gen-pwa-icons.mjs` after either SVG changes.

// Pixels come off a canvas and are encoded here rather than screenshotted: iOS
// paints a transparent pixel black, and only our own encoder can drop alpha.

import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { crc32, deflateSync } from "node:zlib";
import { chromium } from "@playwright/test";

const publicDir = join(dirname(fileURLToPath(import.meta.url)), "..", "public");

const ICONS = [
  { out: "favicon-32.png", from: "icon.svg", size: 32 },
  { out: "icon-192.png", from: "icon.svg", size: 192 },
  { out: "icon-512.png", from: "icon.svg", size: 512 },
  {
    out: "icon-maskable-512.png",
    from: "icon-maskable.svg",
    size: 512,
    maskable: true,
  },
  { out: "apple-touch-icon.png", from: "icon.svg", size: 180, opaque: true },
];

// A launcher may crop a maskable icon to any shape that holds this circle.
const SAFE_RADIUS = 0.4;

const SIGNATURE = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
const RGB = 2;
const RGBA = 6;

const browser = await chromium.launch();
try {
  const page = await browser.newPage();
  for (const icon of ICONS) {
    const svg = readFileSync(join(publicDir, icon.from), "utf8");
    const ground = groundOf(svg, icon.from);
    const rgba = await rasterize(page, svg, icon.size, icon.opaque && ground);
    let note = "";
    if (icon.maskable) {
      const reach = markReach(rgba, icon.size, ground);
      if (reach > SAFE_RADIUS) {
        throw new Error(
          `${icon.out}: the mark reaches ${reach.toFixed(3)} of the width from the centre; a launcher crops past ${SAFE_RADIUS}`,
        );
      }
      note = `  mark reaches ${reach.toFixed(3)} of ${SAFE_RADIUS}`;
    }
    const encoded = icon.opaque
      ? encodePng(withoutAlpha(rgba, icon.out), icon.size, RGB)
      : encodePng(rgba, icon.size, RGBA);
    writeFileSync(join(publicDir, icon.out), encoded);
    process.stdout.write(
      `${icon.out}  ${icon.size}x${icon.size}  ${encoded.length} B${note}\n`,
    );
  }
} finally {
  await browser.close();
}

function groundOf(svg, file) {
  const fill = /<rect\b[^>]*\bfill="(#[0-9a-fA-F]{6})"/.exec(svg);
  if (!fill) {
    throw new Error(
      `${file} has no <rect fill="#rrggbb"> ground to flatten on`,
    );
  }
  return fill[1];
}

// The SVGs declare only a viewBox, and an image with no intrinsic size is laid
// out at 300x150 before it is scaled; sizing the root rasterizes it at `size`.
async function rasterize(page, svg, size, ground) {
  const sized = svg.replace("<svg ", `<svg width="${size}" height="${size}" `);
  const src = `data:image/svg+xml;base64,${Buffer.from(sized).toString("base64")}`;
  const pixels = await page.evaluate(
    async ({ src, size, ground }) => {
      const image = new Image();
      image.src = src;
      await image.decode();
      const canvas = document.createElement("canvas");
      canvas.width = size;
      canvas.height = size;
      const context = canvas.getContext("2d");
      if (ground) {
        context.fillStyle = ground;
        context.fillRect(0, 0, size, size);
      }
      context.drawImage(image, 0, 0, size, size);
      return Array.from(context.getImageData(0, 0, size, size).data);
    },
    { src, size, ground },
  );
  return Buffer.from(pixels);
}

// A share of the width, the unit SAFE_RADIUS is stated in.
function markReach(rgba, size, ground) {
  const [r, g, b] = [1, 3, 5].map((at) =>
    Number.parseInt(ground.slice(at, at + 2), 16),
  );
  const centre = size / 2;
  let reach = 0;
  for (let y = 0; y < size; y++) {
    for (let x = 0; x < size; x++) {
      const at = (y * size + x) * 4;
      if (rgba[at] === r && rgba[at + 1] === g && rgba[at + 2] === b) continue;
      const corner = Math.hypot(
        Math.abs(x + 0.5 - centre) + 0.5,
        Math.abs(y + 0.5 - centre) + 0.5,
      );
      reach = Math.max(reach, corner / size);
    }
  }
  return reach;
}

function withoutAlpha(rgba, file) {
  const rgb = Buffer.alloc((rgba.length / 4) * 3);
  for (let pixel = 0; pixel < rgba.length / 4; pixel++) {
    if (rgba[pixel * 4 + 3] !== 255) {
      throw new Error(`${file}: pixel ${pixel} is not opaque after flattening`);
    }
    rgba.copy(rgb, pixel * 3, pixel * 4, pixel * 4 + 3);
  }
  return rgb;
}

function encodePng(pixels, size, colourType) {
  const header = Buffer.alloc(13);
  header.writeUInt32BE(size, 0);
  header.writeUInt32BE(size, 4);
  header[8] = 8;
  header[9] = colourType;
  const stride = pixels.length / size;
  const scanlines = Buffer.alloc((stride + 1) * size);
  for (let row = 0; row < size; row++) {
    pixels.copy(
      scanlines,
      row * (stride + 1) + 1,
      row * stride,
      (row + 1) * stride,
    );
  }
  return Buffer.concat([
    SIGNATURE,
    chunk("IHDR", header),
    chunk("IDAT", deflateSync(scanlines, { level: 9 })),
    chunk("IEND", Buffer.alloc(0)),
  ]);
}

function chunk(type, data) {
  const typed = Buffer.concat([Buffer.from(type, "latin1"), data]);
  const framed = Buffer.alloc(8 + typed.length);
  framed.writeUInt32BE(data.length, 0);
  typed.copy(framed, 4);
  framed.writeUInt32BE(crc32(typed), 4 + typed.length);
  return framed;
}

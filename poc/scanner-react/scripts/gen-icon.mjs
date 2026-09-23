// Writes public/apple-touch-icon.png: a single-colour 180 x 180 px PNG.
import { mkdirSync, writeFileSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { crc32, deflateSync } from "node:zlib";

const size = 180;
const [r, g, b] = [0x02, 0x84, 0xc7];

function chunk(type, data) {
  const length = Buffer.alloc(4);
  length.writeUInt32BE(data.length);
  const body = Buffer.concat([Buffer.from(type, "ascii"), data]);
  const crc = Buffer.alloc(4);
  crc.writeUInt32BE(crc32(body));
  return Buffer.concat([length, body, crc]);
}

const header = Buffer.alloc(13);
header.writeUInt32BE(size, 0);
header.writeUInt32BE(size, 4);
header.writeUInt8(8, 8); // bit depth
header.writeUInt8(2, 9); // colour type RGB
header.writeUInt8(0, 10); // compression
header.writeUInt8(0, 11); // filter
header.writeUInt8(0, 12); // no interlace

const row = Buffer.alloc(1 + size * 3); // filter byte 0, then RGB pixels
for (let x = 0; x < size; x++) {
  row[1 + x * 3] = r;
  row[2 + x * 3] = g;
  row[3 + x * 3] = b;
}
const pixels = Buffer.concat(Array.from({ length: size }, () => row));

const png = Buffer.concat([
  Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
  chunk("IHDR", header),
  chunk("IDAT", deflateSync(pixels)),
  chunk("IEND", Buffer.alloc(0)),
]);

const target = fileURLToPath(new URL("../public/apple-touch-icon.png", import.meta.url));
mkdirSync(dirname(target), { recursive: true });
writeFileSync(target, png);
console.log(`gen:icon: ${target} (${png.length} bytes)`);

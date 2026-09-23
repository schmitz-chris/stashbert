// Writes public/apple-touch-icon.png: a single-color 180 x 180 PNG.
import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { crc32, deflateSync } from 'node:zlib';

const SIZE = 180;
const COLOR = [0xff, 0x3e, 0x00]; // RGB

function chunk(type, data) {
	const length = Buffer.alloc(4);
	length.writeUInt32BE(data.length);
	const typeAndData = Buffer.concat([Buffer.from(type, 'ascii'), data]);
	const crc = Buffer.alloc(4);
	crc.writeUInt32BE(crc32(typeAndData));
	return Buffer.concat([length, typeAndData, crc]);
}

const header = Buffer.alloc(13);
header.writeUInt32BE(SIZE, 0); // width
header.writeUInt32BE(SIZE, 4); // height
header.writeUInt8(8, 8); // bit depth
header.writeUInt8(2, 9); // color type: truecolor RGB
header.writeUInt8(0, 10); // compression
header.writeUInt8(0, 11); // filter
header.writeUInt8(0, 12); // interlace

// Each scanline starts with filter type 0 (none).
const row = Buffer.concat([Buffer.from([0]), Buffer.from(Array(SIZE).fill(COLOR).flat())]);
const pixels = Buffer.concat(Array(SIZE).fill(row));

const png = Buffer.concat([
	Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]),
	chunk('IHDR', header),
	chunk('IDAT', deflateSync(pixels)),
	chunk('IEND', Buffer.alloc(0))
]);

const publicDir = join(dirname(fileURLToPath(import.meta.url)), '..', 'public');
mkdirSync(publicDir, { recursive: true });
writeFileSync(join(publicDir, 'apple-touch-icon.png'), png);
console.log(`gen:icon: public/apple-touch-icon.png (${png.length} bytes)`);

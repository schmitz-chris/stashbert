// Copies zxing_reader.wasm from the zxing-wasm version that barcode-detector
// depends on into public/ and verifies its SHA-256 against ZXING_WASM_SHA256.
import { createHash } from 'node:crypto';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const projectDir = join(dirname(fileURLToPath(import.meta.url)), '..');
const requireFromProject = createRequire(join(projectDir, 'package.json'));

// Resolve zxing-wasm relative to barcode-detector, not to the project.
const detectorEntry = requireFromProject.resolve('barcode-detector');
const requireFromDetector = createRequire(detectorEntry);
const wasmSource = requireFromDetector.resolve('zxing-wasm/reader/zxing_reader.wasm');

const { ZXING_WASM_SHA256 } = requireFromProject('barcode-detector/ponyfill');

const publicDir = join(projectDir, 'public');
const wasmTarget = join(publicDir, 'zxing_reader.wasm');

// Verify before writing, so a mismatching file never lands in public/.
const wasm = readFileSync(wasmSource);
const actual = createHash('sha256').update(wasm).digest('hex');
if (actual !== ZXING_WASM_SHA256) {
	console.error(`copy:wasm: SHA-256 mismatch for ${wasmSource}`);
	console.error(`  expected ${ZXING_WASM_SHA256}`);
	console.error(`  actual   ${actual}`);
	process.exit(1);
}

mkdirSync(publicDir, { recursive: true });
writeFileSync(wasmTarget, wasm);

console.log(`copy:wasm: ${wasmSource} -> public/zxing_reader.wasm (sha256 ${actual})`);

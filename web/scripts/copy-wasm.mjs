// Copies zxing_reader.wasm from the zxing-wasm version that barcode-detector
// depends on into public/ and verifies it against ZXING_WASM_SHA256 before
// writing. The environment variable ZXING_WASM_SHA256 overrides the expected
// hash (used by copy-wasm.test.mjs).
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { ZXING_WASM_SHA256 } from "barcode-detector/ponyfill";

const require = createRequire(import.meta.url);
const requireFromBarcodeDetector = createRequire(require.resolve("barcode-detector/ponyfill"));
const source = requireFromBarcodeDetector.resolve("zxing-wasm/reader/zxing_reader.wasm");
const target = fileURLToPath(new URL("../public/zxing_reader.wasm", import.meta.url));
const expected = process.env.ZXING_WASM_SHA256 ?? ZXING_WASM_SHA256;

// Verify before writing, so a mismatching file never lands in public/.
const wasm = readFileSync(source);
const actual = createHash("sha256").update(wasm).digest("hex");
if (actual !== expected) {
  console.error(`copy:wasm: SHA-256 mismatch for ${source}`);
  console.error(`  expected ${expected}`);
  console.error(`  actual   ${actual}`);
  process.exit(1);
}

mkdirSync(dirname(target), { recursive: true });
writeFileSync(target, wasm);
console.log(`copy:wasm: ${source} -> ${target} (sha256 ${actual})`);

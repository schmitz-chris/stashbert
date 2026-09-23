// Copies zxing_reader.wasm from the zxing-wasm version that barcode-detector
// depends on into public/ and verifies it against ZXING_WASM_SHA256.
import { createHash } from "node:crypto";
import { copyFileSync, mkdirSync, readFileSync, rmSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { ZXING_WASM_SHA256 } from "barcode-detector/ponyfill";

const require = createRequire(import.meta.url);
const requireFromBarcodeDetector = createRequire(require.resolve("barcode-detector/ponyfill"));
const source = requireFromBarcodeDetector.resolve("zxing-wasm/reader/zxing_reader.wasm");
const target = fileURLToPath(new URL("../public/zxing_reader.wasm", import.meta.url));

mkdirSync(dirname(target), { recursive: true });
copyFileSync(source, target);

const actual = createHash("sha256").update(readFileSync(target)).digest("hex");
if (actual !== ZXING_WASM_SHA256) {
  rmSync(target, { force: true });
  console.error(`copy:wasm: SHA-256 mismatch for ${source}`);
  console.error(`  expected ${ZXING_WASM_SHA256}`);
  console.error(`  actual   ${actual}`);
  process.exit(1);
}
console.log(`copy:wasm: ${source} -> ${target} (sha256 ${actual})`);

// Runs copy-wasm.mjs with a wrong expected hash. The script has to fail and
// leave public/zxing_reader.wasm as it was: unchanged, or still missing.
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { existsSync, readFileSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { expect, it } from "vitest";

const script = fileURLToPath(new URL("./copy-wasm.mjs", import.meta.url));
const target = fileURLToPath(new URL("../public/zxing_reader.wasm", import.meta.url));

function targetState() {
  if (!existsSync(target)) {
    return null;
  }
  return {
    sha256: createHash("sha256").update(readFileSync(target)).digest("hex"),
    mtimeMs: statSync(target).mtimeMs,
  };
}

it("fails on a hash mismatch and writes nothing", () => {
  const before = targetState();
  const result = spawnSync(process.execPath, [script], {
    env: { ...process.env, ZXING_WASM_SHA256: "0".repeat(64) },
    encoding: "utf8",
  });
  expect(result.status).not.toBe(0);
  expect(result.stderr).toContain("SHA-256 mismatch");
  expect(targetState()).toStrictEqual(before);
});

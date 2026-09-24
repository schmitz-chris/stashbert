// Checks the PWA parts of the build in dist/ (plan F13b). It runs as
// postbuild, so a broken PWA build fails npm run build:
//   - dist/manifest.webmanifest has the required fields and both icons,
//   - dist/sw.js precaches zxing_reader.wasm and index.html,
//   - dist/index.html links the manifest and has no inline <script>
//     (CSP, architecture.md 8).
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

const dist = fileURLToPath(new URL("../dist/", import.meta.url));

const expectedManifest = {
  name: "StashBert",
  short_name: "StashBert",
  lang: "de",
  display: "standalone",
  start_url: "/",
  scope: "/",
  theme_color: "#fafaf9",
};
const expectedIcons = ["192x192", "512x512"];
const expectedPrecache = ["zxing_reader.wasm", "index.html"];

const errors = [];

function readDist(name) {
  const file = dist + name;
  if (!existsSync(file)) {
    errors.push(`dist/${name} is missing`);
    return null;
  }
  return readFileSync(file, "utf8");
}

function checkManifest(text) {
  let manifest;
  try {
    manifest = JSON.parse(text);
  } catch (err) {
    errors.push(`dist/manifest.webmanifest is not valid JSON: ${err.message}`);
    return;
  }
  for (const [field, value] of Object.entries(expectedManifest)) {
    if (manifest[field] !== value) {
      errors.push(`manifest: ${field} is ${JSON.stringify(manifest[field])}, want ${JSON.stringify(value)}`);
    }
  }
  const icons = Array.isArray(manifest.icons) ? manifest.icons : [];
  for (const sizes of expectedIcons) {
    const icon = icons.find((i) => i.sizes === sizes);
    if (!icon) {
      errors.push(`manifest: no icon with sizes ${sizes}`);
      continue;
    }
    if (icon.type !== "image/png") {
      errors.push(`manifest: icon ${sizes} has type ${JSON.stringify(icon.type)}, want "image/png"`);
    }
    if (!String(icon.purpose ?? "any").split(" ").includes("any")) {
      errors.push(`manifest: icon ${sizes} has purpose ${JSON.stringify(icon.purpose)}, want "any"`);
    }
    if (typeof icon.src !== "string" || !existsSync(dist + icon.src.replace(/^\//, ""))) {
      errors.push(`manifest: icon ${sizes} points to ${JSON.stringify(icon.src)}, which is not in dist/`);
    }
  }
}

// precacheUrls returns the URLs of the precache list that generateSW writes
// into sw.js as precacheAndRoute([{url:"…",revision:…},…]).
function precacheUrls(sw) {
  const start = sw.indexOf("precacheAndRoute(");
  if (start < 0) {
    return null;
  }
  const list = sw.slice(start, sw.indexOf("]", start));
  return [...list.matchAll(/"?url"?\s*:\s*"([^"]+)"/g)].map((m) => m[1]);
}

function checkServiceWorker(sw) {
  const urls = precacheUrls(sw);
  if (!urls) {
    errors.push("dist/sw.js has no precacheAndRoute call");
    return;
  }
  for (const url of expectedPrecache) {
    if (!urls.includes(url)) {
      errors.push(`sw.js: the precache list lacks ${url}`);
    }
  }
}

function checkIndex(html) {
  for (const [tag, attributes] of html.matchAll(/<script\b([^>]*)>/gi)) {
    if (!/\bsrc\s*=/i.test(attributes)) {
      errors.push(`dist/index.html has an inline script: ${tag}`);
    }
  }
  const manifestLink = [...html.matchAll(/<link\b[^>]*>/gi)].some(
    ([tag]) => /\brel\s*=\s*["']?manifest\b/i.test(tag) && tag.includes("/manifest.webmanifest"),
  );
  if (!manifestLink) {
    errors.push("dist/index.html does not link /manifest.webmanifest");
  }
}

const manifest = readDist("manifest.webmanifest");
if (manifest !== null) {
  checkManifest(manifest);
}
const sw = readDist("sw.js");
if (sw !== null) {
  checkServiceWorker(sw);
}
const index = readDist("index.html");
if (index !== null) {
  checkIndex(index);
}

if (errors.length > 0) {
  console.error("check:pwa: the PWA build is incomplete:");
  for (const error of errors) {
    console.error(`  ${error}`);
  }
  process.exit(1);
}
console.log("check:pwa: manifest, service worker and index.html are complete");

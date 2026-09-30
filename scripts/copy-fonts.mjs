// Copies the self-hosted woff2 brand fonts from node_modules into
// web/static/fonts so `make css` and production deploys stay self-contained.
import { copyFileSync, existsSync, mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const out = resolve(root, "web/static/fonts");
mkdirSync(out, { recursive: true });

const fonts = [
  ["node_modules/geist/dist/fonts/geist-sans/Geist-Variable.woff2", "geist-variable.woff2"],
  [
    "node_modules/@fontsource-variable/jetbrains-mono/files/jetbrains-mono-latin-wght-normal.woff2",
    "jetbrains-mono-variable.woff2",
  ],
  ["node_modules/material-symbols/material-symbols-outlined.woff2", "material-symbols-outlined.woff2"],
];

let missing = 0;
for (const [src, dst] of fonts) {
  const from = resolve(root, src);
  if (!existsSync(from)) {
    console.error(`fonts: missing ${src} — run npm install first`);
    missing += 1;
    continue;
  }
  copyFileSync(from, resolve(out, dst));
  console.log(`fonts: web/static/fonts/${dst}`);
}
if (missing > 0) {
  process.exitCode = 1;
}

import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { join, relative } from "node:path";

const hash = (data) => createHash("sha256").update(data).digest("hex");
const files = (dir) => readdirSync(dir, { withFileTypes: true }).flatMap((entry) =>
  entry.isDirectory() ? files(join(dir, entry.name)) : [join(dir, entry.name)]);
const sourceFiles = [...files("src"), ...(existsSync("public") ? files("public") : []),
  ...readdirSync(".").filter((path) => path === ".env" || path.startsWith(".env.")),
  "index.html", "package.json", "package-lock.json", "tsconfig.json", "vite.config.ts",
  "scripts/record-live-build.mjs"].sort();
const source = createHash("sha256");
for (const path of sourceFiles) source.update(path).update("\0").update(readFileSync(path)).update("\0");
const artifactFiles = files("dist").filter((path) => !path.endsWith("hp1-build-manifest.json")).sort();
const actual = {
  git_head: execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim(),
  source_sha256: source.digest("hex"),
  build_mode: "production",
  vite_environment: Object.fromEntries(Object.entries(process.env).filter(([key]) => key.startsWith("VITE_")).sort(([a], [b]) => a.localeCompare(b))),
  artifacts: Object.fromEntries(artifactFiles.map((path) => [relative("dist", path), hash(readFileSync(path))])),
};
const manifestPath = "dist/hp1-build-manifest.json";
if (process.argv.includes("--check")) {
  const expected = JSON.parse(readFileSync(manifestPath, "utf8"));
  if (JSON.stringify(expected) !== JSON.stringify(actual))
    throw new Error("Live browser assets do not match the current source/head; rebuild and record the manifest");
} else writeFileSync(manifestPath, JSON.stringify(actual, null, 2) + "\n");
process.stdout.write(JSON.stringify(actual) + "\n");

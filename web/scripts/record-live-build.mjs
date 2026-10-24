import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, readdirSync, writeFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";

const hash = (data) => createHash("sha256").update(data).digest("hex");
const files = (dir) => readdirSync(dir, { withFileTypes: true }).flatMap((entry) =>
  entry.isDirectory() ? files(join(dir, entry.name)) : [join(dir, entry.name)]);
const head = () => execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8" }).trim();
const sourceHash = () => {
  const paths = [...files("src"), ...(existsSync("public") ? files("public") : []),
    ...readdirSync(".").filter((path) => path === ".env" || path.startsWith(".env.")),
    "index.html", "package.json", "package-lock.json", "tsconfig.json", "vite.config.ts",
    "scripts/record-live-build.mjs"].sort();
  const source = createHash("sha256");
  for (const path of paths) source.update(path).update("\0").update(readFileSync(path)).update("\0");
  return source.digest("hex");
};
const sourceBefore = sourceHash(), headBefore = head();
const checking = process.argv.includes("--check");
if (!checking) {
  // Recording is inseparable from the actual build. Never bless preexisting
  // assets merely by hashing them after an unrelated source change.
  execFileSync("npm", ["run", "build"], {
    stdio: "inherit",
    env: { ...process.env, NODE_ENV: "production", PATH: `${dirname(process.execPath)}:${process.env.PATH ?? ""}` },
  });
  if (sourceBefore !== sourceHash() || headBefore !== head())
    throw new Error("Frontend source/head changed while building; rebuild a frozen checkout");
}
const artifactFiles = files("dist").filter((path) => relative("dist", path) !== "hp1-build-manifest.json").sort();
const actual = {
  git_head: headBefore,
  source_sha256: sourceBefore,
  build_mode: "production",
  node_environment: "production",
  vite_environment: Object.fromEntries(Object.entries(process.env).filter(([key]) => key.startsWith("VITE_")).sort(([a], [b]) => a.localeCompare(b))),
  artifacts: Object.fromEntries(artifactFiles.map((path) => [relative("dist", path), hash(readFileSync(path))])),
};
const manifestPath = "dist/hp1-build-manifest.json";
if (checking) {
  const expected = JSON.parse(readFileSync(manifestPath, "utf8"));
  if (JSON.stringify(expected) !== JSON.stringify(actual))
    throw new Error("Live browser assets do not match the current source/head; rebuild with this recorder");
} else writeFileSync(manifestPath, JSON.stringify(actual, null, 2) + "\n");
process.stdout.write(JSON.stringify(actual) + "\n");

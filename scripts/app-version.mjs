import { spawnSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { checkFrontendVersion, compareVersions, normalizeVersion } from "./frontend-version.mjs";

const root = resolve(dirname(fileURLToPath(import.meta.url)), "..");

export function publishedVersion(releases) {
  if (!Array.isArray(releases)) throw new Error("Expected a GitHub releases array");
  let best;
  for (const release of releases) {
    if (!release || typeof release.draft !== "boolean") throw new Error("Invalid GitHub release response");
    if (release.draft) continue;
    const version = normalizeVersion(release.tag_name);
    if (!best || compareVersions(version, best) > 0) best = version;
  }
  if (!best) throw new Error("No published TauShell releases found (prereleases are included)");
  return best;
}

export function githubVersion(repository) {
  if (!/^[A-Za-z0-9_.-]+\/[A-Za-z0-9_.-]+$/.test(repository)) throw new Error("Expected owner/repository");
  const result = spawnSync("gh", ["api", "--paginate", "--slurp", `repos/${repository}/releases?per_page=100`], {
    encoding: "utf8", windowsHide: true, timeout: 60000,
  });
  if (result.error) throw new Error(`Could not query GitHub releases: ${result.error.message}`, { cause: result.error });
  if (result.status !== 0) throw new Error(`GitHub release query failed: ${result.stderr.trim()}`);
  const pages = JSON.parse(result.stdout);
  if (!Array.isArray(pages) || pages.some((page) => !Array.isArray(page))) throw new Error("Invalid paginated GitHub release response");
  return publishedVersion(pages.flat());
}

export function androidVersionCode(value) {
  const version = normalizeVersion(value);
  const [major, minor, patch] = version.split(/[-+]/, 1)[0].split(".").map(Number);
  const code = major * 10000 + minor * 100 + patch;
  if (minor > 99 || patch > 99 || !Number.isSafeInteger(code) || code < 1 || code > 2100000000) {
    throw new Error(`Version ${version} cannot be represented by the Android major*10000 + minor*100 + patch scheme`);
  }
  return code;
}

export function checkAndroidVersion(metadata, value) {
  const version = normalizeVersion(value);
  const code = androidVersionCode(version);
  if (!Array.isArray(metadata?.elements) || !metadata.elements.length) throw new Error("Android output metadata has no APK elements");
  for (const element of metadata.elements) {
    if (element.versionName !== version || element.versionCode !== code) {
      throw new Error(`Android version mismatch: expected ${version} (${code}); got ${element.versionName} (${element.versionCode})`);
    }
  }
}

function main(args) {
  const [command, value, file, ...extra] = args;
  if (extra.length) throw new Error("Too many app-version arguments");
  if (command === "android" && value && file) {
    checkAndroidVersion(JSON.parse(readFileSync(resolve(file), "utf8")), value);
    console.log(`Android APK version ${normalizeVersion(value)} verified`);
  } else if ((command === "check" || command === "github") && !file) {
    const version = command === "github"
      ? githubVersion(value ?? "ma99us/tau-shell")
      : normalizeVersion(value ?? JSON.parse(readFileSync(join(root, "frontend", "package.json"), "utf8")).version);
    checkFrontendVersion(root, version);
    console.log(`App manifests match ${command === "github" ? "published GitHub release " : ""}${version}`);
  } else {
    throw new Error("Usage: app-version.mjs check [version] | github [owner/repository] | android <version> <output-metadata.json>");
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(`App version check failed: ${error.message}`);
    process.exitCode = 1;
  }
}

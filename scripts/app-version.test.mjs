import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { androidVersionCode, checkAndroidVersion, publishedVersion } from "./app-version.mjs";

test("published version includes prereleases, excludes drafts and ignores release listing order", () => {
  assert.equal(publishedVersion([
    { tag_name: "v1.0.0", draft: false },
    { tag_name: "v9.0.0", draft: true },
    { tag_name: "v1.0.1-rc.1", draft: false },
    { tag_name: "v1.0.1", draft: false },
  ]), "1.0.1");
  assert.equal(publishedVersion([{ tag_name: "v1.0.1-rc.1", draft: false }]), "1.0.1-rc.1");
  assert.throws(() => publishedVersion([]), /No published/);
  assert.throws(() => publishedVersion([{ tag_name: "not-a-version", draft: false }]), /Invalid release version/);
  assert.throws(() => publishedVersion([{ tag_name: "v1.0.1" }]), /Invalid GitHub release/);
});

test("APK metadata must match the full release version and its numeric code", () => {
  for (const version of ["1.0.1", "1.0.1-rc.1", "1.0.1+build.9"]) {
    assert.equal(androidVersionCode(version), 10001);
    const metadata = { elements: [{ versionName: version, versionCode: 10001 }] };
    assert.doesNotThrow(() => checkAndroidVersion(metadata, version));
    assert.throws(() => checkAndroidVersion(metadata, "1.0.2"), /Android version mismatch/);
    assert.throws(() => checkAndroidVersion({ elements: [...metadata.elements, { versionName: version, versionCode: 1 }] }, version), /Android version mismatch/);
  }
  assert.throws(() => checkAndroidVersion({ elements: [] }, "1.0.1"), /no APK/);
  for (const version of ["0.0.0", "1.100.0", "1.0.100", "999999999999999999999.0.0"]) {
    assert.throws(() => androidVersionCode(version), /cannot be represented/);
  }
});

test("CI checks published metadata before replacing it with the smoke version", () => {
  const ci = readFileSync(new URL("../.github/workflows/ci.yml", import.meta.url), "utf8");
  assert.match(ci, /app-version\.mjs github/);
  assert.ok(ci.indexOf("app-version.mjs github") < ci.indexOf("frontend-version.mjs set"));
  const release = readFileSync(new URL("../.github/workflows/release.yml", import.meta.url), "utf8");
  assert.match(release, /app-version\.mjs check \$env:VERSION/);
  assert.match(release, /app-version\.mjs android \$bare/);
  const gradle = readFileSync(new URL("../android/app/build.gradle.kts", import.meta.url), "utf8");
  assert.match(gradle, /frontend\/package\.json/);
  assert.doesNotMatch(gradle, /versionName = .*"0\.1\.0"/);
});

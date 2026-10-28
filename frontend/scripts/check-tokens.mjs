// Fails when screen code hard-codes colors, font sizes or radii instead of using
// the tokens in src/theme/tokens.css. Run: npm run check:tokens
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const srcDir = fileURLToPath(new URL("../src/", import.meta.url));
const skip = new Set([join(srcDir, "theme", "tokens.css")]);
const rules = [
  [/#[0-9a-fA-F]{3,8}\b/, "hex color"],
  [/\brgba?\(/, "rgb()/rgba() color"],
  [/font-size:\s*[1-9]\d*px/, "literal font-size"],
  [/border-radius:\s*[1-9]\d*px/, "literal border-radius"],
  [/fontSize\s*:\s*[`"']?\d/, "literal fontSize"],
];

const problems = [];
const walk = (dir) => {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) { walk(path); continue; }
    if (!/\.(css|tsx?)$/.test(name) || skip.has(path)) continue;
    readFileSync(path, "utf8").split(/\r?\n/).forEach((line, index) => {
      for (const [pattern, label] of rules) {
        if (pattern.test(line)) problems.push(`${relative(srcDir, path).split(sep).join("/")}:${index + 1}: ${label}: ${line.trim()}`);
      }
    });
  }
};
walk(srcDir);
if (problems.length) {
  console.error(`Hard-coded theme values (use tokens from src/theme/tokens.css):\n${problems.join("\n")}`);
  process.exit(1);
}
console.log("Theme tokens OK");

import fs from "node:fs";
import path from "node:path";

const root = path.resolve(import.meta.dirname, "..");
const htmlPath = path.join(root, "frontend", "dist", "index.html");
let html = fs.readFileSync(htmlPath, "utf8");
const styleMatch = html.match(/<style>\n?([\s\S]*?)\n?<\/style>/);
const scriptMatch = html.match(/<script>\n?([\s\S]*?)\n?<\/script>/);
let css = styleMatch ? styleMatch[1].trim() : fs.readFileSync(path.join(root, "frontend", "dist", "app.css"), "utf8").trim();
let js = scriptMatch ? scriptMatch[1].trim() : fs.readFileSync(path.join(root, "frontend", "dist", "app.js"), "utf8").trim();

const declarations = new Map();
let next = Math.max(0, ...[...css.matchAll(/\.u(\d+)\{/g)].map(match => Number(match[1]))) + 1;
function replaceStaticStyles(source) {
return source.replace(/<[A-Za-z][^<>]*?\sstyle="([^"]*)"[^<>]*?>/g, tag => {
  const match = tag.match(/\sstyle="([^"]*)"/);
  if (!match || match[1].includes("${")) return tag;
  const declaration = match[1].trim().replace(/;?$/, ";");
  let className = declarations.get(declaration);
  if (!className) {
    className = `u${next++}`;
    declarations.set(declaration, className);
  }
  tag = tag.replace(/\sstyle="[^"]*"/, "");
  if (/\sclass="/.test(tag)) return tag.replace(/\sclass="([^"]*)"/, (_, classes) => ` class="${classes} ${className}"`);
  return tag.replace(/(\/?>)$/, ` class="${className}"$1`);
});}

html = replaceStaticStyles(html);
js = replaceStaticStyles(js);

const utilityCSS = [...declarations].map(([declaration, className]) => `.${className}{${declaration}}`).join("\n");
css = `${css}\n\n/* Generated from formerly inline static declarations. */\n${utilityCSS}\n`;
js += "\n";
if (styleMatch) html = html.replace(/<style>\n?[\s\S]*?\n?<\/style>/, '<link rel="stylesheet" href="app.css">');
if (scriptMatch) html = html.replace(/<script>\n?[\s\S]*?\n?<\/script>/, '<script src="app.js"></script>');
fs.writeFileSync(path.join(root, "frontend", "dist", "app.css"), css);
fs.writeFileSync(path.join(root, "frontend", "dist", "app.js"), js);
fs.writeFileSync(htmlPath, html);

import { readFileSync } from "node:fs";

const html = ["../frontend/dist/index.html", "../frontend/dist/app.js"]
  .map((f) => readFileSync(new URL(f, import.meta.url), "utf8"))
  .join("\n");
const forbidden = [
  ['raw mission ID in HTML', 'class="nm">${m.id}</span>'],
  ['raw report mission ID in HTML', '<b>${rm.id}</b>'],
  ['unsanitized Markdown href', "href=\"$2\""],
  ['unsanitized custom capture icon', "+t.emoji+'</span>'"],
];
for (const [name, token] of forbidden) {
  if (html.includes(token)) throw new Error(`${name}: forbidden sink is present`);
}
for (const required of ['safeURI(raw,image=false)', 'safeImageURI(raw)']) {
  if (!html.includes(required)) {
    throw new Error(`missing frontend security guard: ${required}`);
  }
}
console.log('frontend security sink guards: ok');

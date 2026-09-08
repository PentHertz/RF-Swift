import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
const context = vm.createContext({});
vm.runInContext(readFileSync(new URL("../frontend/dist/terminal-keys.js", import.meta.url), "utf8"), context);
const keys = context.RFSwiftTerminalKeys;
const event = {type:"keydown", altKey:true, key:"ArrowLeft"};
assert.equal(keys.wordSequence(event, "MacIntel"), "\x1bb");
assert.equal(keys.wordSequence({...event,key:"ArrowRight"}, "MacIntel"), "\x1bf");
for (const patch of [{type:"keyup"}, {isComposing:true}, {ctrlKey:true}, {metaKey:true}, {shiftKey:true}, {altKey:false}, {key:"b"}, {key:"é"}]) {
  assert.equal(keys.wordSequence({...event,...patch}, "MacIntel"), "");
}
for (const platform of ["Win32", "Linux x86_64"]) assert.equal(keys.wordSequence(event, platform), "");
const app=readFileSync(new URL("../frontend/dist/app.js",import.meta.url),"utf8");
const html=readFileSync(new URL("../frontend/dist/index.html",import.meta.url),"utf8");
assert.ok(app.includes("RFSwiftTerminalKeys.wordSequence(e,navigator.platform)"));
assert.ok(app.includes('macOptionIsMeta:store.get("terminalOptionAsMeta","true")==="true"'));
assert.ok(html.indexOf('src="terminal-keys.js"') < html.indexOf('src="app.js"'));
console.log("terminal keyboard regression tests: ok");

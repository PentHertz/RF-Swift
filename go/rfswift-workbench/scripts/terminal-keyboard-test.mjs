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
// Zoom: Ctrl on Linux/Windows, Cmd on macOS; Ctrl+Shift+- ("_") stays the shell's undo.
const zoom = (patch, platform) => keys.zoomStep({type:"keydown", ...patch}, platform);
for (const platform of ["Linux x86_64", "Win32"]) {
  assert.equal(zoom({ctrlKey:true, key:"="}, platform), "in");
  assert.equal(zoom({ctrlKey:true, shiftKey:true, key:"+"}, platform), "in");
  assert.equal(zoom({ctrlKey:true, key:"-"}, platform), "out");
  assert.equal(zoom({ctrlKey:true, key:"0"}, platform), "reset");
  for (const patch of [{ctrlKey:true, shiftKey:true, key:"_"}, {ctrlKey:true, shiftKey:true, key:"-"}, {key:"="}, {metaKey:true, key:"="}, {ctrlKey:true, altKey:true, key:"="}, {ctrlKey:true, key:"c"}]) {
    assert.equal(zoom(patch, platform), "", JSON.stringify(patch));
  }
  assert.equal(keys.zoomStep({type:"keyup", ctrlKey:true, key:"="}, platform), "");
  assert.equal(zoom({ctrlKey:true, key:"=", isComposing:true}, platform), "");
}
assert.equal(zoom({metaKey:true, key:"="}, "MacIntel"), "in");
assert.equal(zoom({metaKey:true, key:"-"}, "MacIntel"), "out");
assert.equal(zoom({metaKey:true, key:"0"}, "MacIntel"), "reset");
assert.equal(zoom({ctrlKey:true, key:"="}, "MacIntel"), "");
const app=readFileSync(new URL("../frontend/dist/app.js",import.meta.url),"utf8");
const html=readFileSync(new URL("../frontend/dist/index.html",import.meta.url),"utf8");
assert.ok(app.includes("RFSwiftTerminalKeys.wordSequence(e,navigator.platform)"));
assert.ok(app.includes("RFSwiftTerminalKeys.zoomStep(e,navigator.platform)"));
assert.ok(app.includes("fontFamily:TERM_FONT,fontSize:terminalFontSize()"));
assert.ok(app.includes('macOptionIsMeta:store.get("terminalOptionAsMeta","true")==="true"'));
assert.ok(html.indexOf('src="terminal-keys.js"') < html.indexOf('src="app.js"'));
console.log("terminal keyboard regression tests: ok");

// Kept independent of the DOM so keyboard behavior is regression-tested.
globalThis.RFSwiftTerminalKeys = Object.freeze({
  isMac(platform) { return /Mac|iPhone|iPad|iPod/i.test(platform || ""); },
  wordSequence(event, platform) {
    if (!this.isMac(platform) || event.type !== "keydown" || event.isComposing ||
        !event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return "";
    if (event.key === "ArrowLeft") return "\x1bb";
    if (event.key === "ArrowRight") return "\x1bf";
    return "";
  },
  // Zoom shortcuts of desktop terminals: Ctrl + / Ctrl - / Ctrl 0 (Cmd on
  // macOS). Ctrl+Shift+- ("_") is left to the shell, where it is undo.
  zoomStep(event, platform) {
    if (event.type !== "keydown" || event.isComposing || event.altKey) return "";
    const mac = this.isMac(platform);
    if (mac ? !event.metaKey || event.ctrlKey : !event.ctrlKey || event.metaKey) return "";
    if (event.key === "=" || event.key === "+") return "in";
    if (event.key === "-" && !event.shiftKey) return "out";
    if (event.key === "0" && !event.shiftKey) return "reset";
    return "";
  }
});

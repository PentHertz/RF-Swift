// Kept independent of the DOM so keyboard behavior is regression-tested.
globalThis.RFSwiftTerminalKeys = Object.freeze({
  isMac(platform) { return /Mac|iPhone|iPad|iPod/i.test(platform || ""); },
  wordSequence(event, platform) {
    if (!this.isMac(platform) || event.type !== "keydown" || event.isComposing ||
        !event.altKey || event.ctrlKey || event.metaKey || event.shiftKey) return "";
    if (event.key === "ArrowLeft") return "\x1bb";
    if (event.key === "ArrowRight") return "\x1bf";
    return "";
  }
});

(() => {
  const system = matchMedia("(prefers-color-scheme: dark)");
  const root = document.documentElement;
  const head = document.head;
  function link(id, rel) {
    const existing = document.getElementById(id);
    if (existing) return existing;
    const element = document.createElement("link");
    element.id = id;
    element.rel = rel;
    head.append(element);
    return element;
  }
  if (document.getElementById("peergit-color")) return;
  const icon = link("peergit-icon", "icon");
  const apple = link("peergit-apple", "apple-touch-icon");
  const manifest = link("peergit-manifest", "manifest");
  const color = document.createElement("meta");
  color.id = "peergit-color";
  color.name = "theme-color";
  head.append(color);
  function apply() {
    const dark = root.dataset.theme === "dark" || (!root.dataset.theme && system.matches);
    const theme = dark ? "dark" : "light";
    root.dataset.resolvedTheme = theme;
    icon.href = "/icons/favicon-" + theme + ".ico";
    apple.href = "/icons/apple-icon-" + theme + ".png";
    manifest.href = "/manifest-" + theme + ".json";
    color.content = dark ? "#0D111B" : "#F5F7FC";
  }
  function restore() {
    try {
      const value = localStorage.getItem("peergit-theme");
      if (value === "dark" || value === "light") root.dataset.theme = value;
      else delete root.dataset.theme;
    } catch {}
    apply();
  }
  restore();
  system.addEventListener("change", apply);
  window.addEventListener("peergit-appearance", apply);
  window.addEventListener("storage", event => { if (event.key === "peergit-theme" || event.key === null) restore(); });
})();

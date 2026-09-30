// CloudStore console chrome. Bound once on window: Drive morphs #amarra-main,
// so listeners attached to morphed nodes would be orphaned on the next morph.
(function () {
  if (window.__cloudstoreBound) return;
  window.__cloudstoreBound = true;

  function storedView() {
    try {
      return localStorage.getItem("cloudstore-view");
    } catch (e) {
      return null;
    }
  }

  function applyView(view) {
    if (view !== "table" && view !== "grid") return;
    document.querySelectorAll("[data-view-panel]").forEach(function (panel) {
      panel.toggleAttribute("hidden", panel.getAttribute("data-view-panel") !== view);
    });
    document.querySelectorAll("[data-view-toggle]").forEach(function (button) {
      var active = button.getAttribute("data-view-toggle") === view;
      button.classList.toggle("bg-surface-container-highest", active);
      button.classList.toggle("text-primary", active);
      button.classList.toggle("text-on-surface-variant", !active);
    });
  }

  document.addEventListener("click", function (event) {
    var button = event.target.closest("[data-view-toggle]");
    if (!button) return;
    var view = button.getAttribute("data-view-toggle");
    try {
      localStorage.setItem("cloudstore-view", view);
    } catch (e) {}
    applyView(view);
  });

  // ⌘K / Ctrl+K focuses the global search field.
  document.addEventListener("keydown", function (event) {
    if (!(event.metaKey || event.ctrlKey) || event.key.toLowerCase() !== "k") return;
    var input = document.querySelector("[data-cloudstore-search]");
    if (!input) return;
    event.preventDefault();
    input.focus();
    input.select();
  });

  document.addEventListener("amarra:morphed", function () {
    applyView(storedView());
  });

  document.addEventListener("DOMContentLoaded", function () {
    applyView(storedView());
  });
})();

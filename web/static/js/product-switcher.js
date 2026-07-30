(function () {
  "use strict";

  // Behaviour for the header/sidebar product switcher. The panel itself is
  // shown by CSS :focus-within (see ui/dropdown.templ), so "close" here means
  // moving focus out of the switcher.
  //
  // Listeners are delegated from the switcher root because the panel contents
  // arrive later via htmx — binding directly to the filter input or the rows
  // would miss everything loaded after init.

  function visibleRows(root) {
    return Array.prototype.slice
      .call(root.querySelectorAll("[data-switcher-row]"))
      .filter(function (row) {
        return !row.hidden;
      });
  }

  function applyFilter(root, term) {
    var needle = term.trim().toLowerCase();
    var rows = root.querySelectorAll("[data-switcher-row]");
    var shown = 0;

    rows.forEach(function (row) {
      var name = (row.dataset.productName || "").toLowerCase();
      var match = needle === "" || name.indexOf(needle) !== -1;
      row.hidden = !match;
      if (match) shown++;
    });

    var empty = root.querySelector("[data-switcher-empty]");
    if (empty) empty.hidden = shown > 0 || rows.length === 0;
  }

  // Arrow keys walk the visible rows. Focus stays inside the switcher, which is
  // what keeps the panel open.
  function moveFocus(root, from, delta) {
    var rows = visibleRows(root);
    if (rows.length === 0) return;

    var index = rows.indexOf(from);
    var next;
    if (index === -1) {
      next = delta > 0 ? rows[0] : rows[rows.length - 1];
    } else {
      next = rows[(index + delta + rows.length) % rows.length];
    }
    next.focus();
  }

  function initSwitcher(root) {
    if (root.dataset.switcherInit === "1") return;
    root.dataset.switcherInit = "1";

    root.addEventListener("input", function (evt) {
      if (evt.target && evt.target.hasAttribute("data-switcher-filter")) {
        applyFilter(root, evt.target.value);
      }
    });

    root.addEventListener("keydown", function (evt) {
      if (evt.key === "Escape") {
        // Nothing to restore focus to: refocusing the trigger would re-open the
        // panel, since the panel is driven by :focus-within.
        if (document.activeElement) document.activeElement.blur();
        evt.preventDefault();
        return;
      }
      if (evt.key === "ArrowDown") {
        moveFocus(root, evt.target, 1);
        evt.preventDefault();
        return;
      }
      if (evt.key === "ArrowUp") {
        moveFocus(root, evt.target, -1);
        evt.preventDefault();
      }
    });
  }

  function initAll() {
    document.querySelectorAll("[data-product-switcher]").forEach(initSwitcher);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initAll);
  } else {
    initAll();
  }
  document.body.addEventListener("htmx:afterSwap", initAll);
})();

(function () {
  "use strict";

  // Test case selection for a new campaign. Filtering happens client side on
  // purpose: re-rendering the list from the server would drop checkboxes that
  // fall outside the current filter, silently shrinking the campaign scope.

  function initPicker(picker) {
    if (picker.dataset.pickerInit === "1") return;
    picker.dataset.pickerInit = "1";

    var rows = Array.prototype.slice.call(picker.querySelectorAll("[data-picker-row]"));
    var componentFilter = picker.querySelector("#picker-component");
    var modeFilter = picker.querySelector("#picker-mode");
    var selectAll = picker.querySelector("[data-picker-select-all]");
    var clear = picker.querySelector("[data-picker-clear]");
    var count = picker.querySelector("[data-picker-count]");

    function matches(row) {
      var wantComponent = componentFilter ? componentFilter.value : "";
      var wantMode = modeFilter ? modeFilter.value : "";
      var component = row.dataset.component || "";
      var mode = row.dataset.mode || "";

      if (wantComponent === "__none__") {
        if (component !== "") return false;
      } else if (wantComponent !== "" && component !== wantComponent) {
        return false;
      }
      return wantMode === "" || mode === wantMode;
    }

    function updateCount() {
      if (!count) return;
      var n = rows.filter(function (row) {
        var box = row.querySelector("input[type=checkbox]");
        return box && box.checked;
      }).length;
      count.textContent = String(n);
    }

    function applyFilter() {
      rows.forEach(function (row) {
        row.hidden = !matches(row);
      });
    }

    // Only rows the user can currently see are affected, so "select all" means
    // "select all of what I filtered down to".
    function setVisible(checked) {
      rows.forEach(function (row) {
        if (row.hidden) return;
        var box = row.querySelector("input[type=checkbox]");
        if (box) box.checked = checked;
      });
      updateCount();
    }

    if (componentFilter) componentFilter.addEventListener("change", applyFilter);
    if (modeFilter) modeFilter.addEventListener("change", applyFilter);
    if (selectAll) {
      selectAll.addEventListener("click", function () {
        setVisible(true);
      });
    }
    if (clear) {
      clear.addEventListener("click", function () {
        setVisible(false);
      });
    }
    picker.addEventListener("change", function (evt) {
      if (evt.target && evt.target.type === "checkbox") updateCount();
    });

    applyFilter();
    updateCount();
  }

  function initAll() {
    document.querySelectorAll("[data-campaign-picker]").forEach(initPicker);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initAll);
  } else {
    initAll();
  }
  document.body.addEventListener("htmx:afterSwap", initAll);
})();

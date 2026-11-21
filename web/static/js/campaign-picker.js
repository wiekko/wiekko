// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

(function () {
  "use strict";

  // Test case selection for a new campaign. Filtering happens client side on
  // purpose: re-rendering the list from the server would drop checkboxes that
  // fall outside the current filter, silently shrinking the campaign scope.

  function initPicker(picker) {
    if (picker.dataset.pickerInit === "1") return;
    picker.dataset.pickerInit = "1";

    var rows = Array.prototype.slice.call(picker.querySelectorAll("[data-picker-row]"));
    var search = picker.querySelector("#picker-search");
    var componentFilter = picker.querySelector("#picker-component");
    var capabilityFilter = picker.querySelector("#picker-capability");
    var pyramidFilter = picker.querySelector("#picker-pyramid");
    var modeFilter = picker.querySelector("#picker-mode");
    var selectAll = picker.querySelector("[data-picker-select-all]");
    var clear = picker.querySelector("[data-picker-clear]");
    var count = picker.querySelector("[data-picker-count]");
    var shown = picker.querySelector("[data-picker-shown]");
    var emptyNote = picker.querySelector("[data-picker-empty]");

    // Rows currently passing the filter, in document order. Rebuilt by
    // applyFilter so range selection never has to consult hidden rows.
    var visible = rows.slice();
    // The row a shift-click measures its range from. Held as an element rather
    // than an index, because indices shift whenever the filter changes.
    var anchor = null;
    // Shift state captured at mousedown. Clicking a row's label makes the
    // browser forward a synthetic click to the checkbox; engines do copy the
    // modifier flags onto it, but nothing in the spec requires them to, and a
    // shift-click that silently degrades to a plain click would be the worst
    // failure here. Captured in a handler we need for preventDefault anyway.
    var shiftDown = false;

    function box(row) {
      return row.querySelector("input[type=checkbox]");
    }

    // An empty select means "no constraint". "__none__" means the opposite of a
    // named value: rows with nothing linked at all.
    function matchesID(want, actual) {
      if (want === "") return true;
      if (want === "__none__") return actual === "";
      return actual === want;
    }

    function matches(row) {
      var term = search ? search.value.trim().toLowerCase() : "";
      if (term !== "" && (row.dataset.title || "").indexOf(term) === -1) return false;
      if (!matchesID(componentFilter ? componentFilter.value : "", row.dataset.component || "")) return false;
      if (!matchesID(capabilityFilter ? capabilityFilter.value : "", row.dataset.capability || "")) return false;
      if (!matchesID(pyramidFilter ? pyramidFilter.value : "", row.dataset.pyramid || "")) return false;
      var wantMode = modeFilter ? modeFilter.value : "";
      return wantMode === "" || (row.dataset.mode || "") === wantMode;
    }

    // Counts every checked row, not just the visible ones: selections made under
    // a previous filter are still part of the scope and will still be submitted.
    function updateCount() {
      if (!count) return;
      var n = 0;
      for (var i = 0; i < rows.length; i++) {
        var b = box(rows[i]);
        if (b && b.checked) n++;
      }
      count.textContent = String(n);
    }

    function applyFilter() {
      visible = [];
      for (var i = 0; i < rows.length; i++) {
        var ok = matches(rows[i]);
        rows[i].hidden = !ok;
        if (ok) visible.push(rows[i]);
      }
      if (shown) shown.textContent = String(visible.length);
      if (emptyNote) emptyNote.hidden = visible.length !== 0;
    }

    // Only rows the user can currently see are affected, so the buttons mean
    // "everything I filtered down to" — which is what their labels say.
    function setVisible(checked) {
      for (var i = 0; i < visible.length; i++) {
        var b = box(visible[i]);
        if (b) b.checked = checked;
      }
      updateCount();
    }

    // Applies the clicked row's new state across the visible span between the
    // anchor and the click. Hidden rows are skipped entirely: pulling unseen
    // test cases into the campaign is the one thing this must never do.
    function applyRange(from, to, checked) {
      var lo = Math.min(from, to);
      var hi = Math.max(from, to);
      for (var i = lo; i <= hi; i++) {
        var b = box(visible[i]);
        if (b) b.checked = checked;
      }
    }

    if (search) {
      search.addEventListener("input", applyFilter);
      // The picker lives inside the campaign form, where Enter would submit and
      // create the campaign mid-selection.
      search.addEventListener("keydown", function (evt) {
        if (evt.key === "Enter") evt.preventDefault();
      });
    }
    [componentFilter, capabilityFilter, pyramidFilter, modeFilter].forEach(function (sel) {
      if (sel) sel.addEventListener("change", applyFilter);
    });
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

    // Shift-click otherwise paints a text selection across the rows. Cancelling
    // the mousedown default suppresses that without stopping the click that
    // follows, so the checkbox still toggles. Scoped to rows so the search box
    // keeps normal caret placement.
    picker.addEventListener("mousedown", function (evt) {
      if (!evt.target.closest || !evt.target.closest("[data-picker-row]")) return;
      shiftDown = evt.shiftKey;
      if (evt.shiftKey) evt.preventDefault();
    });

    // The captured flag can only go stale after an aborted press — mousedown on
    // a row, mouse released elsewhere — followed by a keyboard toggle. Shift
    // itself is excluded so holding it never clears the flag mid-click.
    picker.addEventListener("keydown", function (evt) {
      if (evt.key !== "Shift") shiftDown = false;
    });

    // Each row is wrapped in a label, so clicking one fires a click on the label
    // *and* a synthetic click on the checkbox, both of which bubble to here.
    // Acting only on the checkbox target collapses that back to one event per
    // row click, whether the user hit the box or the surrounding label.
    picker.addEventListener("click", function (evt) {
      var target = evt.target;
      if (!target || target.type !== "checkbox") return;
      var row = target.closest("[data-picker-row]");
      if (!row) return;

      var shift = evt.shiftKey || shiftDown;
      shiftDown = false;

      var index = visible.indexOf(row);
      // An anchor that has since been filtered away can't define a range the
      // user can see, so the click just re-anchors instead.
      var start = anchor ? visible.indexOf(anchor) : -1;
      if (shift && start !== -1 && index !== -1) {
        // Never preventDefault here: cancelling a checkbox click reverts the
        // toggle. The clicked box already holds the intended state, so the
        // range just copies it outward.
        applyRange(start, index, target.checked);
        // The shift-mousedown above suppressed focus along with the selection.
        target.focus({ preventScroll: true });
      } else {
        anchor = row;
      }
      updateCount();
    });

    // Backstop for any toggle that doesn't arrive as a click on the checkbox.
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

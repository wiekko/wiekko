// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

(function () {
  "use strict";

  // Row selection for bulk operations (UC-76, FR-130, FR-131).
  //
  // A table offering bulk operations renders a selection bar
  // ([data-bulk-bar="<region id>"], hidden) next to its search and filters,
  // and a "Select" button ([data-bulk-toggle="<region id>"]) in the page
  // header. The region is the element the table is swapped into.
  //
  // In selection mode a click on a row selects it instead of following the
  // title link, Shift+click selects a range, and the filter row gives way to
  // the selection bar. Ctrl/Cmd+click on a row (not on its link, which keeps
  // opening a new tab) selects it at any time. The bar's operations ask the
  // server for a confirmation, which shows what will change; nothing is
  // changed from here.

  var anchors = {}; // region id -> row a Shift+click measures from
  var matching = {}; // region id -> "all matching" chosen

  function barOf(region) {
    return region.querySelector('[data-bulk-bar="' + region.id + '"]');
  }

  function controlsOf(region) {
    return region.querySelector('[id$="-controls"]');
  }

  function rowsOf(region) {
    return Array.prototype.slice.call(region.querySelectorAll("tr[data-bulk-id]"));
  }

  function selectedOf(region) {
    return rowsOf(region).filter(function (r) {
      return r.getAttribute("aria-selected") === "true";
    });
  }

  function selecting(region) {
    return region.hasAttribute("data-selecting");
  }

  // regionOf finds the table region an element belongs to: the nearest
  // ancestor that a selection bar names.
  function regionOf(el) {
    for (var n = el.parentElement; n; n = n.parentElement) {
      if (n.id && n.querySelector('[data-bulk-bar="' + n.id + '"]')) return n;
    }
    return null;
  }

  function setRow(row, on) {
    row.setAttribute("aria-selected", on ? "true" : "false");
  }

  // show puts the region's current content into selection mode: after
  // entering it, and again after each swap of the table while in it.
  function show(region) {
    var bar = barOf(region);
    if (!bar) return;
    bar.hidden = false;
    var controls = controlsOf(region);
    if (controls) controls.hidden = true;
    var table = region.querySelector("table");
    if (table) {
      table.setAttribute("role", "grid");
      table.setAttribute("aria-multiselectable", "true");
    }
    rowsOf(region).forEach(function (r) {
      if (!r.hasAttribute("aria-selected")) setRow(r, false);
      r.tabIndex = 0;
    });
    update(region);
  }

  function enter(region) {
    if (selecting(region) || !barOf(region)) return;
    region.setAttribute("data-selecting", "");
    var toggle = document.querySelector('[data-bulk-toggle="' + region.id + '"]');
    if (toggle) toggle.setAttribute("aria-pressed", "true");
    show(region);
  }

  function leave(region) {
    if (!selecting(region)) return;
    region.removeAttribute("data-selecting");
    delete anchors[region.id];
    delete matching[region.id];
    var bar = barOf(region);
    if (bar) bar.hidden = true;
    var controls = controlsOf(region);
    if (controls) controls.hidden = false;
    var table = region.querySelector("table");
    if (table) {
      table.removeAttribute("role");
      table.removeAttribute("aria-multiselectable");
    }
    rowsOf(region).forEach(function (r) {
      r.removeAttribute("aria-selected");
      r.removeAttribute("tabindex");
    });
    var toggle = document.querySelector('[data-bulk-toggle="' + region.id + '"]');
    if (toggle) {
      toggle.setAttribute("aria-pressed", "false");
      if (region.contains(document.activeElement) || document.activeElement === document.body) toggle.focus();
    }
  }

  function update(region) {
    var bar = barOf(region);
    if (!bar) return;
    var rows = rowsOf(region);
    var picked = selectedOf(region);
    var total = parseInt(bar.getAttribute("data-total"), 10) || rows.length;
    if (matching[region.id] && picked.length !== rows.length) delete matching[region.id];
    var all = matching[region.id];
    var n = all ? total : picked.length;

    bar.querySelector("[data-bulk-count]").textContent = all ? "All " + total + " matching selected" : n + " selected";
    var page = bar.querySelector("[data-bulk-page]");
    var more = bar.querySelector("[data-bulk-matching]");
    var clear = bar.querySelector("[data-bulk-clear]");
    if (page) page.hidden = rows.length === 0 || picked.length === rows.length;
    if (more) more.hidden = all || picked.length !== rows.length || total <= rows.length;
    if (clear) clear.hidden = n === 0;
    bar.querySelectorAll("[data-bulk-action], [data-bulk-menu]").forEach(function (b) {
      if (b.getAttribute("role") !== "menuitem") b.disabled = n === 0;
    });
  }

  function toggleRow(region, row, range) {
    var anchor = anchors[region.id];
    if (range && anchor && region.contains(anchor) && anchor !== row) {
      var rows = rowsOf(region);
      var a = rows.indexOf(anchor);
      var b = rows.indexOf(row);
      var on = anchor.getAttribute("aria-selected") === "true";
      rows.slice(Math.min(a, b), Math.max(a, b) + 1).forEach(function (r) {
        setRow(r, on);
      });
    } else {
      setRow(row, row.getAttribute("aria-selected") !== "true");
      anchors[region.id] = row;
    }
    update(region);
  }

  function preview(region, action) {
    var bar = barOf(region);
    var values = { action: action, state: window.location.search.replace(/^\?/, "") };
    if (matching[region.id]) {
      values.scope = "matching";
    } else {
      values.ids = selectedOf(region)
        .map(function (r) {
          return r.getAttribute("data-bulk-id");
        })
        .join(",");
    }
    window.htmx.ajax("POST", bar.getAttribute("data-preview-url"), {
      source: bar,
      target: "#modal",
      swap: "innerHTML",
      values: values,
    });
  }

  document.addEventListener("click", function (evt) {
    var t = evt.target;
    if (!(t instanceof Element)) return;

    var toggle = t.closest("[data-bulk-toggle]");
    if (toggle) {
      var r = document.getElementById(toggle.getAttribute("data-bulk-toggle"));
      if (r) (selecting(r) ? leave : enter)(r);
      return;
    }

    var bar = t.closest("[data-bulk-bar]");
    if (bar) {
      var region = document.getElementById(bar.getAttribute("data-bulk-bar"));
      if (!region) return;
      if (t.closest("[data-bulk-exit]")) {
        leave(region);
      } else if (t.closest("[data-bulk-page]")) {
        rowsOf(region).forEach(function (r) {
          setRow(r, true);
        });
        update(region);
      } else if (t.closest("[data-bulk-matching]")) {
        matching[region.id] = true;
        update(region);
      } else if (t.closest("[data-bulk-clear]")) {
        delete matching[region.id];
        rowsOf(region).forEach(function (r) {
          setRow(r, false);
        });
        update(region);
      } else {
        var action = t.closest("[data-bulk-action]");
        if (action && !action.disabled) {
          if (document.activeElement) document.activeElement.blur(); // closes the menu
          preview(region, action.getAttribute("data-bulk-action"));
        }
      }
      return;
    }

    var row = t.closest("tr[data-bulk-id]");
    if (!row) return;
    var reg = regionOf(row);
    if (!reg) return;
    var link = t.closest("a[href]");
    var control = t.closest("button, input, select, textarea, label, [role=menu], [role=menuitem]");

    if (!selecting(reg)) {
      // Ctrl/Cmd+click on the link keeps opening it in a new tab.
      if ((evt.ctrlKey || evt.metaKey) && !link && !control) {
        evt.preventDefault();
        enter(reg);
        toggleRow(reg, row, false);
      }
      return;
    }
    if (control && !link) return; // the row's own actions stay usable
    evt.preventDefault();
    toggleRow(reg, row, evt.shiftKey);
  });

  // Shift+click would also extend the text selection across the rows.
  document.addEventListener("mousedown", function (evt) {
    if (evt.shiftKey && evt.target instanceof Element) {
      var row = evt.target.closest("[data-selecting] tr[data-bulk-id]");
      if (row) evt.preventDefault();
    }
  });

  // dialogOpen reports whether a modal is showing. modal.js closes one by
  // emptying it, so an empty [aria-modal] element is a closed dialog.
  function dialogOpen() {
    return Array.prototype.some.call(document.querySelectorAll('[aria-modal="true"]'), function (d) {
      return d.childElementCount > 0;
    });
  }

  document.addEventListener("keydown", function (evt) {
    // The dialog has the keys; an Escape that just closed one (modal.js
    // handles it first) is not also a request to leave selection mode.
    if (evt.defaultPrevented || dialogOpen()) return;
    var row = document.activeElement && document.activeElement.closest && document.activeElement.closest("tr[data-bulk-id]");
    var region = row ? regionOf(row) : null;
    if (region && selecting(region) && document.activeElement === row) {
      if (evt.key === " " || evt.key === "x") {
        evt.preventDefault();
        toggleRow(region, row, evt.shiftKey);
        return;
      }
      if (evt.key === "ArrowDown" || evt.key === "ArrowUp") {
        var rows = rowsOf(region);
        var next = rows[rows.indexOf(row) + (evt.key === "ArrowDown" ? 1 : -1)];
        if (next) {
          evt.preventDefault();
          next.focus();
        }
        return;
      }
    }
    if (evt.key === "Escape") {
      var active = document.querySelector("[data-selecting]");
      if (active) {
        evt.preventDefault();
        leave(active);
      }
    }
  });

  // Sorting, filtering or paging swaps the table: stay in selection mode on
  // the new rows, with nothing selected.
  document.addEventListener("htmx:afterSettle", function (evt) {
    var region = evt.detail && evt.detail.target;
    if (region instanceof Element && selecting(region) && barOf(region)) {
      delete anchors[region.id];
      delete matching[region.id];
      show(region);
    }
  });

  // An applied operation re-renders the table; the selection is done with.
  document.body.addEventListener("bulkApplied", function () {
    document.querySelectorAll("[data-selecting]").forEach(leave);
  });
})();

(function () {
  "use strict";

  function initColumn(col) {
    if (col.dataset.sortableInit === "1") return;
    col.dataset.sortableInit = "1";
    new Sortable(col, {
      group: col.dataset.group,
      // Cards without a move URL are read-only for this viewer (for example
      // not activated for work, FR-116), so they cannot be picked up.
      draggable: "[data-move-url]",
      animation: 150,
      ghostClass: "opacity-40",
      dragClass: "shadow-lg",
      onEnd: function (evt) {
        var card = evt.item;
        var from = evt.from;
        var to = evt.to;
        if (from === to) return;

        var entity = card.dataset.entityType;
        var url = card.dataset.moveUrl;
        // The drop target names the field it moves and the value per entity
        // type, so status boards and the sprint planning lists share this code.
        var field = to.dataset.moveField || "status_id";
        var value = entity === "defect" ? to.dataset.defMoveValue : to.dataset.reqMoveValue;
        // An empty value is meaningful (it clears the sprint), so only a
        // missing attribute counts as "this column can't take the card".
        if (!url || value === undefined) {
          window.location.reload();
          return;
        }

        // The status endpoints name the entity type in their URL, but the
        // sprint endpoint is one URL for all three types, so send it along.
        var values = { entity_type: entity };
        values[field] = value;

        card.classList.add("opacity-60");
        htmx.ajax("PATCH", url, {
          values: values,
          swap: "none",
          source: card,
        }).then(function () {
          card.classList.remove("opacity-60");
          card.dataset[field === "status_id" ? "statusId" : "sprintId"] = value;
        }).catch(function () {
          window.location.reload();
        });
      },
    });
  }

  function initBoard(root) {
    root.querySelectorAll("[data-board-column]").forEach(initColumn);
  }

  function boot() {
    if (typeof Sortable === "undefined") return;
    document.querySelectorAll("[data-board]").forEach(initBoard);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", boot);
  } else {
    boot();
  }

  // Board surfaces also arrive via htmx (the sprint detail loads its tabs
  // lazily), so re-run after every swap. initColumn is idempotent.
  document.body.addEventListener("htmx:afterSwap", boot);
})();

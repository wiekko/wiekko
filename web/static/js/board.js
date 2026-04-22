(function () {
  "use strict";

  function initColumn(col) {
    if (col.dataset.sortableInit === "1") return;
    col.dataset.sortableInit = "1";
    new Sortable(col, {
      group: col.dataset.group,
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
        var statusID = entity === "defect" ? to.dataset.defStatusId : to.dataset.reqStatusId;
        if (!url || !statusID) {
          window.location.reload();
          return;
        }

        card.classList.add("opacity-60");
        htmx.ajax("PATCH", url, {
          values: { status_id: statusID },
          swap: "none",
          source: card,
        }).then(function () {
          card.classList.remove("opacity-60");
          card.dataset.statusId = statusID;
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
})();

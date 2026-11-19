(function () {
  "use strict";

  // The Capabilities field of the New Requirement form (FR-156). htmx loads
  // the dropdown and the parent's suggestion from the server; this script
  // opens and closes the dropdown, moves through it with the keyboard and
  // turns a picked option into a chip. Chips carry hidden capability_ids
  // inputs, so the form posts them like any other field.

  function initField(field) {
    if (field.dataset.fieldInit === "1") return;
    field.dataset.fieldInit = "1";

    var input = field.querySelector("#capability-search");
    var list = field.querySelector("#capability-options");
    var chips = field.querySelector("#capability-chips");
    var template = field.querySelector("#capability-chip-template");
    if (!input || !list || !chips || !template) return;
    var active = -1;

    function options() {
      return Array.prototype.slice.call(list.querySelectorAll("[data-capability-option]"));
    }

    function setActive(i) {
      var opts = options();
      active = i;
      opts.forEach(function (opt, j) {
        var on = j === i;
        opt.setAttribute("aria-selected", on ? "true" : "false");
        opt.classList.toggle("bg-indigo-50", on);
        opt.classList.toggle("dark:bg-gray-700", on);
        if (on) opt.scrollIntoView({ block: "nearest" });
      });
      if (i >= 0 && opts[i]) {
        input.setAttribute("aria-activedescendant", opts[i].id);
      } else {
        input.removeAttribute("aria-activedescendant");
      }
    }

    function open() {
      list.hidden = false;
      input.setAttribute("aria-expanded", "true");
      // With a search term, Enter takes the best match right away.
      setActive(input.value.trim() !== "" && options().length > 0 ? 0 : -1);
    }

    function close() {
      list.hidden = true;
      input.setAttribute("aria-expanded", "false");
      setActive(-1);
    }

    // Once the user changes the chips they are their own choice: a later
    // parent change no longer replaces them, and the hint goes back to normal.
    function claim() {
      var flag = chips.querySelector("[name=capability_suggested]");
      if (flag) flag.remove();
      var hint = field.querySelector("#capability-hint");
      if (hint && hint.dataset.default) hint.textContent = hint.dataset.default;
    }

    function refresh() {
      if (window.htmx) window.htmx.trigger(input, "capability-refresh");
    }

    function add(opt) {
      var chip = template.content.firstElementChild.cloneNode(true);
      chip.querySelector("input[name=capability_ids]").value = opt.dataset.capabilityId;
      var name = chip.querySelector("[data-chip-name]");
      name.textContent = opt.dataset.name;
      name.title = opt.dataset.component || "";
      chip.querySelector("[data-capability-remove]").setAttribute("aria-label", "Remove " + opt.dataset.name);
      chips.appendChild(chip);
      claim();
      input.value = "";
      input.focus();
      refresh();
    }

    function removeChip(chip) {
      chip.remove();
      claim();
      input.focus();
      refresh();
    }

    // A fresh list from the server: show it while the user is in the field.
    list.addEventListener("htmx:afterSwap", function () {
      if (document.activeElement === input) open();
    });

    input.addEventListener("keydown", function (evt) {
      var opts = options();
      switch (evt.key) {
        case "ArrowDown":
          evt.preventDefault();
          if (list.hidden) {
            open();
          }
          if (opts.length) setActive(Math.min(active + 1, opts.length - 1));
          break;
        case "ArrowUp":
          evt.preventDefault();
          if (opts.length) setActive(Math.max(active - 1, 0));
          break;
        case "Enter":
          // Enter picks a capability; it must never submit the form from here.
          evt.preventDefault();
          if (!list.hidden && active >= 0 && opts[active]) add(opts[active]);
          break;
        case "Escape":
          if (!list.hidden) {
            evt.preventDefault();
            close();
          }
          break;
        case "Backspace":
          if (input.value === "") {
            var all = chips.querySelectorAll("[data-capability-chip]");
            if (all.length) removeChip(all[all.length - 1]);
          }
          break;
      }
    });

    // Keep the focus in the search box while an option is clicked or tapped.
    list.addEventListener("mousedown", function (evt) {
      evt.preventDefault();
    });
    list.addEventListener("click", function (evt) {
      var opt = evt.target.closest("[data-capability-option]");
      if (opt) add(opt);
    });

    chips.parentElement.addEventListener("click", function (evt) {
      var remove = evt.target.closest("[data-capability-remove]");
      if (remove) {
        removeChip(remove.closest("[data-capability-chip]"));
        return;
      }
      // A click on the empty part of the box goes to the search.
      if (evt.target === chips.parentElement) input.focus();
    });

    // Leaving the field closes the dropdown.
    field.addEventListener("focusout", function (evt) {
      if (!evt.relatedTarget || !field.contains(evt.relatedTarget)) close();
    });
  }

  function initAll() {
    document.querySelectorAll("[data-capability-field]").forEach(initField);
  }

  if (document.readyState === "loading") {
    document.addEventListener("DOMContentLoaded", initAll);
  } else {
    initAll();
  }
  document.body.addEventListener("htmx:afterSwap", initAll);
})();

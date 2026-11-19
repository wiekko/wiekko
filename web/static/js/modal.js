// Keyboard, touch and confirmation behaviour of the application modal
// (ui.Modal, UR-154).
//
// - Requests from elements with hx-confirm (ui.Confirm) ask in the
//   application's confirmation dialog instead of the browser's confirm().
// - Focus moves into an opened modal, Tab cycles inside it, Escape closes
//   it, and focus returns to where it was when the modal closes.
// - In a confirmation dialog (one with a [data-confirm-action] button),
//   Enter confirms and a tap or click outside the dialog cancels.
(function () {
  "use strict";

  var FOCUSABLE =
    'a[href], button:not([disabled]), input:not([disabled]):not([type="hidden"]), ' +
    'select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

  // Open modals, innermost last: { dialog, returnFocus, onCancel }.
  var stack = [];

  function isOpen(dialog) {
    return dialog.isConnected && dialog.childElementCount > 0;
  }

  function top() {
    return stack.length ? stack[stack.length - 1] : null;
  }

  function focusables(dialog) {
    return Array.prototype.filter.call(dialog.querySelectorAll(FOCUSABLE), function (el) {
      return el.offsetParent !== null || el === document.activeElement;
    });
  }

  function initialFocus(dialog) {
    var target =
      dialog.querySelector("[autofocus]") ||
      dialog.querySelector("[data-confirm-action]") ||
      focusables(dialog).filter(function (el) {
        return !el.hasAttribute("data-modal-close");
      })[0] ||
      focusables(dialog)[0];
    if (target) target.focus();
  }

  // activate registers a dialog that has just appeared.
  function activate(dialog, onCancel) {
    for (var i = 0; i < stack.length; i++) {
      if (stack[i].dialog === dialog) return;
    }
    var active = document.activeElement;
    stack.push({
      dialog: dialog,
      returnFocus: active && active !== document.body ? active : null,
      onCancel: onCancel || null,
    });
    // htmx:load fires before htmx has finished its swap, which still moves
    // focus afterwards; take it once the swap is done.
    window.setTimeout(function () {
      if (isOpen(dialog) && !dialog.contains(document.activeElement)) initialFocus(dialog);
    }, 0);
  }

  // prune drops dialogs that were closed or swapped away (a cancel button, a
  // confirm button's hx-swap="delete", a form that succeeded) and returns
  // focus to the element that opened them.
  function prune() {
    var restore = null;
    while (stack.length && !isOpen(top().dialog)) {
      restore = stack.pop().returnFocus;
    }
    if (restore && restore.isConnected && !(top() && top().dialog.contains(document.activeElement))) {
      restore.focus();
    }
  }

  // close empties a modal the way its cancel button does.
  function close(entry, cancelled) {
    if (cancelled && entry.onCancel) entry.onCancel();
    var id = entry.dialog.id;
    var el = id ? document.getElementById(id) : entry.dialog;
    if (el) el.innerHTML = "";
    prune();
  }

  function entryFor(dialog) {
    for (var i = stack.length - 1; i >= 0; i--) {
      if (stack[i].dialog === dialog) return stack[i];
    }
    return null;
  }

  // --- Opening ---

  function dialogsIn(root) {
    if (!root || root.nodeType !== 1) return [];
    var found = Array.prototype.slice.call(root.querySelectorAll('[aria-modal="true"]'));
    var host = root.closest('[aria-modal="true"]');
    if (host) found.unshift(host);
    return found;
  }

  document.addEventListener("htmx:load", function (evt) {
    dialogsIn(evt.detail.elt).forEach(function (dialog) {
      if (isOpen(dialog)) activate(dialog);
    });
  });

  new MutationObserver(prune).observe(document.documentElement, { childList: true, subtree: true });

  // --- hx-confirm ---

  function resetValue(elt) {
    // A cancelled change to a field (e.g. a prefix rename) leaves the field
    // showing the value it had.
    if ((elt.tagName === "INPUT" || elt.tagName === "TEXTAREA") && "defaultValue" in elt) {
      elt.value = elt.defaultValue;
    }
  }

  document.addEventListener("htmx:confirm", function (evt) {
    var question = evt.detail.question;
    var tpl = document.getElementById("confirm-dialog-template");
    var container = document.getElementById("modal");
    if (!question || !tpl || !tpl.content || !container) return;
    evt.preventDefault();

    var elt = evt.detail.elt;
    var variant = elt.getAttribute("data-confirm-variant") === "danger" ? "danger" : "neutral";

    container.innerHTML = "";
    container.appendChild(tpl.content.cloneNode(true));
    var dialog = container.querySelector('[aria-modal="true"]');
    var title = dialog.querySelector("#" + dialog.id + "-title");
    var message = dialog.querySelector("[data-confirm-message]");
    var action = dialog.querySelector("[data-confirm-action]");

    title.textContent = elt.getAttribute("data-confirm-title") || "Please confirm";
    message.textContent = question;
    action.textContent = elt.getAttribute("data-confirm-label") || "Confirm";
    action.className = action.getAttribute("data-class-" + variant);

    action.addEventListener("click", function () {
      var entry = entryFor(dialog);
      if (entry) close(entry, false);
      evt.detail.issueRequest(true);
    });

    activate(dialog, function () {
      resetValue(elt);
    });
  });

  // --- Closing, keyboard and touch ---

  document.addEventListener("click", function (evt) {
    var closer = evt.target.closest("[data-modal-close]");
    if (closer) {
      var target = document.getElementById(closer.getAttribute("data-modal-close"));
      var dialog = closer.closest('[aria-modal="true"]');
      var entry = dialog && entryFor(dialog);
      if (entry) {
        close(entry, true);
      } else if (target) {
        target.innerHTML = "";
      }
      return;
    }

    // A tap or click beside a confirmation dialog cancels it. Form modals
    // stay open, so that a stray tap does not discard what was typed.
    var current = top();
    if (
      current &&
      current.dialog.contains(evt.target) &&
      !evt.target.closest("[data-modal-panel]") &&
      current.dialog.querySelector("[data-confirm-action]")
    ) {
      close(current, true);
    }
  });

  document.addEventListener("keydown", function (evt) {
    var current = top();
    if (!current) return;
    var dialog = current.dialog;

    if (evt.key === "Escape") {
      evt.preventDefault();
      close(current, true);
      return;
    }

    if (evt.key === "Tab") {
      var items = focusables(dialog);
      if (!items.length) {
        evt.preventDefault();
        return;
      }
      var first = items[0];
      var last = items[items.length - 1];
      var inside = dialog.contains(document.activeElement);
      if (evt.shiftKey && (!inside || document.activeElement === first)) {
        evt.preventDefault();
        last.focus();
      } else if (!evt.shiftKey && (!inside || document.activeElement === last)) {
        evt.preventDefault();
        first.focus();
      }
      return;
    }

    if (evt.key === "Enter" && !evt.isComposing) {
      var action = dialog.querySelector("[data-confirm-action]");
      var tag = evt.target.tagName;
      // Buttons and links act on Enter themselves (Cancel stays Cancel).
      if (action && tag !== "BUTTON" && tag !== "A" && tag !== "TEXTAREA") {
        evt.preventDefault();
        action.click();
      }
    }
  });
})();

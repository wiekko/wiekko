// Surfaces failed htmx requests to the user.
//
// htmx's default responseHandling treats 4xx/5xx as "error, do not swap", which
// means a failing request is otherwise completely invisible: the modal never
// opens, the row never disappears, and nothing explains why.
(function () {
  "use strict";

  var TOAST_LIFETIME_MS = 4000;

  // Statuses whose response bodies are internal shorthand ("not found",
  // "forbidden") rather than something worth showing a user.
  var STATUS_MESSAGES = {
    401: "Your session has expired. Please sign in again.",
    403: "You are not allowed to do that.",
    404: "That item could not be found. Try reloading the page.",
    405: "That action is not available here.",
    413: "That file is too large to upload.",
    429: "Too many requests. Please wait a moment and try again.",
  };

  function messageFor(xhr) {
    var mapped = STATUS_MESSAGES[xhr.status];
    if (mapped) return mapped;

    // Handlers report actionable failures (e.g. 422 from a delete that would
    // break a constraint) as a plain-text body. Anything else is noise.
    var contentType = xhr.getResponseHeader("Content-Type") || "";
    var body = (xhr.responseText || "").trim();
    if (body && body.length <= 200 && contentType.indexOf("text/plain") === 0) {
      return body.charAt(0).toUpperCase() + body.slice(1);
    }

    if (xhr.status >= 500) {
      return "The server ran into a problem. Please try again.";
    }
    return "Something went wrong (error " + xhr.status + "). Please try again.";
  }

  function showError(message) {
    var container = document.getElementById("toast-container");
    var tpl = document.getElementById("toast-error-template");
    if (!container || !tpl || !tpl.content) {
      window.console && console.error(message);
      return;
    }

    var toast = tpl.content.firstElementChild.cloneNode(true);
    var text = toast.querySelector("p");
    if (text) text.textContent = message;

    container.prepend(toast);
    window.setTimeout(function () {
      toast.remove();
    }, TOAST_LIFETIME_MS);
  }

  document.addEventListener("htmx:responseError", function (evt) {
    showError(messageFor(evt.detail.xhr));
  });

  document.addEventListener("htmx:sendError", function () {
    showError("Could not reach the server. Check your connection and try again.");
  });

  document.addEventListener("htmx:timeout", function () {
    showError("The server took too long to respond. Please try again.");
  });
})();

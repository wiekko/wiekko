// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Version switcher and "unreleased" banner of the documentation site.
//
// The site serves every version under its own top-level path (/latest/,
// /stable/, /v1.2.0/, ...) and lists them in /versions.json, which build.sh
// writes. Without that file, as under `mdbook serve`, this script does nothing.
(function () {
  "use strict";

  var parts = window.location.pathname.split("/");
  var current = parts[1] || "";
  var rest = parts.slice(2).join("/"); // page path within the version

  function el(tag, attrs, text) {
    var e = document.createElement(tag);
    Object.keys(attrs || {}).forEach(function (k) { e.setAttribute(k, attrs[k]); });
    if (text) e.textContent = text;
    return e;
  }

  function label(id, info) {
    if (id === "latest") return "main (unreleased)";
    if (id === "stable") return "stable (" + info.stable + ")";
    return id;
  }

  // Go to the same page in another version, or to that version's start page
  // if the page does not exist there.
  function switchTo(id) {
    var target = "/" + id + "/" + rest;
    fetch(target, { method: "HEAD" })
      .then(function (r) { window.location.href = r.ok ? target : "/" + id + "/"; })
      .catch(function () { window.location.href = "/" + id + "/"; });
  }

  function addSwitcher(info, ids) {
    var bar = document.querySelector("#mdbook-menu-bar .left-buttons");
    if (!bar) return;
    var select = el("select", { "class": "version-switcher", "aria-label": "Documentation version" });
    ids.forEach(function (id) {
      var opt = el("option", { value: id }, label(id, info));
      if (id === current) opt.selected = true;
      select.appendChild(opt);
    });
    select.addEventListener("change", function () { switchTo(select.value); });
    bar.appendChild(select);
  }

  function addBanner(info) {
    var main = document.querySelector("#mdbook-content main");
    if (!main) return;
    var banner = el("div", { "class": "version-banner", role: "note" });
    banner.appendChild(document.createTextNode(
      "This is the documentation of the development version (main) and may describe features that are not released yet."));
    if (info.stable) {
      banner.appendChild(document.createTextNode(" "));
      var link = el("a", { href: "/stable/" + rest }, "Read the documentation of " + info.stable + ".");
      banner.appendChild(link);
    }
    main.insertBefore(banner, main.firstChild);
  }

  fetch("/versions.json", { cache: "no-cache" })
    .then(function (r) { return r.ok ? r.json() : null; })
    .then(function (info) {
      if (!info) return;
      var ids = ["latest"].concat(info.stable ? ["stable"] : [], info.versions || []);
      if (ids.indexOf(current) < 0) return;
      addSwitcher(info, ids);
      if (current === "latest") addBanner(info);
    })
    .catch(function () { /* no version information: leave the page as it is */ });
})();

// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

// Wiekko logo at the top of the sidebar, linking to the start page of the
// version being read. The SVG is a copy of web/static/img/logo.svg, inlined
// rather than loaded as an image so that "ko" takes the sidebar's text
// colour (currentColor) and "Wiek" can be lightened in the dark themes (see
// logo.css). mdBook fills the sidebar (mdbook-sidebar-scrollbox) from toc.js
// in <head>, so it is complete by the time this script runs.
(function () {
  "use strict";

  var svg =
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 240 64" fill="none" role="img" aria-label="Wiekko">' +
    '<circle cx="32" cy="32" r="30" fill="#4f46e5"/>' +
    '<circle cx="32" cy="32" r="22" fill="none" stroke="#c7d2fe" stroke-width="1.5" opacity="0.3"/>' +
    '<circle cx="18.8" cy="38.8" r="3.8" fill="#c7d2fe"/>' +
    '<circle cx="26.16" cy="19.44" r="4.2" fill="#c7d2fe"/>' +
    '<path d="M20 36 L24 22" stroke="#c7d2fe" stroke-width="2" stroke-linecap="round" fill="none"/>' +
    '<path d="M28 21 L35 30" stroke="#c7d2fe" stroke-width="2" stroke-linecap="round" fill="none"/>' +
    '<path d="m41.04 30.96 12-6" stroke="#fbbf24" stroke-width="3" stroke-linecap="round"/>' +
    '<path d="m47.253 23.338 6.395 0.32-1.716 6.169" stroke="#fbbf24" stroke-width="3" stroke-linecap="round" stroke-linejoin="round" fill="none"/>' +
    '<circle cx="37.36" cy="32.64" r="4" fill="#c7d2fe"/>' +
    '<circle cx="30" cy="44" r="2" fill="#c7d2fe" opacity="0.5"/>' +
    '<text x="76" y="42" font-family="Inter, ui-sans-serif, system-ui, sans-serif" font-size="28" font-weight="700">' +
    '<tspan class="logo-nex" fill="#4f46e5">Wiek</tspan><tspan fill="currentColor">ko</tspan>' +
    "</text></svg>";

  var box = document.querySelector("#mdbook-sidebar .sidebar-scrollbox");
  if (!box) return;
  var link = document.createElement("a");
  link.className = "sidebar-logo";
  link.href = (typeof path_to_root === "string" ? path_to_root : "") + "index.html";
  link.innerHTML = svg;
  box.insertBefore(link, box.firstChild);
})();

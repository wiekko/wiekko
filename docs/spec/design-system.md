# Design System

## Color Scheme

All colors use standard Tailwind CSS palettes. The tables below give the light mode values; dark mode (UC-68) uses Tailwind's `dark:` variant on the same palettes.

### Primary: Indigo

Professional, distinct from Jira's blue. Used for brand elements, navigation, and interactive controls.

| Role     | Class        | Hex       | Usage                             |
|----------|--------------|-----------|-----------------------------------|
| Lightest | `indigo-50`  | `#eef2ff` | Hover backgrounds, selected rows  |
| Light    | `indigo-100` | `#e0e7ff` | Active sidebar item background    |
| Focus    | `indigo-200` | `#c7d2fe` | Focus rings, light borders        |
| Mid      | `indigo-500` | `#6366f1` | Links, secondary buttons          |
| Brand    | `indigo-600` | `#4f46e5` | Primary buttons, nav active state |
| Hover    | `indigo-700` | `#4338ca` | Button hover state                |

### Accent: Amber

Warm contrast for attention-drawing elements without implying urgency.

| Role  | Class       | Hex       | Usage                      |
|-------|-------------|-----------|----------------------------|
| Light | `amber-400` | `#fbbf24` | Badges, notification dots  |
| Mid   | `amber-500` | `#f59e0b` | Accent buttons, highlights |
| Hover | `amber-600` | `#d97706` | Accent hover state         |

### Semantic Colors

| Meaning | Base    | Background   | Text/Icon     |
|---------|---------|--------------|---------------|
| Success | emerald | `emerald-50` | `emerald-700` |
| Warning | amber   | `amber-50`   | `amber-700`   |
| Error   | red     | `red-50`     | `red-700`     |
| Info    | sky     | `sky-50`     | `sky-700`     |

Emerald is preferred over green because its blue undertone harmonizes with indigo.

### Neutral: Slate

Blue-gray undertone complements indigo naturally.

| Usage              | Class       | Hex       |
|--------------------|-------------|-----------|
| Page background    | `slate-50`  | `#f8fafc` |
| Card / surface     | `white`     | `#ffffff` |
| Sidebar background | `slate-100` | `#f1f5f9` |
| Borders            | `slate-200` | `#e2e8f0` |
| Secondary text     | `slate-500` | `#64748b` |
| Primary text       | `slate-900` | `#0f172a` |

## Typography

### Fonts

| Purpose     | Font                      | License | File                                                |
|-------------|---------------------------|---------|-----------------------------------------------------|
| UI text     | Inter (variable)          | SIL OFL | `InterVariable.woff2`, `InterVariable-Italic.woff2` |
| Code / mono | JetBrains Mono (variable) | SIL OFL | `JetBrainsMono-Variable.woff2`                      |

Both are self-hosted and embedded into the binary. No external font loading.

### Type Scale

Uses Tailwind's default scale. Body text is 14px (`text-sm`), standard for data-dense tools.

| Element              | Classes                 | Size |
|----------------------|-------------------------|------|
| Page title (h1)      | `text-2xl font-bold`    | 24px |
| Section heading (h2) | `text-xl font-semibold` | 20px |
| Subsection (h3)      | `text-lg font-semibold` | 18px |
| Body text            | `text-sm`               | 14px |
| Small / caption      | `text-xs`               | 12px |
| Code                 | `font-mono text-sm`     | 14px |
| Button / Nav         | `text-sm font-medium`   | 14px |
| Badge / tag          | `text-xs font-medium`   | 12px |

## Logo

The logo combines a route/path motif (connected nodes evoking a project roadmap) with a forward-pointing arrow (progression, "next").

| File                           | Purpose                             |
|--------------------------------|-------------------------------------|
| `web/static/img/logo.svg`      | Full logo: icon + "Nexmap" wordmark |
| `web/static/img/logo-icon.svg` | Icon only: favicon, compact spaces  |

Colors: indigo-600 (`#4f46e5`) for nodes and hexagon, amber-500 (`#f59e0b`) for the forward arrow, indigo-200 (`#c7d2fe`) for the connecting path and waypoints.

## Asset Bundling

All static assets are embedded into the Go binary via `//go:embed` (see `web/embed.go`).

### Build Pipeline

1. `templ generate` — convert `.templ` templates to Go code
2. `just copy-assets` — copy fonts, JS, images to `web/dist/`
3. `tailwindcss` — compile `web/static/css/input.css` to `web/dist/css/styles.css`
4. `go build` — compile binary with embedded `web/dist/`

### Vendored Dependencies

| Asset          | Version | File                                                                 |
|----------------|---------|----------------------------------------------------------------------|
| HTMX           | 2.0.4   | `web/static/js/htmx.min.js`                                          |
| Inter          | 4.1     | `web/static/fonts/InterVariable.woff2`, `InterVariable-Italic.woff2` |
| JetBrains Mono | 2.304   | `web/static/fonts/JetBrainsMono-Variable.woff2`                      |

No CDN or runtime third-party dependencies. Tailwind CSS is compiled at build time using the standalone CLI.

## Components

### Collections

1. empty entity collections shall have a distinct visualisation (cf. /dev/catalogue)
2. collections containing elements shall display a create button above the collection (cf. /dev/catalogue)

// The shared look and feel of the public visualizations (VISUALIZATIONS.md):
// the typefaces, the colours fixed for categories (the colour registry), the
// type sizes, the stylesheet for a visualization's anatomy, and the number
// formats. Every chart module imports from here and nothing else sets a font,
// a size or a colour, so the Hugo site can import the same module and the
// charts look the same there. Dark mode is not built yet; the colours are
// here so it can be.

// Typefaces. IBM Plex Sans is the face of the website and chambers; Plex Mono
// is for numbers on axes and in tooltips, so that they align.
export const fonts = {
  sans: "'IBM Plex Sans', system-ui, -apple-system, 'Segoe UI', sans-serif",
  mono: "'IBM Plex Mono', ui-monospace, 'SFMono-Regular', Menlo, monospace",
};

const FONTS_URL =
  "https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:ital,wght@0,400;0,500;0,600;0,700;1,400&display=swap";

// Colours. The categories come from the registry in VISUALIZATIONS.md; the
// rest is the chart furniture.
//
// A state is coloured by its region (census.js), each of the nine groups
// with one colour of observable10, the two Northeast groups the scheme's two
// blues. The federal courts and the United Kingdom, outside the regions and
// often the longest bars, take the scheme's two most vivid colours, yellow
// and red-orange; the territories, seldom present, the grey.
export const groupColors = {
  "new-england": "#4269d0", // observable10 blue
  "mid-atlantic": "#97bbf5", // observable10 light blue
  midwest: "#3ca951", // observable10 green
  south: "#9c6b4e", // observable10 brown
  "mountain-west": "#a463f2", // observable10 purple
  "pacific-west": "#6cc5b0", // observable10 teal
  federal: "#efb118", // observable10 yellow
  uk: "#ff725c", // observable10 red-orange
  territories: "#9498a0", // observable10 grey
};

export const colors = {
  ink: "#212529",
  muted: "#6c757d",
  faint: "#adb5bd",
  grid: "#e9ecef",
  surface: "#ffffff",
  accent: "#ba02e8", // the website's purple; not fixed for any category
};

// Type sizes in pixels. The title is larger and bold, the subtitle at body
// size, the axis labels and the source and notes smaller and muted.
export const sizes = {
  title: 26,
  subtitle: 17,
  body: 15,
  axis: 12,
  label: 12,
  note: 13,
};

// plotStyle is the style option of every Plot.plot call, so the SVG carries
// its typography inline and an export looks like the screen.
export const plotStyle = {
  fontFamily: fonts.sans,
  fontSize: `${sizes.axis}px`,
  color: colors.ink,
  background: "transparent",
  overflow: "visible",
};

// The stylesheet for the anatomy of a visualization: title, subtitle, chart,
// source and notes, the controls above the chart, and the two-handled year
// slider. Scoped to .viz so it never touches the page around it.
const CSS = `
.viz { font-family: ${fonts.sans}; color: ${colors.ink}; }
.viz .viz-title { font-size: ${sizes.title}px; font-weight: 700; line-height: 1.2; margin: 0 0 0.25rem; letter-spacing: -0.01em; }
.viz .viz-subtitle { font-size: ${sizes.subtitle}px; font-weight: 400; line-height: 1.4; margin: 0 0 0.75rem; max-width: 60em; }
.viz .viz-subtitle .viz-num { font-variant-numeric: tabular-nums; }
.viz .viz-source, .viz .viz-notes { font-size: ${sizes.note}px; color: ${colors.muted}; line-height: 1.45; margin: 0; max-width: 70em; }
.viz .viz-source { margin-top: 0.5rem; }
.viz .viz-notes { margin-top: 0.25rem; }
.viz .viz-num, .viz .tabular { font-variant-numeric: tabular-nums; }
.viz .viz-controls { display: flex; flex-wrap: wrap; gap: 0.75rem 1.5rem; align-items: center; margin: 0 0 0.75rem; font-size: ${sizes.body}px; }
.viz .viz-control { display: flex; align-items: center; gap: 0.5rem; }
.viz .viz-control > label, .viz .viz-control > .viz-label { color: ${colors.muted}; font-size: ${sizes.note}px; white-space: nowrap; }
.viz .viz-select { font-family: inherit; font-size: ${sizes.body}px; padding: 0.25rem 1.75rem 0.25rem 0.5rem; border: 1px solid ${colors.faint}; border-radius: 4px; background: ${colors.surface}; color: ${colors.ink}; }
.viz .viz-toggle { display: inline-flex; border: 1px solid ${colors.faint}; border-radius: 4px; overflow: hidden; }
.viz .viz-toggle input { position: absolute; opacity: 0; width: 0; height: 0; }
.viz .viz-toggle label { padding: 0.25rem 0.75rem; cursor: pointer; font-size: ${sizes.note}px; color: ${colors.ink}; background: ${colors.surface}; border-left: 1px solid ${colors.faint}; margin: 0; }
.viz .viz-toggle label:first-of-type { border-left: 0; }
.viz .viz-toggle input:checked + label { background: ${colors.ink}; color: ${colors.surface}; }
.viz .viz-toggle input:focus-visible + label { outline: 2px solid ${colors.accent}; outline-offset: -2px; }
.viz .viz-range { position: relative; width: 16rem; height: 1.5rem; }
.viz .viz-range input[type=range] { position: absolute; left: 0; top: 0; width: 100%; height: 1.5rem; margin: 0; background: none; pointer-events: none; -webkit-appearance: none; appearance: none; }
.viz .viz-range input[type=range]::-webkit-slider-runnable-track { height: 4px; background: transparent; }
.viz .viz-range input[type=range]::-moz-range-track { height: 4px; background: transparent; }
.viz .viz-range input[type=range]::-webkit-slider-thumb { -webkit-appearance: none; appearance: none; pointer-events: auto; width: 16px; height: 16px; margin-top: -6px; border-radius: 50%; background: ${colors.surface}; border: 2px solid ${colors.ink}; cursor: pointer; }
.viz .viz-range input[type=range]::-moz-range-thumb { pointer-events: auto; width: 12px; height: 12px; border-radius: 50%; background: ${colors.surface}; border: 2px solid ${colors.ink}; cursor: pointer; }
.viz .viz-range input[type=range]:focus-visible::-webkit-slider-thumb { outline: 2px solid ${colors.accent}; outline-offset: 2px; }
.viz .viz-range input[type=range]:focus-visible::-moz-range-thumb { outline: 2px solid ${colors.accent}; outline-offset: 2px; }
.viz .viz-range .viz-range-track { position: absolute; left: 0; right: 0; top: calc(50% - 2px); height: 4px; border-radius: 2px; background: ${colors.grid}; }
.viz .viz-range .viz-range-fill { position: absolute; top: calc(50% - 2px); height: 4px; border-radius: 2px; background: ${colors.ink}; }
.viz .viz-years { font-family: ${fonts.mono}; font-size: ${sizes.body}px; min-width: 7.5em; }
.viz .viz-button { font-family: inherit; font-size: ${sizes.note}px; padding: 0.2rem 0.6rem; border: 1px solid ${colors.faint}; border-radius: 4px; background: ${colors.surface}; color: ${colors.ink}; cursor: pointer; }
.viz .viz-button:hover { border-color: ${colors.ink}; }
.viz .viz-chart { position: relative; }
.viz .viz-chart svg { display: block; }
.viz .viz-chart figure { margin: 0; }
.viz .viz-message { color: ${colors.muted}; font-size: ${sizes.body}px; padding: 2rem 0; }
.viz .viz-table { font-size: ${sizes.note}px; border-collapse: collapse; margin-top: 0.5rem; }
.viz .viz-table th, .viz .viz-table td { padding: 0.15rem 1rem 0.15rem 0; text-align: left; border-bottom: 1px solid ${colors.grid}; }
.viz .viz-table td.num, .viz .viz-table th.num { text-align: right; font-family: ${fonts.mono}; font-variant-numeric: tabular-nums; }
.viz details.viz-data { font-size: ${sizes.note}px; color: ${colors.muted}; margin-top: 0.75rem; }
.viz details.viz-data summary { cursor: pointer; }
.viz .visually-hidden { position: absolute !important; width: 1px !important; height: 1px !important; padding: 0 !important; margin: -1px !important; overflow: hidden !important; clip: rect(0, 0, 0, 0) !important; white-space: nowrap !important; border: 0 !important; }
@media (prefers-reduced-motion: no-preference) {
  .viz .viz-range .viz-range-fill { transition: left 80ms linear, width 80ms linear; }
}
`;

// ensureStyles loads the typefaces and the stylesheet once per page.
export function ensureStyles() {
  if (!document.getElementById("viz-fonts")) {
    const link = document.createElement("link");
    link.id = "viz-fonts";
    link.rel = "stylesheet";
    link.href = FONTS_URL;
    document.head.appendChild(link);
  }
  if (!document.getElementById("viz-style")) {
    const style = document.createElement("style");
    style.id = "viz-style";
    style.textContent = CSS;
    document.head.appendChild(style);
  }
}

// Number formats: thousands separated, and a percentage of a whole.
const numberFormat = new Intl.NumberFormat("en-US");
export const fmt = (n) => numberFormat.format(n);
export const pct = (part, whole) => {
  if (!(whole > 0)) return "\u2014";
  const p = (100 * part) / whole;
  return p > 0 && p < 0.5 ? "<1%" : `${Math.round(p)}%`;
};

// reducedMotion is whether the reader asked for no animation.
export const reducedMotion = () =>
  window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches;

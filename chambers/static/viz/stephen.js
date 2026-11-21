// Stephen on Pleading (issue #439): the cases cited in each American edition
// of Henry John Stephen's Treatise on the Principles of Pleading, as Claude read
// the official copy of each edition page by page and linked its citations, split
// by the jurisdiction of the case cited: American, English, or unknown. Two
// charts on one x-axis of the editions' years, so the uneven gaps between them
// show: the number of citations in each edition, and the share of each.
//
// The module takes data and options and returns nodes; it reads nothing from
// the page. `summarize` reduces the rows of /api/viz/stephen (one per edition
// and jurisdiction) to the editions and the words for the title and subtitle;
// `chart` draws either measure; `legend` and `table` are the key and the
// accessible form of the same numbers.

import * as Plot from "https://esm.sh/@observablehq/plot@0.6?bundle-deps";
import { colors, citedColors, fonts, sizes, plotStyle, fmt, pct } from "./style.js";

// JURISDICTIONS is the cited jurisdictions in legend and stacking order, with
// the words the chart labels them by and the shape and dash that pair with the
// colour, so that no series is told apart by colour alone.
export const JURISDICTIONS = [
  { key: "US", label: "American cases", short: "US", symbol: "circle", dash: null },
  { key: "UK", label: "English cases", short: "UK", symbol: "square", dash: "6,3" },
  { key: "Unknown", label: "Unknown", short: "Unknown", symbol: "triangle", dash: "2,3" },
];
const byKey = new Map(JURISDICTIONS.map((j) => [j.key, j]));

// summarize gathers the rows into one object per edition, in order of year,
// with its citations to each jurisdiction, the total, and whether the copy is
// only partly read. A row's share is of its edition's total.
export function summarize(data) {
  const editions = new Map();
  for (const r of data) {
    let e = editions.get(r.bibliographicid);
    if (!e) {
      e = {
        year: r.year, bibliographicid: r.bibliographicid, psmid: r.psmid,
        edition_statement: r.edition_statement || "", pages_read: r.pages_read, pages: r.pages,
        partial: r.pages_read < r.pages, total: 0, US: 0, UK: 0, Unknown: 0,
      };
      editions.set(r.bibliographicid, e);
    }
    e[r.jurisdiction] = (e[r.jurisdiction] || 0) + r.citations;
    e.total += r.citations;
  }
  const list = [...editions.values()].sort((a, b) => a.year - b.year || a.psmid.localeCompare(b.psmid));

  // The long form Plot draws: one row per edition and jurisdiction, with the
  // share and the bounds of its segment in the stacked bars (US at the bottom).
  const rows = [];
  for (const e of list) {
    let y = 0;
    for (const j of JURISDICTIONS) {
      const share = e.total > 0 ? e[j.key] / e.total : 0;
      rows.push({ ...e, jurisdiction: j.key, label: j.label, citations: e[j.key], share, y1: y, y2: y + share });
      y += share;
    }
  }

  const summary = { editions: list, rows };
  summary.title = "The cases cited in the American editions of Stephen on Pleading";
  summary.subtitle = subtitle(list);
  return summary;
}

// subtitle states the finding: how English the citations of the American
// editions are, and the first edition, if any, in which American cases are the
// majority.
function subtitle(editions) {
  const n = (x) => `<span class="viz-num">${x}</span>`;
  const read = editions.filter((e) => e.total > 0);
  if (read.length === 0) return "None of the American editions read so far cites a case.";
  const turn = read.find((e) => e.US / e.total > 0.5);
  const before = turn ? read.filter((e) => e.year < turn.year) : read;
  let s = "";
  if (before.length > 0) {
    const shares = before.map((e) => e.UK / e.total);
    const lo = Math.min(...shares), hi = Math.max(...shares);
    const span = pct(lo, 1) === pct(hi, 1) ? n(pct(lo, 1)) : `${n(pct(lo, 1))} to ${n(pct(hi, 1))}`;
    const years = before.length === 1 ? `the ${before[0].year} edition` : `the editions of ${before[0].year} to ${before[before.length - 1].year}`;
    s = `In ${years}, ${span} of the cases cited are English.`;
  }
  if (turn) {
    s += `${s ? " " : ""}In the ${turn.year} edition, American cases are ${n(pct(turn.US, turn.total))} of the ${n(fmt(turn.total))} citations${turn.partial ? " on the pages read so far" : ""}.`;
  }
  return s;
}

// height is the height of one chart for a width: wide and short, as a figure
// in an article is, within bounds.
export const height = (width) => Math.round(Math.max(240, Math.min(380, width * 0.42)));

const MARGIN_RIGHT = 84; // room for a direct label like "Unknown"
const MARGIN_TOP = 30; // room for the y-axis label above the top tick

// EDITION_SPAN is the first and last years of the American editions.
const EDITION_SPAN = [1831, 1901];

// tooltip is the text of an edition's tip.
function tooltip(e) {
  const lines = [`${e.year} edition (${e.psmid})`];
  if (e.edition_statement) lines.push(e.edition_statement);
  for (const j of JURISDICTIONS) lines.push(`${j.label}: ${fmt(e[j.key])} (${pct(e[j.key], e.total)})`);
  lines.push(`All citations: ${fmt(e.total)}`);
  lines.push(e.partial ? `Partly read: ${fmt(e.pages_read)} of ${fmt(e.pages)} pages` : `${fmt(e.pages)} pages read`);
  return lines.join("\n");
}

// chart draws one measure at the given size and returns Plot's SVG:
// "count", a line for each jurisdiction with a dot at each edition, or
// "share", a stacked bar at each edition. A partly read edition has a hollow
// dot, or an asterisk over its bar. Each series is labelled at its last
// edition, so the chart needs no legend of its own.
export function chart(summary, { width, height: h, measure = "count" }) {
  const { editions, rows } = summary;
  // The axis spans every American edition, read or not, so that it stays put
  // as editions are added.
  const years = [...editions.map((e) => e.year), ...EDITION_SPAN];
  const x = {
    domain: [Math.min(...years) - 3, Math.max(...years) + 3],
    label: null,
    tickFormat: (d) => `${d}`,
    tickSize: 0,
    ticks: 8,
  };
  const color = {
    domain: JURISDICTIONS.map((j) => j.key),
    range: JURISDICTIONS.map((j) => citedColors[j.key]),
  };
  const last = editions[editions.length - 1];
  const lastRows = rows.filter((r) => r.bibliographicid === last.bibliographicid);
  const description = summary.subtitle.replace(/<[^>]+>/g, "");
  const tipStyle = { fontFamily: fonts.sans, fontSize: sizes.label, fill: colors.surface, stroke: colors.faint };
  const labelStyle = { fontFamily: fonts.sans, fontSize: sizes.label, fontWeight: 600, textAnchor: "start", dx: 8 };

  if (measure === "share") {
    const half = 1.1; // half a bar's width in years, so the bars of 1851, 1854 and 1857 do not touch
    return Plot.plot({
      width, height: h, marginRight: MARGIN_RIGHT, marginTop: MARGIN_TOP, marginLeft: 44,
      style: plotStyle,
      ariaLabel: "Share of each American edition's citations that are to American, English and unknown cases",
      ariaDescription: description,
      x,
      y: { domain: [0, 1], label: "Share of the edition's citations", tickFormat: (d) => `${Math.round(100 * d)}%`, tickSize: 0, grid: true },
      color,
      marks: [
        Plot.rect(rows, {
          x1: (d) => d.year - half, x2: (d) => d.year + half, y1: "y1", y2: "y2",
          fill: "jurisdiction", stroke: colors.surface, strokeWidth: 0.75,
        }),
        Plot.text(editions.filter((e) => e.partial), { x: "year", y: 1, text: () => "*", dy: -6, fill: colors.ink, fontSize: sizes.body, fontWeight: 700 }),
        // Direct labels at the middle of the last bar's segments; a segment
        // too thin to carry one is in the legend and the table.
        Plot.text(lastRows.filter((r) => r.share >= 0.04), {
          x: (d) => d.year + half, y: (d) => (d.y1 + d.y2) / 2, text: (d) => byKey.get(d.jurisdiction).short,
          fill: colors.ink, ...labelStyle,
        }),
        Plot.ruleY([0], { stroke: colors.faint }),
        Plot.tip(editions, Plot.pointerX({ x: "year", y: 1, title: tooltip, ...tipStyle })),
      ],
    });
  }

  const max = Math.max(1, ...rows.map((r) => r.citations));
  const marks = [Plot.ruleY([0], { stroke: colors.faint })];
  for (const j of JURISDICTIONS) {
    const series = rows.filter((r) => r.jurisdiction === j.key);
    marks.push(Plot.line(series, { x: "year", y: "citations", stroke: citedColors[j.key], strokeWidth: 2, strokeDasharray: j.dash }));
    marks.push(Plot.dot(series, {
      x: "year", y: "citations", symbol: () => j.symbol, r: 4.5,
      stroke: citedColors[j.key], strokeWidth: 1.5,
      fill: (d) => (d.partial ? colors.surface : citedColors[j.key]),
    }));
  }
  marks.push(Plot.text(lastRows, {
    x: "year", y: "citations", text: (d) => byKey.get(d.jurisdiction).short,
    fill: colors.ink, ...labelStyle,
  }));
  marks.push(Plot.tip(editions, Plot.pointerX({ x: "year", y: (e) => Math.max(e.US, e.UK, e.Unknown), title: tooltip, ...tipStyle })));
  return Plot.plot({
    width, height: h, marginRight: MARGIN_RIGHT, marginTop: MARGIN_TOP, marginLeft: 52,
    style: plotStyle,
    ariaLabel: "Number of citations to American, English and unknown cases in each American edition",
    ariaDescription: description,
    x,
    y: { domain: [0, max], nice: true, label: "Citations", tickFormat: fmt, tickSize: 0, grid: true },
    symbol: { domain: JURISDICTIONS.map((j) => j.key), range: JURISDICTIONS.map((j) => j.symbol) },
    marks,
  });
}

// legend is the key the two charts share: each jurisdiction's colour, shape
// and name.
export function legend() {
  const el = document.createElement("div");
  el.className = "viz-legend";
  const shapes = {
    circle: '<circle cx="7" cy="7" r="5"/>',
    square: '<rect x="2.5" y="2.5" width="9" height="9"/>',
    triangle: '<path d="M7 1.5 L12.5 12 L1.5 12 Z"/>',
  };
  for (const j of JURISDICTIONS) {
    const item = document.createElement("span");
    item.className = "viz-swatch";
    item.innerHTML = `<svg width="14" height="14" aria-hidden="true" fill="${citedColors[j.key]}">${shapes[j.symbol]}</svg>${j.label}`;
    el.appendChild(item);
  }
  return el;
}

// table lays the editions out as an HTML table, the accessible form of both
// charts.
export function table(summary) {
  const t = document.createElement("table");
  t.className = "viz-table";
  const head = JURISDICTIONS.map((j) => `<th scope="col" class="num">${j.label}</th><th scope="col" class="num">Share</th>`).join("");
  t.innerHTML = `<caption class="visually-hidden">${summary.title}</caption>
    <thead><tr><th scope="col">Edition</th><th scope="col">Copy read</th>${head}<th scope="col" class="num">All</th><th scope="col" class="num">Pages read</th></tr></thead>`;
  const body = document.createElement("tbody");
  for (const e of summary.editions) {
    const tr = document.createElement("tr");
    const cells = JURISDICTIONS.map((j) => `<td class="num">${fmt(e[j.key])}</td><td class="num">${pct(e[j.key], e.total)}</td>`).join("");
    const pages = e.partial ? `${fmt(e.pages_read)} of ${fmt(e.pages)}` : fmt(e.pages_read);
    tr.innerHTML = `<td>${e.year}${e.partial ? " *" : ""}</td><td>${e.psmid}</td>${cells}<td class="num">${fmt(e.total)}</td><td class="num">${pages}</td>`;
    body.appendChild(tr);
  }
  t.appendChild(body);
  return t;
}

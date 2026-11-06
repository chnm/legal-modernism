// Citations between jurisdictions (issue #216): for one jurisdiction and a
// range of years, a bar for each other jurisdiction its cases cite, or that
// cites it, longest first. The citations are our links from the opinions of
// the CAP cases to the cases of CAP and of the English Reports, rolled up by
// jurisdiction in opinion_citations.jurisdiction_citations.
//
// The module takes data and options and returns nodes; it reads nothing from
// the page. `summarize` reduces the edges of one jurisdiction (the rows of
// /api/viz/jurisdictions/{slug}) to the bars, the home figure and the words
// for the title and subtitle; `chart` draws the bars; `table` lays the same
// numbers out for a screen reader or a sceptic.

import * as Plot from "https://esm.sh/@observablehq/plot@0.6?bundle-deps";
import { colors, groupColors, fonts, sizes, plotStyle, fmt, pct } from "./style.js";
import { GROUPS, groupOf, regionOf } from "./census.js";

// Jurisdictions whose long name takes the article.
const ARTICLE = new Set(["United States", "United Kingdom", "District of Columbia"]);
const the = (name) => (ARTICLE.has(name) ? `the ${name}` : name);
const possessive = (name) => `${the(name)}'s`;
const capital = (s) => s.charAt(0).toUpperCase() + s.slice(1);

export const yearSpan = (from, to) => (from === to ? `${from}` : `${from}–${to}`);

// summarize turns the edges of the selected jurisdiction into the chart's
// bars under the options: `jurisdiction` (a slug), `direction` ("cites" for
// the jurisdictions its cases cite, "cited" for those whose cases cite it),
// `from` and `to` (the citing cases' decision years, inclusive), and
// `measure` ("citations" or "cases", the distinct citing cases). Citations
// between cases of the selected jurisdiction are the home figure, not a bar.
export function summarize(edges, jurisdictions, options) {
  const { jurisdiction, direction, from, to, measure } = options;
  const bySlug = new Map(jurisdictions.map((j) => [j.jurisdiction, j]));
  const selected = bySlug.get(jurisdiction) || { jurisdiction, name: jurisdiction, name_long: jurisdiction };
  const otherKey = direction === "cites" ? "cited_jurisdiction" : "citing_jurisdiction";
  const selfKey = direction === "cites" ? "citing_jurisdiction" : "cited_jurisdiction";

  const acc = new Map();
  const home = { citations: 0, citing_cases: 0 };
  for (const e of edges) {
    if (e.citing_year < from || e.citing_year > to) continue;
    if (e[selfKey] !== jurisdiction) continue;
    const other = e[otherKey];
    if (other === jurisdiction) {
      home.citations += e.citations;
      home.citing_cases += e.citing_cases;
      continue;
    }
    let bar = acc.get(other);
    if (!bar) {
      const j = bySlug.get(other) || { jurisdiction: other, name: other, name_long: other };
      const group = groupOf(other);
      bar = {
        jurisdiction: other, name: j.name, name_long: j.name_long,
        group: group.name, region: regionOf(group),
        citations: 0, citing_cases: 0,
      };
      acc.set(other, bar);
    }
    bar.citations += e.citations;
    bar.citing_cases += e.citing_cases;
  }

  const valueOf = measure === "cases" ? (b) => b.citing_cases : (b) => b.citations;
  const bars = [...acc.values()].filter((b) => valueOf(b) > 0);
  for (const b of bars) b.value = valueOf(b);
  bars.sort((a, b) => b.value - a.value || a.name.localeCompare(b.name));
  const otherCitations = bars.reduce((s, b) => s + b.citations, 0);
  for (const b of bars) b.share = otherCitations > 0 ? b.citations / otherCitations : 0;

  const summary = {
    selected, direction, measure, from, to, bars, home,
    otherCitations,
    allCitations: otherCitations + home.citations,
  };
  summary.title = title(summary);
  summary.subtitle = subtitle(summary);
  summary.measureLabel = measure === "cases" ? "Citing cases" : "Citations";
  return summary;
}

function title({ selected, direction, measure, from, to, bars }) {
  const n = bars.length;
  const years = yearSpan(from, to);
  const who = `${n} jurisdiction${n === 1 ? "" : "s"}`;
  if (n === 0) {
    return direction === "cites"
      ? `No citations from ${selected.name} to other jurisdictions, ${years}`
      : `No citations to ${selected.name} from other jurisdictions, ${years}`;
  }
  if (direction === "cites") {
    return measure === "cases"
      ? `${selected.name} cases citing ${who}, ${years}`
      : `Citations from ${selected.name} to ${who}, ${years}`;
  }
  return measure === "cases"
    ? `Cases from ${who} citing ${selected.name}, ${years}`
    : `Citations to ${selected.name} from ${who}, ${years}`;
}

function subtitle({ selected, direction, measure, from, to, bars, home, otherCitations, allCitations }) {
  const years = from === to ? `in ${from}` : `between ${from} and ${to}`;
  const S = selected.name_long;
  if (bars.length === 0) {
    return direction === "cites"
      ? `No case decided in ${the(S)} ${years} has a linked citation to a case of another jurisdiction.`
      : `No case decided ${years} in another jurisdiction has a linked citation to a case of ${the(S)}.`;
  }
  const top = bars[0];
  const T = top.name_long;
  const n = (x) => `<span class="viz-num">${fmt(x)}</span>`;
  const p = (a, b) => `<span class="viz-num">${pct(a, b)}</span>`;
  if (measure === "cases") {
    if (direction === "cites") {
      let s = `Of the cases decided in ${the(S)} ${years}, ${n(top.citing_cases)} cite a case of ${the(T)}, more than cite the cases of any other jurisdiction.`;
      if (home.citing_cases > 0) s += ` ${n(home.citing_cases)} cite another case of ${the(S)}.`;
      return s;
    }
    let s = `Of the cases decided ${years} in other jurisdictions, more of ${possessive(T)} cite a case of ${the(S)} than of any other: ${n(top.citing_cases)} of them.`;
    if (home.citing_cases > 0) s += ` ${n(home.citing_cases)} of ${possessive(S)} own cases cite another.`;
    return s;
  }
  if (direction === "cites") {
    let s = `Cases decided in ${the(S)} ${years} cite ${the(T)} most: ${n(top.citations)} of their ${n(otherCitations)} citations to the cases of other jurisdictions, ${p(top.citations, otherCitations)}.`;
    if (home.citations > 0) {
      s += ` A further ${n(home.citations)}, ${p(home.citations, allCitations)} of all their citations, are to ${possessive(S)} own cases.`;
    }
    return s;
  }
  let s = `Of the cases decided ${years} in other jurisdictions, those of ${the(T)} cite the cases of ${the(S)} most: ${n(top.citations)} of the ${n(otherCitations)} citations from elsewhere, ${p(top.citations, otherCitations)}.`;
  if (home.citations > 0) {
    s += ` ${capital(possessive(S))} own cases add ${n(home.citations)} more, ${p(home.citations, allCitations)} of all the citations to them.`;
  }
  return s;
}

// tooltip is the text of a bar's tip.
function tooltip(d, { selected, direction, otherCitations }) {
  const citing = direction === "cites" ? selected.name : d.name;
  const cited = direction === "cites" ? d.name : selected.name;
  return [
    d.region ? `${d.name_long} (${d.group}, ${d.region})` : `${d.name_long} (${d.group})`,
    `${fmt(d.citations)} citation${d.citations === 1 ? "" : "s"} from ${fmt(d.citing_cases)} ${citing} case${d.citing_cases === 1 ? "" : "s"} to ${cited} cases`,
    `${pct(d.citations, otherCitations)} of the citations ${direction === "cites" ? "to" : "from"} other jurisdictions`,
  ].join("\n");
}

// Chart geometry: the pixel height a bar wants and the least it can take
// before its label collides with the next, and the margins.
export const BAR_HEIGHT = 22;
export const MIN_BAR_HEIGHT = 16;
const MARGIN_TOP = 36;
const MARGIN_BOTTOM = 8;
const MARGIN_RIGHT = 72;

// height is the height the chart takes: what its bars want, shrunk to the
// `available` height down to the least the bars can take, beyond which the
// chart is taller than the space and its container scrolls.
export function height(summary, available) {
  const n = Math.max(1, summary.bars.length);
  const margins = MARGIN_TOP + MARGIN_BOTTOM;
  const wanted = margins + n * BAR_HEIGHT;
  const least = margins + n * MIN_BAR_HEIGHT;
  return Math.max(120, least, Math.min(wanted, available));
}

// chart draws the bars at the given size and returns Plot's figure, the SVG
// with its legend.
export function chart(summary, { width, height: h }) {
  const { bars, selected, direction, measure } = summary;
  const longest = bars.reduce((m, b) => Math.max(m, b.name.length), 4);
  const marginLeft = Math.min(140, 16 + longest * 7.5);
  const description = summary.subtitle.replace(/<[^>]+>/g, "");
  let label;
  if (measure === "cases") {
    label = direction === "cites" ? `${selected.name} cases citing →` : `Cases citing ${selected.name} →`;
  } else {
    label = direction === "cites" ? `Citations from ${selected.name} →` : `Citations to ${selected.name} →`;
  }

  return Plot.plot({
    width,
    height: h,
    marginLeft,
    marginRight: MARGIN_RIGHT,
    marginTop: MARGIN_TOP,
    marginBottom: MARGIN_BOTTOM,
    style: plotStyle,
    ariaLabel: summary.title,
    ariaDescription: description,
    x: {
      axis: "top",
      grid: true,
      label,
      labelAnchor: "right",
      tickFormat: (d) => fmt(d),
      tickSize: 0,
      nice: true,
    },
    y: {
      label: null,
      domain: bars.map((b) => b.name),
      tickSize: 0,
      padding: 0.18,
    },
    color: {
      // The legend lists the groups present, in the Census Bureau's order.
      domain: GROUPS.filter((g) => bars.some((b) => b.group === g.name)).map((g) => g.name),
      range: GROUPS.filter((g) => bars.some((b) => b.group === g.name)).map((g) => groupColors[g.key]),
      legend: true,
      style: { fontFamily: fonts.sans, fontSize: `${sizes.label}px`, color: colors.ink },
      swatchSize: 12,
    },
    marks: [
      Plot.barX(bars, { x: "value", y: "name", fill: "group" }),
      Plot.ruleX([0], { stroke: colors.faint }),
      Plot.text(bars, {
        x: "value",
        y: "name",
        text: (d) => fmt(d.value),
        textAnchor: "start",
        dx: 6,
        fill: colors.ink,
        fontFamily: fonts.mono,
        fontSize: sizes.label,
      }),
      Plot.tip(bars, Plot.pointerY({
        x: "value",
        y: "name",
        title: (d) => tooltip(d, summary),
        fontFamily: fonts.sans,
        fontSize: sizes.label,
        fill: colors.surface,
        stroke: colors.faint,
      })),
    ],
  });
}

// table lays the bars out as an HTML table, the accessible form of the chart.
export function table(summary) {
  const { bars, selected, direction, home } = summary;
  const t = document.createElement("table");
  t.className = "viz-table";
  const other = direction === "cites" ? "Cited jurisdiction" : "Citing jurisdiction";
  const cases = direction === "cites" ? `${selected.name} cases citing it` : "Cases citing it";
  t.innerHTML = `<caption class="visually-hidden">${summary.title}</caption>
    <thead><tr><th scope="col">${other}</th><th scope="col">Census division</th><th scope="col" class="num">Citations</th><th scope="col" class="num">${cases}</th><th scope="col" class="num">Share</th></tr></thead>`;
  const body = document.createElement("tbody");
  for (const b of bars) {
    const tr = document.createElement("tr");
    const division = b.region ? `${b.group} (${b.region})` : b.group;
    tr.innerHTML = `<td>${b.name_long}</td><td>${division}</td><td class="num">${fmt(b.citations)}</td><td class="num">${fmt(b.citing_cases)}</td><td class="num">${pct(b.citations, summary.otherCitations)}</td>`;
    body.appendChild(tr);
  }
  if (home.citations > 0) {
    const tr = document.createElement("tr");
    tr.innerHTML = `<td>${selected.name_long} (its own cases; not charted)</td><td></td><td class="num">${fmt(home.citations)}</td><td class="num">${fmt(home.citing_cases)}</td><td class="num">—</td>`;
    body.appendChild(tr);
  }
  t.appendChild(body);
  return t;
}

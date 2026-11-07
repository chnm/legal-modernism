// The migration of the Field Code (issue #117): figure 5 of Kellen Funk and
// Lincoln A. Mullen, "The Spine of American Law: Digital Text Analysis and
// U.S. Legal Practice," American Historical Review 123, no. 1 (2018), as an
// interactive network. Each node is a code or statute of civil procedure in
// the article's corpus, each arrow a borrowing: it runs from the code
// borrowed from to the code that borrowed, as wide as the sections it carried.
// A code is coloured by its distance, along the arrows, from one of the seven
// drafts of New York's Field Code.
//
// The module takes data and options and returns nodes; it reads nothing from
// the page. `graph` joins the two arrays of /api/viz/field-code once;
// `summarize` reduces the graph to what the chart shows under the options
// (the last year shown, whether the codes outside the Field Code's network
// are shown, the selected code) and the words for the title and subtitle;
// `network` draws the force layout and returns a controller the page calls as
// the summary changes; `table` lays the visible codes out for a screen reader
// or a sceptic. This is D3 rather than Plot because Plot has no force layout
// (VISUALIZATIONS.md).

import * as d3 from "https://esm.sh/d3@7.9.0?bundle-deps";
import { colors, borrowingColors, fonts, sizes, fmt, pct, reducedMotion } from "./style.js";

// The four categories, in legend order, and their names.
export const CATEGORIES = Object.keys(borrowingColors);
const FIELD = "Field code";
const ORIGINAL = "Original borrower";
const SUBSEQUENT = "Subsequent borrower";

// graph joins the codes and the borrowings into the nodes and links of the
// network, once. A link's `source` is the code borrowed from and its `target`
// the borrower, the way the arrow runs. Each node gets its `sources` (the
// links to the codes it borrowed from, largest first), its `borrowers` (the
// links from the codes that borrowed from it, earliest first), whether it is
// `connected` to a Field code by any chain of borrowings in either direction
// (the codes that are not form the small clusters at the edge of the figure),
// and its `path`, the shortest chain of codes from it to a Field code, which
// the subtitle recites.
export function graph(codes, borrowings) {
  const nodes = codes.map((c) => ({ ...c, name: `${c.jurisdiction} ${c.year}`, sources: [], borrowers: [] }));
  const byCode = new Map(nodes.map((n) => [n.code, n]));
  const links = [];
  for (const b of borrowings) {
    const source = byCode.get(b.borrowed_from);
    const target = byCode.get(b.borrower);
    if (!source || !target) continue;
    const link = { ...b, source, target };
    links.push(link);
    target.sources.push(link);
    source.borrowers.push(link);
  }
  for (const n of nodes) {
    n.sources.sort((a, b) => b.sections_borrowed - a.sections_borrowed || a.source.code.localeCompare(b.source.code));
    n.borrowers.sort((a, b) => a.target.year - b.target.year || a.target.code.localeCompare(b.target.code));
  }

  const fieldCodes = nodes.filter((n) => n.category === FIELD);
  const connected = new Set(fieldCodes);
  const queue = [...fieldCodes];
  while (queue.length) {
    const n = queue.pop();
    for (const l of n.sources) if (!connected.has(l.source)) { connected.add(l.source); queue.push(l.source); }
    for (const l of n.borrowers) if (!connected.has(l.target)) { connected.add(l.target); queue.push(l.target); }
  }
  for (const n of nodes) {
    n.connected = connected.has(n);
    n.path = pathToFieldCode(n);
  }
  return {
    nodes,
    links,
    byCode,
    years: d3.extent(nodes, (n) => n.year),
    maxSections: d3.max(links, (l) => l.sections_borrowed) || 1,
  };
}

// pathToFieldCode is the shortest chain of codes from a node to a Field code,
// following each borrowing to its source: the node first, the Field code
// last, or null when no chain reaches one. A Field code's path is itself.
function pathToFieldCode(start) {
  const parent = new Map([[start, null]]);
  const queue = [start];
  while (queue.length) {
    const n = queue.shift();
    if (n.category === FIELD) {
      const path = [];
      for (let p = n; p; p = parent.get(p)) path.push(p);
      return path.reverse();
    }
    for (const l of n.sources) {
      if (!parent.has(l.source)) {
        parent.set(l.source, n);
        queue.push(l.source);
      }
    }
  }
  return null;
}

// summarize reduces the graph to what the chart shows under the options:
// `year`, the last year of enactment shown (the codes enacted later are hidden
// in place, so the network can be watched growing); `outside`, whether the
// codes with no connection to a Field code are shown; and `selected`, the code
// whose line of descent is traced, or null. It returns the visible nodes and
// links, the counts by category, the lineage of the selected code, and the
// words for the title and the subtitle.
export function summarize(g, options = {}) {
  const year = options.year ?? g.years[1];
  const outside = options.outside ?? true;
  const visible = g.nodes.filter((n) => n.year <= year && (outside || n.connected));
  const visibleSet = new Set(visible);
  const links = g.links.filter((l) => visibleSet.has(l.source) && visibleSet.has(l.target));
  const counts = { total: visible.length, hidden: outside ? 0 : g.nodes.filter((n) => n.year <= year && !n.connected).length };
  for (const c of CATEGORIES) counts[c] = 0;
  for (const n of visible) counts[n.category]++;
  const candidate = options.selected ? g.byCode.get(options.selected) : null;
  const selected = candidate && visibleSet.has(candidate) ? candidate : null;
  const summary = {
    graph: g, year, outside, allYears: year >= g.years[1],
    visible, visibleSet, links, counts, selected,
    lineage: selected ? lineage(selected, visibleSet) : null,
  };
  summary.title = title(summary);
  summary.subtitle = subtitle(summary);
  return summary;
}

// lineage is a code's line of descent among the visible codes: its ancestors
// (the codes it borrowed from, and theirs, and so on), its descendants (the
// codes that borrowed from it, and from them), and the links among them that
// carry text toward it or away from it.
function lineage(node, visibleSet) {
  const ancestors = new Set();
  const up = [node];
  while (up.length) {
    const n = up.pop();
    for (const l of n.sources) {
      if (visibleSet.has(l.source) && !ancestors.has(l.source)) { ancestors.add(l.source); up.push(l.source); }
    }
  }
  const descendants = new Set();
  const down = [node];
  while (down.length) {
    const n = down.pop();
    for (const l of n.borrowers) {
      if (visibleSet.has(l.target) && !descendants.has(l.target)) { descendants.add(l.target); down.push(l.target); }
    }
  }
  const nodes = new Set([node, ...ancestors, ...descendants]);
  const links = new Set();
  for (const n of nodes) {
    for (const l of n.sources) {
      const towardRoot = (n === node || ancestors.has(n)) && ancestors.has(l.source);
      const fromNode = descendants.has(n) && (l.source === node || descendants.has(l.source));
      if (towardRoot || fromNode) links.add(l);
    }
  }
  return { node, ancestors, descendants, nodes, links };
}

const num = (x) => `<span class="viz-num">${fmt(x)}</span>`;
const plural = (k, word) => `${fmt(k)} ${word}${k === 1 ? "" : "s"}`;

// list joins words as prose: a; a and b; a, b and c.
function list(items) {
  if (items.length <= 1) return items.join("");
  return `${items.slice(0, -1).join(", ")} and ${items[items.length - 1]}`;
}

function title({ allYears, year }) {
  return allYears ? "The migration of the Field Code" : `The migration of the Field Code by ${year}`;
}

function subtitle(s) {
  if (s.selected) return describe(s);
  const { counts, year, allYears, graph: g } = s;
  if (counts.total === 0) return `No code in the corpus had been enacted by ${year}.`;
  const when = allYears ? `between ${g.years[0]} and ${g.years[1]}` : `by ${year}`;
  let text;
  if (counts[FIELD] === 0) {
    text = `Of the ${num(counts.total)} codes and statutes of civil procedure enacted ${when}, none yet borrowed from New York's Field Code, whose first draft came in 1848.`;
  } else {
    const drafts = counts[FIELD] === 7 ? `${num(7)} are the drafts of New York's Field Code` : `${num(counts[FIELD])} ${counts[FIELD] === 1 ? "is a draft" : "are drafts"} of New York's Field Code`;
    const nothing = counts["Did not borrow"] === 0 ? "none took nothing from one" : `${num(counts["Did not borrow"])} took nothing from one`;
    text = `Of the ${num(counts.total)} codes and statutes of civil procedure enacted ${when}, ${drafts}, ${num(counts[ORIGINAL])} borrowed from one directly and ${num(counts[SUBSEQUENT])} more through another code; ${nothing}.`;
  }
  if (counts.hidden > 0) text += ` The ${num(counts.hidden)} with no tie to a Field code are hidden.`;
  return text;
}

// describe says where the selected code stands: its place in the line of
// descent, what it took and from whom, and who took from it, as far as the
// visible years allow.
function describe(s) {
  const d = s.selected;
  const { visibleSet, year, allYears, lineage: lin } = s;
  const parts = [];
  switch (d.category) {
    case FIELD:
      parts.push(`${d.name} (${d.code}) is one of the seven drafts of New York's Field Code.`);
      break;
    case ORIGINAL:
      parts.push(`${d.name} (${d.code}) borrowed directly from a draft of the Field Code.`);
      break;
    case SUBSEQUENT: {
      const root = d.path[d.path.length - 1];
      const via = d.path.slice(1, -1).map((p) => p.name);
      parts.push(`${d.name} (${d.code}) descends from ${root.name} by way of ${list(via)}.`);
      break;
    }
    default:
      parts.push(`${d.name} (${d.code}) took nothing from a draft of the Field Code, directly or indirectly.`);
  }

  const sources = d.sources.filter((l) => visibleSet.has(l.source));
  if (sources.length) {
    const clauses = sources.map((l, i) =>
      `${num(l.sections_borrowed)}${i === 0 ? ` of its ${num(d.sections)} sections` : ""} (${pct(l.sections_borrowed, d.sections)}) from ${l.source.name}`);
    parts.push(`It took ${list(clauses)}.`);
  } else {
    parts.push("It has no source among the codes shown.");
  }

  const borrowers = d.borrowers.filter((l) => visibleSet.has(l.target));
  const further = lin.descendants.size - borrowers.length;
  const by = allYears ? "" : ` by ${year}`;
  if (borrowers.length === 0) {
    parts.push(`No later code${by} borrowed from it.`);
  } else if (borrowers.length === 1) {
    const more = further > 0 ? `, and ${plural(further, "more code")} descend from it through that one` : "";
    parts.push(`One later code${by} borrowed from it, ${borrowers[0].target.name}${more}.`);
  } else {
    const first = borrowers[0].target.name;
    const last = borrowers[borrowers.length - 1].target.name;
    const more = further > 0 ? `, and ${plural(further, "more code")} descend from it through them` : "";
    parts.push(`${num(borrowers.length)} later codes${by} borrowed from it, from ${first} to ${last}${more}.`);
  }
  return parts.join(" ");
}

const esc = (s) => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);

// Chart geometry and colours. The edge grey is the muted ink of the shared
// palette; an emphasised edge takes the ink itself.
const RADIUS = 7; // of a node's circle
const COLLIDE = 33; // the radius a node keeps clear of other nodes' centres
const LABEL_LEFT = RADIUS + 2; // a node's reach left of its centre, its circle ...
const LABEL_RIGHT = 58; // ... and right, the label beside the circle
const LABEL_H = 13; // half the label's height
const GAP = 36; // between the main network and the clusters, and between clusters
const MIN_SCALE = 0.75; // the frame never shrinks the network below this: labels must stay legible
const EDGE = colors.muted;
const EDGE_OPACITY = 0.5;
const FADE_NODE = 0.2;
const FADE_LINK = 0.06;

// legend returns the swatches of the four categories, for the page to set
// above the chart, as Plot sets its legends.
export function legend() {
  const div = document.createElement("div");
  div.className = "viz-legend";
  div.setAttribute("aria-label", "Legend");
  for (const c of CATEGORIES) {
    const item = document.createElement("span");
    item.className = "viz-swatch";
    const dot = document.createElement("span");
    dot.className = "viz-swatch-dot";
    dot.style.background = borrowingColors[c];
    item.append(dot, c);
    div.appendChild(item);
  }
  return div;
}

// network draws the force layout of the whole graph at the given size and
// returns a controller: `node`, the element to put on the page; `update(summary)`,
// which shows the codes the summary makes visible, in place, and traces the
// selected code's lineage; `resize(width, height)`; `fit()`, which frames the
// whole network; and `reveal(code)`, which pans to a code outside the frame.
// `onSelect(code | null)` is called when the reader clicks a node, presses
// Enter or Space on one, presses Escape, or clicks the background.
//
// The layout is computed once, by `layout`, before anything is drawn, so
// that the page does not animate into place; it is the same on every visit.
// A simulation anchored to that layout stays alive only for dragging, so that
// a dragged node's neighbours give way and the rest holds still, and not even
// for that when the reader asks for reduced motion.
export function network(g, { width, height, onSelect = () => {} }) {
  const { nodes, links } = g;
  const widthScale = d3.scaleSqrt().domain([1, g.maxSections]).range([1, 4]);

  layout(g);
  const simulation = d3.forceSimulation(nodes)
    .randomSource(d3.randomLcg(0.117))
    .force("link", d3.forceLink(links).distance(70).strength(0.5))
    .force("charge", d3.forceManyBody().strength(-120).distanceMax(200))
    .force("collide", d3.forceCollide(COLLIDE))
    .force("x", d3.forceX((d) => d.x0).strength(0.12))
    .force("y", d3.forceY((d) => d.y0).strength(0.12))
    .alpha(0)
    .stop();

  const wrapper = document.createElement("div");
  wrapper.className = "viz-network-wrap";
  const svg = d3.select(wrapper).append("svg")
    .attr("class", "viz-network")
    .attr("role", "group")
    .attr("aria-labelledby", "fc-title")
    .attr("aria-describedby", "fc-desc")
    .attr("font-family", fonts.sans)
    .attr("font-size", sizes.label);
  const titleEl = svg.append("title").attr("id", "fc-title");
  const descEl = svg.append("desc").attr("id", "fc-desc");

  const defs = svg.append("defs");
  const marker = (id, fill) => defs.append("marker")
    .attr("id", id).attr("viewBox", "0 -4 8 8").attr("refX", 8).attr("refY", 0)
    .attr("markerWidth", 8).attr("markerHeight", 8).attr("markerUnits", "userSpaceOnUse").attr("orient", "auto")
    .append("path").attr("d", "M0,-4L8,0L0,4Z").attr("fill", fill);
  marker("fc-arrow", EDGE);
  marker("fc-arrow-hi", colors.ink);

  const background = svg.append("rect").attr("fill", "transparent").on("click", () => onSelect(null));
  const view = svg.append("g");

  const link = view.append("g").attr("fill", "none").selectAll("line").data(links).join("line")
    .attr("stroke", EDGE)
    .attr("stroke-width", (d) => widthScale(d.sections_borrowed))
    .attr("marker-end", "url(#fc-arrow)")
    .attr("opacity", EDGE_OPACITY);

  const node = view.append("g").selectAll("g").data(nodes).join("g")
    .attr("class", "node")
    .attr("tabindex", 0)
    .attr("role", "button")
    .attr("aria-label", ariaLabel);
  node.append("circle")
    .attr("r", RADIUS)
    .attr("fill", (d) => borrowingColors[d.category])
    .attr("stroke", colors.surface)
    .attr("stroke-width", 1.5);
  node.append("text")
    .text((d) => d.code)
    .attr("x", RADIUS + 3)
    .attr("dy", "0.35em")
    .attr("font-family", fonts.sans)
    .attr("font-size", sizes.label)
    .attr("font-weight", 500)
    .attr("fill", colors.ink)
    .attr("paint-order", "stroke")
    .attr("stroke", colors.surface)
    .attr("stroke-width", 2.5)
    .attr("stroke-linejoin", "round")
    .attr("pointer-events", "none")
    .style("font-variant-numeric", "tabular-nums");

  const tip = document.createElement("div");
  tip.className = "viz-tooltip";
  tip.hidden = true;
  wrapper.appendChild(tip);

  let summary = null;
  let hovered = null;
  let transform = d3.zoomIdentity;
  let W = width;
  let H = height;

  // end is where a link's line stops, at the edge of the borrower's circle,
  // so that the arrowhead touches it.
  function end(d) {
    const dx = d.target.x - d.source.x;
    const dy = d.target.y - d.source.y;
    const len = Math.hypot(dx, dy) || 1;
    const k = (len - RADIUS - 1) / len;
    return [d.source.x + dx * k, d.source.y + dy * k];
  }

  function place() {
    link.attr("x1", (d) => d.source.x).attr("y1", (d) => d.source.y)
      .attr("x2", (d) => end(d)[0]).attr("y2", (d) => end(d)[1]);
    node.attr("transform", (d) => `translate(${d.x},${d.y})`);
    if (hovered) positionTip(hovered);
  }

  const zoom = d3.zoom().scaleExtent([0.25, 4]).on("zoom", (ev) => {
    transform = ev.transform;
    view.attr("transform", transform);
    if (hovered) positionTip(hovered);
  });
  svg.call(zoom).on("dblclick.zoom", null);

  // fit frames the codes the summary shows (the page calls it on load, on
  // resize, when the codes outside the network are hidden or shown, and on
  // request, but not when the years change, so that the frame holds still
  // then), never shrinking them below MIN_SCALE: a network that overflows the
  // frame and can be panned beats one whose labels cannot be read.
  function fit() {
    const shown = summary ? summary.visible : nodes;
    if (shown.length === 0) return;
    const [x0, x1] = d3.extent(shown, (d) => d.x);
    const [y0, y1] = d3.extent(shown, (d) => d.y);
    const left = x0 - LABEL_LEFT;
    const right = x1 + LABEL_RIGHT;
    const top = y0 - LABEL_H;
    const bottom = y1 + LABEL_H;
    const pad = 24;
    const k = Math.max(MIN_SCALE, Math.min(1.5, (W - 2 * pad) / (right - left), (H - 2 * pad) / (bottom - top)));
    const t = d3.zoomIdentity.translate(W / 2 - (k * (left + right)) / 2, H / 2 - (k * (top + bottom)) / 2).scale(k);
    svg.call(zoom.transform, t);
  }

  // reveal pans to a code if it is outside the frame, keeping the scale.
  function reveal(code) {
    const d = g.byCode.get(code);
    if (!d) return;
    const [x, y] = transform.apply([d.x, d.y]);
    if (x >= 0 && x <= W && y >= 0 && y <= H) return;
    const t = d3.zoomIdentity.translate(W / 2 - transform.k * d.x, H / 2 - transform.k * d.y).scale(transform.k);
    svg.call(zoom.transform, t);
  }

  node.call(d3.drag().clickDistance(4)
    .on("start", (ev, d) => {
      if (!ev.active && !reducedMotion()) simulation.alphaTarget(0.15).restart();
      d.fx = d.x;
      d.fy = d.y;
      svg.classed("grabbing", true);
    })
    .on("drag", (ev, d) => {
      d.fx = ev.x;
      d.fy = ev.y;
      if (reducedMotion()) { d.x = ev.x; d.y = ev.y; place(); }
    })
    .on("end", (ev, d) => {
      if (!ev.active) simulation.alphaTarget(0);
      d.fx = null;
      d.fy = null;
      svg.classed("grabbing", false);
    }));
  simulation.on("tick", place);

  const toggle = (d) => onSelect(summary && summary.selected === d ? null : d.code);
  node.on("pointerenter", (ev, d) => hover(d))
    .on("pointerleave", () => hover(null))
    .on("focus", (ev, d) => hover(d))
    .on("blur", () => hover(null))
    .on("click", (ev, d) => { ev.stopPropagation(); toggle(d); })
    .on("keydown", (ev, d) => {
      if (ev.key === "Enter" || ev.key === " ") { ev.preventDefault(); toggle(d); }
      else if (ev.key === "Escape") onSelect(null);
    });

  function hover(d) {
    hovered = d;
    restyle();
    if (d) {
      tip.innerHTML = tooltip(d);
      tip.hidden = false;
      positionTip(d);
    } else {
      tip.hidden = true;
    }
  }

  function positionTip(d) {
    const [x, y] = transform.apply([d.x, d.y]);
    const tw = tip.offsetWidth;
    const th = tip.offsetHeight;
    let left = x + 14;
    let top = y - 12;
    if (left + tw > W - 8) left = x - tw - 14;
    if (top + th > H - 8) top = H - th - 8;
    tip.style.left = `${Math.max(8, left)}px`;
    tip.style.top = `${Math.max(8, top)}px`;
  }

  // visible is whether the summary shows a node; before the first summary,
  // everything is shown.
  const visible = (d) => !summary || summary.visibleSet.has(d);

  // restyle applies the summary and the hover: hidden codes, the emphasised
  // codes (the lineage of the selection, else the hovered code and its
  // neighbours) with the rest faded, and the ring on the selected code.
  function restyle() {
    const lin = summary ? summary.lineage : null;
    const sel = summary ? summary.selected : null;
    let emph = null;
    let emphLinks = null;
    if (lin) {
      emph = new Set(lin.nodes);
      emphLinks = lin.links;
      if (hovered) emph.add(hovered);
    } else if (hovered) {
      emph = new Set([hovered]);
      emphLinks = new Set();
      for (const l of hovered.sources) { emph.add(l.source); emphLinks.add(l); }
      for (const l of hovered.borrowers) { emph.add(l.target); emphLinks.add(l); }
    }
    node.attr("display", (d) => (visible(d) ? null : "none"))
      .attr("opacity", (d) => (emph && !emph.has(d) ? FADE_NODE : 1))
      .attr("aria-pressed", (d) => (sel ? String(d === sel) : null));
    node.select("circle")
      .attr("stroke", (d) => (d === sel ? colors.ink : colors.surface))
      .attr("stroke-width", (d) => (d === sel ? 3 : 1.5));
    link.attr("display", (d) => (visible(d.source) && visible(d.target) ? null : "none"))
      .attr("opacity", (d) => (emphLinks ? (emphLinks.has(d) ? 0.9 : FADE_LINK) : EDGE_OPACITY))
      .attr("stroke", (d) => (emphLinks && emphLinks.has(d) ? colors.ink : EDGE))
      .attr("marker-end", (d) => (emphLinks && emphLinks.has(d) ? "url(#fc-arrow-hi)" : "url(#fc-arrow)"));
  }

  function ariaLabel(d) {
    return `${d.name}, ${d.code}: ${d.category.toLowerCase()}; ${plural(d.sections, "section")}; borrowed from ${plural(d.sources.length, "code")}, borrowed by ${plural(d.borrowers.length, "code")}`;
  }

  // tooltip is the text shown beside a hovered or focused code, counting
  // only the codes the summary shows.
  function tooltip(d) {
    const sources = d.sources.filter((l) => visible(l.source));
    const borrowers = d.borrowers.filter((l) => visible(l.target));
    const corpus = d.corpus_code !== d.code ? `, ${esc(d.corpus_code)} in the corpus` : "";
    const steps = d.distance > 1 ? `, ${d.distance} steps from a Field code` : "";
    const from = sources.length
      ? `Borrowed from ${sources.map((l) => `${esc(l.source.code)} (${fmt(l.sections_borrowed)} sections, ${pct(l.sections_borrowed, d.sections)})`).join(" and ")}`
      : `<span class="viz-tip-muted">No source among the codes shown</span>`;
    let by = `<span class="viz-tip-muted">No later code shown borrowed from it</span>`;
    if (borrowers.length === 1) by = `Borrowed by ${esc(borrowers[0].target.code)}`;
    else if (borrowers.length > 1) by = `Borrowed by ${fmt(borrowers.length)} codes, ${esc(borrowers[0].target.code)} to ${esc(borrowers[borrowers.length - 1].target.code)}`;
    const hint = summary && summary.selected === d ? "Click to release it" : "Click to trace its line of descent";
    return [
      `<div class="viz-tip-title">${esc(d.name)} <span class="viz-tip-muted">${esc(d.code)}${corpus}</span></div>`,
      `<div>${esc(d.category)}${steps} · ${fmt(d.sections)} sections, ${fmt(d.sections_matched)} matched to an earlier code</div>`,
      `<div>${from}</div>`,
      `<div>${by}</div>`,
      `<div class="viz-tip-muted">${hint}</div>`,
    ].join("");
  }

  function update(s) {
    summary = s;
    titleEl.text(s.title);
    descEl.text(s.subtitle.replace(/<[^>]+>/g, ""));
    restyle();
    if (hovered) tip.innerHTML = tooltip(hovered);
  }

  function resize(w, h) {
    W = w;
    H = h;
    svg.attr("width", w).attr("height", h).attr("viewBox", [0, 0, w, h]);
    background.attr("width", w).attr("height", h);
    fit();
  }

  place();
  restyle();
  resize(width, height);
  return { node: wrapper, update, resize, fit, reveal };
}

// layout places every node once, deterministically. The Field Code's network,
// where nearly everything is, is laid out by a force simulation pulled a
// little flatter than it is wide, since the frame is. The clusters with no
// tie to it, which a single simulation would scatter around the outside and
// so waste the frame, are each laid out alone and then stacked in columns to
// the right and left of the network, tallest first, as the figure set them at
// its edges. Each node keeps its place as `x0`, `y0`, the anchor the
// drag-time simulation pulls it back toward.
function layout(g) {
  const main = g.nodes.filter((n) => n.connected);
  simulate(main, g.links.filter((l) => l.source.connected),
    { distance: 70, charge: -220, distanceMax: 400, collide: COLLIDE, x: 0.08, y: 0.2 }, 300);

  const clusters = components(g.nodes.filter((n) => !n.connected), g.links.filter((l) => !l.source.connected));
  for (const c of clusters) {
    simulate(c.nodes, c.links, { distance: 50, charge: -80, distanceMax: 150, collide: COLLIDE - 3, x: 0.4, y: 0.4 }, 200);
    const [x0, x1] = d3.extent(c.nodes, (n) => n.x);
    const [y0, y1] = d3.extent(c.nodes, (n) => n.y);
    const left = x0 - LABEL_LEFT;
    const right = x1 + LABEL_RIGHT;
    Object.assign(c, { cx: (left + right) / 2, cy: (y0 + y1) / 2, w: right - left, h: y1 - y0 + 2 * LABEL_H });
  }
  clusters.sort((a, b) => b.h - a.h || b.w - a.w || a.nodes[0].code.localeCompare(b.nodes[0].code));

  // Fill columns as tall as the network, the first to its right, the second
  // to its left, and so on outward.
  const [mx0, mx1] = d3.extent(main, (n) => n.x);
  const [my0, my1] = d3.extent(main, (n) => n.y);
  const capacity = Math.max(my1 - my0 + 2 * LABEL_H, 300);
  const columns = [];
  for (const c of clusters) {
    let col = columns.find((k) => k.height + c.h + (k.items.length ? GAP / 2 : 0) <= capacity);
    if (!col) {
      const i = columns.length;
      col = { side: i % 2 ? -1 : 1, rank: i >> 1, items: [], height: 0, width: 0 };
      columns.push(col);
    }
    col.height += c.h + (col.items.length ? GAP / 2 : 0);
    col.width = Math.max(col.width, c.w);
    col.items.push(c);
  }
  for (const side of [1, -1]) {
    let edge = side === 1 ? mx1 + LABEL_RIGHT : mx0 - LABEL_LEFT;
    for (const col of columns.filter((k) => k.side === side).sort((a, b) => a.rank - b.rank)) {
      const cx = edge + side * (GAP + col.width / 2);
      let y = (my0 + my1) / 2 - col.height / 2;
      for (const c of col.items) {
        const dx = cx - c.cx;
        const dy = y + c.h / 2 - c.cy;
        for (const n of c.nodes) { n.x += dx; n.y += dy; }
        y += c.h + GAP / 2;
      }
      edge = cx + side * (col.width / 2);
    }
  }
  for (const n of g.nodes) { n.x0 = n.x; n.y0 = n.y; }
}

// simulate runs a force layout of some nodes and the links among them to
// rest, about the origin, with a seeded source of randomness so that it comes
// out the same every time.
function simulate(nodes, links, p, ticks) {
  d3.forceSimulation(nodes)
    .randomSource(d3.randomLcg(0.117))
    .force("link", d3.forceLink(links).distance(p.distance).strength(0.7))
    .force("charge", d3.forceManyBody().strength(p.charge).distanceMax(p.distanceMax))
    .force("collide", d3.forceCollide(p.collide).iterations(2))
    .force("x", d3.forceX().strength(p.x))
    .force("y", d3.forceY().strength(p.y))
    .stop()
    .tick(ticks);
}

// components splits nodes into the groups the links connect, each with its
// nodes and its links.
function components(nodes, links) {
  const parent = new Map(nodes.map((n) => [n, n]));
  const find = (n) => {
    while (parent.get(n) !== n) {
      parent.set(n, parent.get(parent.get(n)));
      n = parent.get(n);
    }
    return n;
  };
  for (const l of links) parent.set(find(l.source), find(l.target));
  const groups = new Map();
  for (const n of nodes) {
    const r = find(n);
    if (!groups.has(r)) groups.set(r, { nodes: [], links: [] });
    groups.get(r).nodes.push(n);
  }
  for (const l of links) groups.get(find(l.source)).links.push(l);
  return [...groups.values()];
}

// table lays the visible codes out as an HTML table, the accessible form of
// the chart: each code with its category, sections, sources and borrowers.
export function table(summary) {
  const t = document.createElement("table");
  t.className = "viz-table";
  t.innerHTML = `<caption class="visually-hidden">${esc(summary.title)}</caption>
    <thead><tr><th scope="col">Code</th><th scope="col">Jurisdiction</th><th scope="col" class="num">Year</th><th scope="col">Category</th><th scope="col" class="num">Sections</th><th scope="col" class="num">Matched</th><th scope="col">Borrowed from (sections, share of its sections)</th><th scope="col">Borrowed by</th></tr></thead>`;
  const body = document.createElement("tbody");
  for (const d of summary.visible) {
    const from = d.sources.filter((l) => summary.visibleSet.has(l.source))
      .map((l) => `${esc(l.source.code)}: ${fmt(l.sections_borrowed)} (${pct(l.sections_borrowed, d.sections)})`).join("; ") || "—";
    const by = d.borrowers.filter((l) => summary.visibleSet.has(l.target)).map((l) => esc(l.target.code)).join(", ") || "—";
    const tr = document.createElement("tr");
    tr.innerHTML = `<td>${esc(d.code)}</td><td>${esc(d.jurisdiction)}</td><td class="num">${d.year}</td><td>${esc(d.category)}</td><td class="num">${fmt(d.sections)}</td><td class="num">${fmt(d.sections_matched)}</td><td>${from}</td><td>${by}</td>`;
    body.appendChild(tr);
  }
  t.appendChild(body);
  return t;
}

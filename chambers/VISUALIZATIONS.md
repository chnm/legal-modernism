# Visualizations for public consumption

This is the specification for visualizations meant to be read by people outside
the project: in articles, talks, and eventually the project website. It does not
govern chambers' internal dashboards (`/linking`, `/linking/tiers`, the case and
work pages), which can stay quick and utilitarian, though they may borrow from it.

## Where they are built

For now, public visualizations are developed inside chambers, which serves both
the page and its data. Eventually they may move to the Hugo website as
JavaScript that calls the project's data through [Apiary](https://github.com/chnm/apiary),
the RRCHNM API. Write them so that the move is cheap:

- The chart code should depend only on a JSON array fetched from a URL. Don't let
  it read Go template values, other than the URL itself.
- Keep everything that sets the look and feel (fonts, sizes, colours, margins) in
  one shared module rather than in each chart, so the Hugo site can import the
  same module.
- Keep the code for each chart in its own module, with a function that takes the
  data and the options and returns a node.

Conventions in chambers:

- Page: `/viz/<name>`, template `templates/viz_<name>.html`.
- Data: `/api/viz/<name>`, written with `writeJSON` (see `linking.go`), following
  the 503 convention of `apiError` for a view that is not populated yet.
- Shared style module: `static/viz/style.js`. It exports the typefaces, the
  colours (the registry below), the type sizes, the `plotStyle` every
  `Plot.plot` call takes, a stylesheet for the anatomy and controls (classes
  `viz-title`, `viz-subtitle`, `viz-source`, `viz-notes`, `viz-controls`, the
  toggles and the two-handled year slider), and the number formats.
  `ensureStyles()` loads the fonts and the stylesheet once.
- Chart module: `static/viz/<name>.js`, with a `summarize(data, options)` that
  reduces the JSON to what the chart shows and the words for the title and
  subtitle, a `chart(summary, {width, height})` that returns Plot's figure, and
  a `table(summary)` for the accessible table.
- Add each visualization to the "Draft public visualizations" section of
  `home.html`; the navbar's "Draft visualizations" links there. The page's
  `Section` is `viz`.
- Keep the view's state (the selection, the years) in the query string, so a
  view can be shared.

## Before starting one

Ask whether the visualization is **full screen** (the chart is the page, sized to
the window) or **in the viewport** (constrained to the normal width of a page
of prose, like a figure in an article). Don't start until that is settled.

## Library

Use [Observable Plot](https://observablehq.com/plot/). Import it the way the
chambers pages already do:

```js
import * as Plot from "https://esm.sh/@observablehq/plot@0.6?bundle-deps";
```

Use D3 only where Plot can't do what is required (a force layout, a custom
projection, an interaction Plot doesn't support). Plot's output is SVG, so mixing
D3 into a Plot chart is acceptable when a chart needs a single D3 addition. A
chart that is all D3, like the Field Code network, imports it the same way:

```js
import * as d3 from "https://esm.sh/d3@7.9.0?bundle-deps";
```

## Data

Visualizations consume JSON: a plain array of flat objects, one per mark
(`[{"year": 1850, "jurisdiction": "US", "citations": 1234}, …]`). Field names
are `snake_case`, matching the database columns. Do the aggregation in SQL (a
query or a materialized view), not in the browser; the browser should only
reshape the data for Plot. The title, subtitle, source and notes are part of the
page, not the data.

Data that is not in the database yet may be served from JSON files embedded in
chambers (`data/`, served by `serveData` at the same `/api/viz/<name>/...` URLs),
derived by a program under `scripts/` that documents its source, so that the
chart does not change when the data moves. The Field Code network
(`/viz/field-code`, issue #117) is the first: two arrays, the codes and the
borrowings, from the compendium of Funk and Mullen's "The Spine of American
Law".

## Anatomy

Every public visualization has, in this order:

1. **Title** — what the chart shows, in a few words.
2. **Subtitle** — the finding or the claim the chart supports, in a sentence.
3. **The chart**, with its legend if it needs one.
4. **Source** — where the data comes from (e.g. "Making of Modern Law; Caselaw
   Access Project; Legal Modernism citation linker").
5. **Notes** — anything a careful reader needs: what was counted, what was left
   out, the date range, caveats.

There is no link to download the data.

## Look and feel

- **Typeface:** IBM Plex Sans, the face of both the website and chambers, for
  titles, labels, and annotations. Use IBM Plex Mono, or Plex Sans with
  `font-variant-numeric: tabular-nums`, for numbers on axes and in tooltips so
  that they align. Load both from Google Fonts.
- **Sizes:** title larger and bold (700), subtitle at body size in regular weight,
  axis labels and source/notes smaller and muted. Define these once in the shared
  style module.
- **Colour:** use the D3 and Observable colour schemes (`d3-scale-chromatic`,
  which Plot exposes via `scheme`): a categorical scheme such as `observable10` or
  `tableau10` for categories, a sequential scheme for quantities, and a diverging
  scheme for quantities with a meaningful midpoint. The website's accent is
  purple (`#ba02e8`), which chambers shares; it is not yet fixed for any category in charts.
- **Consistency:** once a category has a colour, it keeps it in every chart.
  Record each decision in the colour registry below.
- **Jurisdictions:** a state is coloured by its region, of which there are six,
  a hodgepodge of the Census Bureau's regions and divisions: New England and
  the Mid-Atlantic (the Northeast's two divisions), the Midwest and the South
  (each Census region whole), and the Mountain West and the Pacific West (the
  West's two divisions). Each has one colour of `observable10`, the two
  Northeast groups the scheme's two blues. The District of Columbia is in the
  South, as the Bureau has it; Dakota Territory goes with the Dakotas. The
  federal courts, the United Kingdom, and the territories and tribal courts are
  outside the regions and have colours of their own from the same scheme; the
  federal courts and the United Kingdom, often the longest bars, take its two
  most vivid, yellow and red-orange. The
  regions and the map from `cap.jurisdictions` slugs are in
  `static/viz/census.js`, the colours in `style.js` under `groupColors`; a
  chart that colours jurisdictions imports both rather than defining its own.
  Pair the colour with the region's name in the tooltip or a table column:
  nine colours do not all survive colour blindness.
- **Restraint:** light grid lines or none, no chart borders, no 3-D, no
  gradients that encode nothing. Label lines and series directly where that is
  clearer than a legend.
- **Dark mode:** not decided. Don't build for it yet, but keep colours in the
  shared module so it can be added later.

## Interactivity

Add interactivity when it helps a reader: tooltips (Plot's `tip` mark) on dense
charts where values can't be labelled, hover highlighting of one series among
many, filters or toggles where the reader needs to compare subsets. A chart
whose point is clear without interaction should not have it. Every interaction
must be usable by keyboard as well as mouse and touch.

## Accessibility

- Give the SVG an `ariaLabel` and `ariaDescription` (Plot options) that say what
  the chart shows and its main finding, not just its type.
- Provide the data behind the chart as an accessible table, hidden visually
  (Bootstrap's `visually-hidden`) or behind a "Show the data" disclosure.
- Never encode a distinction by colour alone; pair it with position, a direct
  label, shape, or dash pattern. Check that the colours are distinguishable with
  the common forms of colour blindness.
- Text and marks meet WCAG AA contrast against the background.
- Respect `prefers-reduced-motion`: no animated transitions when it is set.

## Size

Target tablets and larger: the chart must work at 768px wide and up. Supporting
phones is welcome but not required. Make charts responsive to their container's
width (re-render on resize) rather than fixing a pixel width. A full-screen chart
fills the window; a viewport chart fills the page's content column.

## Export

Not required yet. SVG export may be wanted later; since Plot renders SVG, keep
all styling as attributes or inline styles on the SVG rather than page CSS, so
that an exported SVG looks like the chart on screen.

## Colour registry

The colours assigned to particular categories, so that they stay the same from
chart to chart. Add a row whenever a visualization fixes one.

| Category | Colour | Scheme / source | First used in |
| -------- | ------ | --------------- | ------------- |
| New England | `#4269d0` | `observable10` blue | `/viz/jurisdictions` |
| Mid-Atlantic | `#97bbf5` | `observable10` light blue | `/viz/jurisdictions` |
| Midwest | `#3ca951` | `observable10` green | `/viz/jurisdictions` |
| South | `#9c6b4e` | `observable10` brown | `/viz/jurisdictions` |
| Mountain West | `#a463f2` | `observable10` purple | `/viz/jurisdictions` |
| Pacific West | `#6cc5b0` | `observable10` teal | `/viz/jurisdictions` |
| The federal courts of the United States | `#efb118` | `observable10` yellow | `/viz/jurisdictions` |
| The United Kingdom | `#ff725c` | `observable10` red-orange | `/viz/jurisdictions` |
| Territories and tribal courts | `#9498a0` | `observable10` grey | `/viz/jurisdictions` |
| Field code (a draft of New York's Field Code) | `#e41a1c` | `schemeSet1` red, as in the article's figure 5 | `/viz/field-code` |
| Original borrower (one step from a Field code) | `#377eb8` | `schemeSet1` blue | `/viz/field-code` |
| Subsequent borrower (two or more steps) | `#4daf4a` | `schemeSet1` green | `/viz/field-code` |
| Did not borrow (no step reaches a Field code) | `#984ea3` | `schemeSet1` purple | `/viz/field-code` |

Only the pink (`#ff8ab7`) of `observable10` is still free. The lighter
colours (`#97bbf5`, `#6cc5b0`, `#ff725c`, `#efb118`, `#9498a0`) are below 3:1
against white, so a mark in one always carries a direct label. The four
colours of the Field Code network are the ones the published figure used,
kept so that the chart and the figure read as one; they colour categories
of a code's descent, which no other chart shows, and sit near the
`observable10` blue, green and purple of New England, the Midwest and the
Mountain West, so a chart that shows both regions and descent will need one
of the two recoloured. In `style.js` they are `borrowingColors`. A code the
figure left out is a hollow ring in the purple of "Did not borrow": a
distinction by shape, not by a fifth colour.

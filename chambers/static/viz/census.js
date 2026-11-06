// The regions the public visualizations group the states by
// (VISUALIZATIONS.md, "Colour"): a hodgepodge of the Census Bureau's regions
// and divisions. The Northeast keeps its two divisions, New England and the
// Mid-Atlantic; the Midwest and the South are each one group; the West is
// split into its two divisions, the Mountain West and the Pacific West. The
// fifty states and the District of Columbia each belong to one group, keyed
// here by cap.jurisdictions slug. The jurisdictions outside the Census
// geography have groups of their own: the federal courts, the United Kingdom,
// and the territories and tribal courts. Dakota Territory is placed with the
// two states made from it.
//
// `census` names the Census region a group draws its hue from, so that the
// two Northeast groups are blues and the two West groups purples. The colours
// are in style.js, keyed by the group's key, so that a group has one colour in
// every chart.

// GROUPS is every group a jurisdiction can belong to, in the order a legend
// lists them: the six regions from northeast to west, then the three groups
// outside them, whose `census` is null.
export const GROUPS = [
  { key: "new-england", name: "New England", census: "Northeast" },
  { key: "mid-atlantic", name: "Mid-Atlantic", census: "Northeast" },
  { key: "midwest", name: "Midwest", census: "Midwest" },
  { key: "south", name: "South", census: "South" },
  { key: "mountain-west", name: "Mountain West", census: "West" },
  { key: "pacific-west", name: "Pacific West", census: "West" },
  { key: "federal", name: "Federal courts", census: null },
  { key: "uk", name: "United Kingdom", census: null },
  { key: "territories", name: "Territories and tribal courts", census: null },
];

const byKey = new Map(GROUPS.map((g) => [g.key, g]));

// GROUP_OF maps a cap.jurisdictions slug to its group's key.
const GROUP_OF = new Map(
  Object.entries({
    "new-england": ["conn", "me", "mass", "nh", "ri", "vt"],
    "mid-atlantic": ["nj", "ny", "pa"],
    midwest: [
      "ill", "ind", "mich", "ohio", "wis", // East North Central
      "iowa", "kan", "minn", "mo", "neb", "nd", "sd", "dakota-territory", // West North Central
    ],
    south: [
      "del", "dc", "fla", "ga", "md", "nc", "sc", "va", "w-va", // South Atlantic
      "ala", "ky", "miss", "tenn", // East South Central
      "ark", "la", "okla", "tex", // West South Central
    ],
    "mountain-west": ["ariz", "colo", "idaho", "mont", "nev", "nm", "utah", "wyo"],
    "pacific-west": ["alaska", "cal", "haw", "or", "wash"],
    federal: ["us"],
    uk: ["uk"],
  }).flatMap(([key, slugs]) => slugs.map((slug) => [slug, key])),
);

// groupOf returns the group of a jurisdiction. A slug the map does not know
// is a territory or a tribal court (am-samoa, guam, n-mar-i, navajo-nation,
// pr, tribal, vi), and so is anything CAP adds later.
export function groupOf(slug) {
  return byKey.get(GROUP_OF.get(slug) || "territories");
}

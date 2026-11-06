// The Census Bureau's regions and divisions, which the public visualizations
// use to group the states (VISUALIZATIONS.md, "Colour"). The four regions
// each hold two or three divisions; the fifty states and the District of
// Columbia each belong to one division. The map is keyed by cap.jurisdictions
// slug. The jurisdictions outside the Census geography have groups of their
// own: the federal courts, the United Kingdom, and the territories and tribal
// courts. Dakota Territory is placed with the two states made from it.
//
// The colours are in style.js, keyed by the group's key, so that a division
// has one colour in every chart.

export const REGIONS = {
  northeast: "Northeast",
  midwest: "Midwest",
  south: "South",
  west: "West",
};

// GROUPS is every group a jurisdiction can belong to, in the order a legend
// lists them: the divisions by region as the Census Bureau numbers them, then
// the three groups outside the regions. `region` is null for those.
export const GROUPS = [
  { key: "new-england", name: "New England", region: "northeast" },
  { key: "middle-atlantic", name: "Middle Atlantic", region: "northeast" },
  { key: "east-north-central", name: "East North Central", region: "midwest" },
  { key: "west-north-central", name: "West North Central", region: "midwest" },
  { key: "south-atlantic", name: "South Atlantic", region: "south" },
  { key: "east-south-central", name: "East South Central", region: "south" },
  { key: "west-south-central", name: "West South Central", region: "south" },
  { key: "mountain", name: "Mountain", region: "west" },
  { key: "pacific", name: "Pacific", region: "west" },
  { key: "federal", name: "Federal courts", region: null },
  { key: "uk", name: "United Kingdom", region: null },
  { key: "territories", name: "Territories and tribal courts", region: null },
];

const byKey = new Map(GROUPS.map((g) => [g.key, g]));

// DIVISION_OF maps a cap.jurisdictions slug to its group's key.
const DIVISION_OF = new Map(
  Object.entries({
    "new-england": ["conn", "me", "mass", "nh", "ri", "vt"],
    "middle-atlantic": ["nj", "ny", "pa"],
    "east-north-central": ["ill", "ind", "mich", "ohio", "wis"],
    "west-north-central": ["iowa", "kan", "minn", "mo", "neb", "nd", "sd", "dakota-territory"],
    "south-atlantic": ["del", "dc", "fla", "ga", "md", "nc", "sc", "va", "w-va"],
    "east-south-central": ["ala", "ky", "miss", "tenn"],
    "west-south-central": ["ark", "la", "okla", "tex"],
    mountain: ["ariz", "colo", "idaho", "mont", "nev", "nm", "utah", "wyo"],
    pacific: ["alaska", "cal", "haw", "or", "wash"],
    federal: ["us"],
    uk: ["uk"],
  }).flatMap(([key, slugs]) => slugs.map((slug) => [slug, key])),
);

// groupOf returns the group of a jurisdiction. A slug the map does not know
// is a territory or a tribal court (am-samoa, guam, n-mar-i, navajo-nation,
// pr, tribal, vi), and so is anything CAP adds later.
export function groupOf(slug) {
  return byKey.get(DIVISION_OF.get(slug) || "territories");
}

// regionOf returns the region's name of a group, or null outside the regions.
export const regionOf = (group) => (group.region ? REGIONS[group.region] : null);

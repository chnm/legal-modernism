package main

import (
	"net/url"
	"strconv"
	"strings"
)

// Every URL chambers builds for itself is made here, so a route can change in
// one place.

func workURL(id int) string { return "/works/" + strconv.Itoa(id) }

func editionURL(bibliographicid string) string {
	return "/editions/" + url.PathEscape(bibliographicid)
}

func editionCasesURL(bibliographicid string) string {
	return editionURL(bibliographicid) + "/cases"
}

func pageURL(psmid, pageid string) string {
	return "/pages/" + url.PathEscape(psmid) + "/" + url.PathEscape(pageid)
}

// caseURL addresses a case by source and id. A stub case's id is its cite
// string, spaces and brackets included, so the segment is escaped.
func caseURL(source, id string) string {
	return "/cases/" + url.PathEscape(source) + "/" + url.PathEscape(id)
}

func reporterURL(standard string) string {
	return "/reporters/" + url.PathEscape(standard)
}

func citationURL(id string) string { return "/citations/" + url.PathEscape(id) }

// citationsURL links to the citation list filtered by the given parameters.
func citationsURL(v url.Values) string {
	if len(v) == 0 {
		return "/citations"
	}
	return "/citations?" + v.Encode()
}

// GaleLinks are the URLs of a MOML volume or page on Gale, routed through the
// two institutions whose proxies the project uses.
type GaleLinks struct {
	GMU      string
	Columbia string
}

// galeLinks builds the Gale links for a volume (pageid empty) or one of its
// pages. Both are empty when the volume has no product link.
func galeLinks(productLink *string, pageid string) GaleLinks {
	return GaleLinks{
		GMU:      momlPageLink(momlVolumeLink(productLink, "https://link.gale.com", "u=viva_gmu&"), pageid),
		Columbia: momlPageLink(momlVolumeLink(productLink, "https://link.gale.com", "u=columbiau&"), pageid),
	}
}

// momlVolumeLink rewrites a stored Gale productlink to the given host and
// inserts the institutional user param (e.g. "u=viva_gmu&"), yielding a MOML
// volume URL routed through that institution's proxy. Returns "" when there is
// no productlink.
func momlVolumeLink(productLink *string, host, userParam string) string {
	if productLink == nil {
		return ""
	}
	u := *productLink
	u = strings.Replace(u, "http://link.galegroup.com", host, 1)
	u = strings.Replace(u, "?sid=dhxml", "?"+userParam+"sid=dhxml", 1)
	return u
}

// momlPageLink appends the page (pg) parameter to a Gale MOML volume URL. The pg
// value is the pageid with its trailing and leading zeros stripped (e.g.
// "06870" -> 687). Returns "" when base is empty.
func momlPageLink(base, pageID string) string {
	if base == "" {
		return ""
	}
	if pageID != "" {
		pg := strings.TrimLeft(strings.TrimRight(pageID, "0"), "0")
		if pg != "" {
			base += "&pg=" + pg
		}
	}
	return base
}

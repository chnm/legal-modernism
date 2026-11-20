package main

import (
	"net/url"
	"strconv"
	"strings"
)

// Every URL chambers builds for itself is made here, so a route can change in
// one place.

func workURL(id int) string { return "/works/" + strconv.Itoa(id) }

// workCasesURL is the matrix of a work's editions and the cases they cite.
func workCasesURL(id int) string { return workURL(id) + "/cases" }

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

// The linking pages of a corpus, and the APIs their scripts fetch. The MOML
// URLs are the section's original ones; the CAP pages live under /linking/cap
// and select their data with corpus=cap.

// TiersURL is the corpus's match-tiers page.
func (c linkingCorpus) TiersURL() string { return c.Base + "/tiers" }

// WhitelistURL is the whitelist extender over the corpus's spellings.
func (c linkingCorpus) WhitelistURL() string { return "/linking/whitelist" + c.query() }

// DashboardAPI, TiersAPI and WhitelistAPI are the JSON behind the three pages.
func (c linkingCorpus) DashboardAPI() string { return "/api/linking-dashboard" + c.query() }
func (c linkingCorpus) TiersAPI() string     { return "/api/tiers" + c.query() }
func (c linkingCorpus) WhitelistAPI() string { return "/api/whitelist-extender" + c.query() }

// query is the corpus parameter, empty for the treatises, the default.
func (c linkingCorpus) query() string {
	if c.Key == momlCorpus.Key {
		return ""
	}
	return "?corpus=" + url.QueryEscape(c.Key)
}

// capGraphCaseURL is the comparison of one citing CAP case's edges with CAP's
// graph.
func capGraphCaseURL(id int64) string {
	return "/linking/cap/cases/" + strconv.FormatInt(id, 10)
}

// capGraphReporterURL is a sample of the edges to a cited CAP reporter that
// one graph has and the other lacks; side is "ours" or "theirs".
func capGraphReporterURL(id int64, side string) string {
	return "/linking/cap/reporters/" + strconv.FormatInt(id, 10) + "?side=" + url.QueryEscape(side)
}

// SourceLinks are the URLs of a volume or page where it was scanned: on Gale
// for a MOML volume, routed through the two institutions whose proxies the
// project uses, or on HathiTrust, which is open to anyone.
type SourceLinks struct {
	GMU        string
	Columbia   string
	HathiTrust string
}

// hathiHandle begins the product link of a volume taken from HathiTrust.
const hathiHandle = "https://hdl.handle.net/2027/"

// sourceLinks builds the links for a volume (pageid empty) or one of its
// pages from the volume's product link. All are empty when it has none.
func sourceLinks(productLink *string, pageid string) SourceLinks {
	if productLink != nil && strings.HasPrefix(*productLink, hathiHandle) {
		return SourceLinks{HathiTrust: hathiPageLink(*productLink, pageid)}
	}
	return SourceLinks{
		GMU:      momlPageLink(momlVolumeLink(productLink, "https://link.gale.com", "u=viva_gmu&"), pageid),
		Columbia: momlPageLink(momlVolumeLink(productLink, "https://link.gale.com", "u=columbiau&"), pageid),
	}
}

// hathiPageLink points a HathiTrust handle at one page. A HathiTrust volume's
// pageid is the page's sequence number times ten ("00450" is sequence 45),
// which the handle takes as urlappend=;seq=45.
func hathiPageLink(handle, pageID string) string {
	seq, err := strconv.Atoi(pageID)
	if err != nil || seq < 10 {
		return handle
	}
	return handle + "?urlappend=%3Bseq=" + strconv.Itoa(seq/10)
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

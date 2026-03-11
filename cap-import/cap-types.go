package main

import (
	"database/sql"
	"encoding/json"
	"time"
)

type Case struct {
	ID               int64           `json:"id"`
	URL              string          `json:"url"`
	Name             string          `json:"name"`
	NameAbbreviation string          `json:"name_abbreviation"`
	DecisionDateRaw  string          `json:"decision_date"`
	DocketNumber     string          `json:"docket_number"`
	FirstPageRaw     string          `json:"first_page"`
	LastPageRaw      string          `json:"last_page"`
	Citations        []Citation      `json:"citations"`
	VolumeRaw        Volume          `json:"volume"`
	Reporter         Reporter        `json:"reporter"`
	Court            Court           `json:"court"`
	Jurisdiction     Jurisdiction    `json:"jurisdiction"`
	FrontEndURL      string          `json:"frontend_url"`
	FrontEndPDFURL   string          `json:"frontend_pdf_url"`
	Analysis         json.RawMessage `json:"analysis"`
	LastUpdated      time.Time       `json:"last_updated"`
	Provenance       json.RawMessage `json:"provenance"`
	Casebody         Casebody        `json:"casebody"`
}

func (c Case) DecisionDate() sql.NullTime {
	return parseDate(c.DecisionDateRaw)
}

func (c Case) FirstPage() sql.NullInt32 {
	return parseInt(c.FirstPageRaw)
}

func (c Case) LastPage() sql.NullInt32 {
	return parseInt(c.LastPageRaw)
}

func (c Case) Imported() time.Time {
	return time.Now()
}

func (c Case) Volume() string {
	return c.VolumeRaw.Barcode
}

func (c Case) Year() sql.NullInt32 {
	if len(c.DecisionDateRaw) < 4 {
		return sql.NullInt32{}
	}
	return parseInt(c.DecisionDateRaw[0:4])
}

type Citation struct {
	Type string `json:"type"`
	Cite string `json:"cite"`
}

type Reporter struct {
	ID int64 `json:"id"`
}

type Court struct {
	URL              string `json:"url"`
	NameAbbreviation string `json:"name_abbreviation"`
	Slug             string `json:"slug"`
	ID               int64  `json:"id"`
	Name             string `json:"name"`
}

type Jurisdiction struct {
	ID          int64  `json:"id"`
	NameLong    string `json:"name_long"`
	URL         string `json:"url"`
	Slug        string `json:"slug"`
	Whitelisted bool   `json:"whitelisted"`
	Name        string `json:"name"`
}

type Volume struct {
	URL          string `json:"url"`
	VolumeNumber string `json:"volume_number"`
	Barcode      string `json:"barcode"`
}

type Opinion struct {
	Type   string          `json:"type"`
	Author json.RawMessage `json:"author"`
	Text   string          `json:"text"`
}

type Casebody struct {
	Data Data `json:"data"`
}

type Data struct {
	Opinions  []Opinion       `json:"opinions"`
	Judges    json.RawMessage `json:"judges"`
	Parties   json.RawMessage `json:"parties"`
	Attorneys json.RawMessage `json:"attorneys"`
}

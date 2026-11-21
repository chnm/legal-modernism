package main

import (
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"
	_ "time/tzdata" // so America/New_York resolves without system zoneinfo
)

type progressRow struct {
	Label string
	Done  int
	Total int
}

func (r progressRow) Pct() float64 {
	if r.Total == 0 {
		return 0
	}
	return 100 * float64(r.Done) / float64(r.Total)
}

var progressPage = template.Must(template.New("p").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta http-equiv="refresh" content="15">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Reading progress</title>
<style>
body{font:15px/1.4 system-ui,sans-serif;max-width:760px;margin:2rem auto;padding:0 16px;color:#222;background:#fff}
.bar{background:#e6e6e6;border-radius:4px;height:2.2rem;overflow:hidden}
.fill{background:#2f6db5;height:100%}
.big{font-size:2rem;font-weight:600;margin:.5rem 0}
table{width:100%;border-collapse:collapse;margin-top:2rem;font-size:13px}
td{padding:3px 6px;border-bottom:1px solid #eee}
.mini{background:#eee;height:8px;width:160px;border-radius:2px;overflow:hidden}
.mini div{background:#2f6db5;height:100%}
</style></head><body>
<h1>{{.Title}}</h1>
<div class="big">{{printf "%.1f" .All.Pct}}%</div>
<div class="bar"><div class="fill" style="width:{{printf "%.2f" .All.Pct}}%"></div></div>
<p>{{.All.Done}} of {{.All.Total}} pages read · {{.Citations}} citations recorded · updated {{.Now}}</p>
<table>{{range .Rows}}<tr><td>{{.Label}}</td><td><div class="mini"><div style="width:{{printf "%.1f" .Pct}}%"></div></div></td><td>{{.Done}} / {{.Total}}</td></tr>{{end}}</table>
</body></html>`))

// newYork is the time zone the progress page reports its time in.
var newYork = func() *time.Location {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.UTC
	}
	return loc
}()

// serve shows a progress bar: the pages of the manifest that have a record,
// re-counted from the results directory on every request.
func serve(manifest, dir, title string, jurisdictions map[string]bool, port int) error {
	pages, err := readManifest(manifest)
	if err != nil {
		return err
	}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			Title     string
			All       progressRow
			Rows      []progressRow
			Citations int
			Now       string
		}{Title: title, Now: time.Now().In(newYork).Format("3:04:05 pm MST")}
		idx := map[string]int{}
		for _, p := range pages {
			if len(jurisdictions) > 0 && !jurisdictions[p.Jurisdiction] {
				continue
			}
			key := p.Bibliographicid
			i, ok := idx[key]
			if !ok {
				i = len(data.Rows)
				idx[key] = i
				data.Rows = append(data.Rows, progressRow{Label: fmt.Sprintf("%d %s (%s)", p.Year, p.Jurisdiction, p.Bibliographicid)})
			}
			data.Rows[i].Total++
			data.All.Total++
			if done(dir, p) {
				data.Rows[i].Done++
				data.All.Done++
				if rec, err := readRecord(resultPath(dir, p.Psmid, p.Pageid)); err == nil {
					data.Citations += len(rec.Citations)
				}
			}
		}
		if err := progressPage.Execute(w, data); err != nil {
			slog.Error("rendering progress", "error", err)
		}
	})
	slog.Info("serving progress", "port", port)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

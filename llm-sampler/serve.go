package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"time"
)

// progress is everything the dashboard shows, recounted from the data
// directory on every request.
type progress struct {
	Pages        int            `json:"pages"`
	PagesWith    int            `json:"pages_with_citations"`
	Editions     int            `json:"editions"`
	Batches      int            `json:"batches"`
	Citations    int            `json:"citations"`
	Linked       int            `json:"linked"`
	Status       map[string]int `json:"status"`
	Relation     map[string]int `json:"relation"`
	PipelineSeen int            `json:"pipeline_detections"`
	Rejected     int            `json:"pipeline_rejected"`
	Rewired      int            `json:"links_differing_from_pipeline"`
	ActiveSecs   float64        `json:"active_seconds"`
	WallSecs     float64        `json:"wall_seconds"`
	Running      int            `json:"running_batch"`
	RunningPages int            `json:"running_pages"`
	Started      string         `json:"started"`
	Recent       []recentPage   `json:"recent"`
}

type recentPage struct {
	Batch        int    `json:"batch"`
	Psmid        string `json:"psmid"`
	Pageid       string `json:"pageid"`
	Title        string `json:"title"`
	Year         int    `json:"year"`
	Jurisdiction string `json:"jurisdiction"`
	Pipeline     int    `json:"pipeline"`
	Citations    int    `json:"citations"`
	Linked       int    `json:"linked"`
	New          int    `json:"new"`
	Corrected    int    `json:"corrected"`
}

func count(dir string) (*progress, error) {
	batches, err := readBatches(dir)
	if err != nil {
		return nil, err
	}
	p := &progress{Status: map[string]int{}, Relation: map[string]int{}, Recent: []recentPage{}}
	now := time.Now()
	editions := map[string]bool{}
	var first, last time.Time
	for _, b := range batches {
		if first.IsZero() {
			first = b.StartedAt
		}
		if b.FinishedAt == nil {
			p.Running, p.RunningPages = b.Batch, len(b.Pages)
			p.ActiveSecs += now.Sub(b.StartedAt).Seconds()
			last = now
			continue
		}
		p.Batches++
		p.ActiveSecs += b.FinishedAt.Sub(b.StartedAt).Seconds()
		last = *b.FinishedAt
		for _, pg := range b.Pages {
			var rec PageRecord
			if err := readJSON(filepath.Join(dir, "results", pg.Psmid, pg.Pageid+".json"), &rec); err != nil {
				continue
			}
			p.Pages++
			editions[rec.Bibliographicid] = true
			p.PipelineSeen += rec.PipelineCount
			p.Rejected += len(rec.PipelineRejected)
			r := recentPage{Batch: b.Batch, Psmid: rec.Psmid, Pageid: rec.Pageid, Title: rec.Title, Year: rec.Year,
				Jurisdiction: rec.Jurisdiction, Pipeline: rec.PipelineCount, Citations: len(rec.Citations)}
			if len(rec.Citations) > 0 {
				p.PagesWith++
			}
			for _, c := range rec.Citations {
				p.Citations++
				p.Status[c.Status]++
				p.Relation[c.Pipeline]++
				if c.Status != "no_match" {
					p.Linked++
					r.Linked++
					if !c.PipelineLink {
						p.Rewired++
					}
				}
				switch c.Pipeline {
				case "new":
					r.New++
				case "corrected":
					r.Corrected++
				}
			}
			p.Recent = append(p.Recent, r)
		}
	}
	p.Editions = len(editions)
	if !first.IsZero() {
		p.WallSecs = last.Sub(first).Seconds()
		p.Started = first.Format(time.RFC3339)
	}
	sort.SliceStable(p.Recent, func(i, j int) bool { return p.Recent[i].Batch > p.Recent[j].Batch })
	if len(p.Recent) > 60 {
		p.Recent = p.Recent[:60]
	}
	return p, nil
}

var dashboard = template.Must(template.New("d").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Random pages read by Claude</title>
<link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.8/dist/css/bootstrap.min.css">
<style>
.stat{font-size:2.4rem;font-weight:600;line-height:1.1;font-variant-numeric:tabular-nums}
.label{font-size:.8rem;text-transform:uppercase;letter-spacing:.04em;color:#666}
.bar{display:flex;height:1.4rem;border-radius:4px;overflow:hidden;background:#e9ecef}
.bar div{height:100%}
td.num,th.num{text-align:right;font-variant-numeric:tabular-nums}
.dot{display:inline-block;width:.7rem;height:.7rem;border-radius:50%;margin-right:.3rem}
</style></head><body><main class="container py-4" style="max-width:1100px">
<h1 class="h3">Random MOML body pages read by Claude</h1>
<p class="text-secondary mb-4">Pages drawn uniformly over treatise editions, each read as two requests to <code>claude-opus-5-5</code>:
detect the citations, then link them to candidate cases. <span id="state"></span></p>
<div class="row g-4 mb-4">
 <div class="col-6 col-lg"><div class="label">Pipeline detections</div><div class="stat" id="pipeline"></div><div class="small text-secondary" id="pipenote"></div></div>
 <div class="col-6 col-lg"><div class="label">Citations Claude detected</div><div class="stat" id="citations"></div><div class="small text-secondary" id="citesnote"></div></div>
 <div class="col-6 col-lg"><div class="label">Active time</div><div class="stat" id="active"></div><div class="small text-secondary" id="rate"></div></div>
 <div class="col-6 col-lg"><div class="label">Wall clock</div><div class="stat" id="wall"></div><div class="small text-secondary" id="started"></div></div>
 <div class="col-6 col-lg"><div class="label">Pages read</div><div class="stat" id="pages"></div><div class="small text-secondary" id="pagesnote"></div></div>
</div>
<div class="row g-4 mb-4">
 <div class="col-md-6"><h2 class="h6">Links</h2><div class="bar" id="statusbar"></div><div class="small mt-2" id="statuskey"></div></div>
 <div class="col-md-6"><h2 class="h6">Against the pipeline's detections</h2><div class="bar" id="relbar"></div><div class="small mt-2" id="relkey"></div></div>
</div>
<h2 class="h6">Pages, latest batch first</h2>
<table class="table table-sm small"><thead><tr><th>Batch</th><th>Treatise</th><th class="num">Year</th><th></th><th>Page</th>
<th class="num">Pipeline</th><th class="num">Claude</th><th class="num">New</th><th class="num">Corrected</th><th class="num">Linked</th></tr></thead>
<tbody id="rows"></tbody></table>
</main>
<script>
const statusNames = {linked_cap:["CAP","#2f6db5"], linked_english_reports:["English Reports","#b5552f"],
  linked_code_reporter:["Code reporter","#7a5fa8"], linked_stub:["Stub","#8a8a3a"], no_match:["No match","#bdbdbd"]};
const relNames = {same:["Same as pipeline","#5b8c5a"], corrected:["Corrected","#d39a2d"], "new":["New","#2f6db5"]};
const n = x => x.toLocaleString("en-US");
function clock(s){ s=Math.max(0,Math.floor(s)); const h=Math.floor(s/3600), m=Math.floor(s%3600/60), x=s%60;
  return (h? h+":"+String(m).padStart(2,"0") : m)+":"+String(x).padStart(2,"0"); }
function bar(el, key, names, counts){
  const total = Object.values(counts).reduce((a,b)=>a+b,0);
  el.replaceChildren(); key.replaceChildren();
  for (const k of Object.keys(names)) { const v = counts[k]||0; if(!v) continue;
    const d=document.createElement("div"); d.style.width=(100*v/total)+"%"; d.style.background=names[k][1]; d.title=names[k][0]+": "+n(v); el.append(d);
    const s=document.createElement("span"); s.className="me-3 text-nowrap";
    const dot=document.createElement("span"); dot.className="dot"; dot.style.background=names[k][1];
    s.append(dot, names[k][0]+" "+n(v)+" ("+(100*v/total).toFixed(0)+"%)"); key.append(s); }
}
let data=null, fetched=0;
function tick(){ if(!data) return; const extra = data.running_batch ? (Date.now()-fetched)/1000 : 0;
  document.getElementById("active").textContent = clock(data.active_seconds+extra);
  document.getElementById("wall").textContent = clock(data.wall_seconds+extra); }
async function load(){
  try { data = await (await fetch("api/progress")).json(); fetched = Date.now(); } catch(e) { return; }
  const $ = id => document.getElementById(id);
  $("pages").textContent = n(data.pages);
  $("pagesnote").textContent = n(data.batches)+" batches · "+n(data.editions)+" editions · "+n(data.pages_with_citations)+" pages with citations";
  $("citations").textContent = n(data.citations);
  $("citesnote").textContent = n(data.linked)+" linked to a case · "+n(data.citations-data.linked)+" with no match";
  $("pipeline").textContent = n(data.pipeline_detections);
  $("pipenote").textContent = "on the same pages · Claude kept "+n(data.pipeline_detections-data.pipeline_rejected)+" ("+n(data.relation.corrected||0)+" of them corrected) and rejected "+n(data.pipeline_rejected)+" · it missed "+n(data.relation["new"]||0)+" that Claude found";
  $("rate").textContent = data.active_seconds>0 && data.pages ? (data.pages/(data.active_seconds/3600)).toFixed(0)+" pages per active hour" : "";
  $("started").textContent = data.started ? "since "+new Date(data.started).toLocaleString() : "";
  $("state").textContent = data.running_batch ? "Batch "+data.running_batch+" ("+data.running_pages+" pages) is being read now." : "No batch is running.";
  bar($("statusbar"), $("statuskey"), statusNames, data.status);
  bar($("relbar"), $("relkey"), relNames, data.relation);
  const rows = $("rows"); rows.replaceChildren();
  for (const r of data.recent) { const tr=document.createElement("tr");
    const cells=[r.batch, r.title.length>70? r.title.slice(0,70)+"…":r.title, r.year||"", r.jurisdiction, r.psmid+"/"+r.pageid, r.pipeline, r.citations, r.new, r.corrected, r.linked];
    cells.forEach((c,i)=>{ const td=document.createElement("td"); td.textContent=c; if(i==2||i>4) td.className="num"; tr.append(td); });
    rows.append(tr); }
  tick();
}
load(); setInterval(load, 10000); setInterval(tick, 1000);
</script></body></html>`))

// serve shows the dashboard: counts, times and the pages read so far.
func serve(dir string, port int) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/progress", func(w http.ResponseWriter, r *http.Request) {
		p, err := count(dir)
		if err != nil {
			slog.Error("counting progress", "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(p)
	})
	mux.HandleFunc("/{$}", func(w http.ResponseWriter, r *http.Request) {
		if err := dashboard.Execute(w, nil); err != nil {
			slog.Error("rendering dashboard", "error", err)
		}
	})
	slog.Info("serving progress", "port", port, "dir", dir)
	return http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
}

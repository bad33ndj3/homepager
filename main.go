package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type MR struct {
	ID        int       `json:"id"`
	IID       int       `json:"iid"`
	ProjectID int       `json:"project_id"`
	Title     string    `json:"title"`
	WebURL    string    `json:"web_url"`
	UpdatedAt time.Time `json:"updated_at"`
	Author    struct {
		Name string `json:"name"`
	} `json:"author"`
	References struct {
		Full string `json:"full"`
	} `json:"references"`
	HeadPipeline *struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
		WebURL string `json:"web_url"`
	} `json:"head_pipeline"`
}

type Todo struct {
	ID         int    `json:"id"`
	ActionName string `json:"action_name"`
	TargetType string `json:"target_type"`
	Target     struct {
		Title  string `json:"title"`
		WebURL string `json:"web_url"`
	} `json:"target"`
	Project struct {
		Name string `json:"name"`
	} `json:"project"`
	CreatedAt time.Time `json:"created_at"`
}

type Pipeline struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
	WebURL string `json:"web_url"`
}

func apiGet(url, token string, v any) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("PRIVATE-TOKEN", token)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("GET %s -> %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(v)
}

func uniqMRs(in []MR) []MR {
	seen := map[string]bool{}
	out := make([]MR, 0, len(in))
	for _, m := range in {
		key := fmt.Sprintf("%d:%d", m.ProjectID, m.IID)
		if !seen[key] {
			seen[key] = true
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out
}

func splitUsers(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Attach latest pipeline if head_pipeline missing.
func attachPipelines(base, token string, mrs []MR) []MR {
	for i := range mrs {
		if mrs[i].HeadPipeline != nil {
			continue
		}
		var pipes []Pipeline
		u := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/pipelines?per_page=1", base, mrs[i].ProjectID, mrs[i].IID)
		if err := apiGet(u, token, &pipes); err != nil || len(pipes) == 0 {
			continue
		}
		p := pipes[0]
		mrs[i].HeadPipeline = &struct {
			ID     int    `json:"id"`
			Status string `json:"status"`
			WebURL string `json:"web_url"`
		}{ID: p.ID, Status: p.Status, WebURL: p.WebURL}
	}
	return mrs
}

func collectTeammateMRs(base, token string, users []string) []MR {
	if len(users) == 0 {
		return nil
	}
	buf := make([]MR, 0, 64)
	for _, u := range users {
		var authored []MR
		var assigned []MR
		_ = apiGet(fmt.Sprintf("%s/api/v4/merge_requests?scope=all&state=opened&author_username=%s&per_page=100&include=head_pipeline", base, u), token, &authored)
		_ = apiGet(fmt.Sprintf("%s/api/v4/merge_requests?scope=all&state=opened&assignee_username=%s&per_page=100&include=head_pipeline", base, u), token, &assigned)
		buf = append(buf, authored...)
		buf = append(buf, assigned...)
	}
	return uniqMRs(buf)
}

var page = template.Must(template.New("p").Parse(`
<!doctype html>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="color-scheme" content="light dark">
<title>GitLab dashboard – {{.User}}</title>
<style>
:root{
  /* light */
  --bg:#f6f7fb;
  --panel:#ffffff;
  --panel-2:#f2f4f8;
  --text:#0b1220;
  --muted:#566173;
  --brand:#0b63ff;
  --border:#dbe1ea;
}
@media (prefers-color-scheme: dark){
  :root{
    --bg:#0b1020;
    --panel:#111731;
    --panel-2:#0e142a;
    --text:#e8ecf1;
    --muted:#9aa6b2;
    --brand:#6aa3ff;
    --border:#223056;
  }
}
*{box-sizing:border-box}
body{
  margin:0;padding:24px;min-height:100vh;
  font:15px/1.5 system-ui, Segoe UI, Roboto, Helvetica, Arial, "Apple Color Emoji","Segoe UI Emoji";
  color:var(--text);
  background:var(--bg);
}
@media (prefers-color-scheme: dark){
  body{background:radial-gradient(1200px 800px at 100% -20%, #1a2447 0%, rgba(26,36,71,0) 60%), var(--bg);}
}
.container{max-width:1100px;margin:0 auto}
.header{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:18px}
.brand{display:flex;align-items:center;gap:12px}
.brand .logo{width:34px;height:34px;border-radius:8px;background:linear-gradient(135deg, var(--brand), #b38cff)}
.brand h1{font-size:20px;margin:0}
.topline{color:var(--muted);font-size:12px;margin-bottom:18px}
.section{margin-top:26px}
.section h2{font-size:16px;color:var(--muted);margin:0 0 10px 0}
.grid{display:grid;grid-template-columns:repeat(auto-fill, minmax(320px,1fr));gap:12px}
.card{
  background:linear-gradient(180deg, var(--panel), var(--panel-2));
  border:1px solid var(--border);border-radius:14px;padding:14px;
  transition:transform .08s ease, box-shadow .2s ease, border-color .2s ease
}
@media (prefers-color-scheme: dark){
  .card{box-shadow:0 6px 18px rgba(0,0,0,.25)}
  .card:hover{transform:translateY(-2px);box-shadow:0 10px 24px rgba(0,0,0,.35);border-color:#2c3e70}
}
.card .title{font-weight:600;margin-bottom:6px}
.card .title a{color:var(--text);text-decoration:none}
.card .title a:hover{color:var(--brand)}
.meta{display:flex;flex-wrap:wrap;gap:8px;align-items:center;color:var(--muted);font-size:12px}
.badge{display:inline-block;padding:2px 8px;border-radius:999px;border:1px solid var(--border);background:var(--panel-2);color:var(--text);font-size:11px}
.small{color:var(--muted);font-size:12px}
.empty{color:var(--muted);font-size:13px;padding:10px;border:1px dashed var(--border);border-radius:10px;background:var(--panel-2)}
hr.sep{border:none;border-top:1px solid var(--border);margin:10px 0}
a{color:var(--brand);text-decoration:none}
a:hover{text-decoration:underline}
footer{margin-top:28px;color:var(--muted);font-size:12px}
.layout{display:grid;grid-template-columns:280px 1fr;gap:16px}
.sidebar{background:linear-gradient(180deg, var(--panel), var(--panel-2));border:1px solid var(--border);border-radius:14px;padding:14px;height:fit-content;position:sticky;top:16px}
.sidebar h2{font-size:15px;margin:0 0 8px 0;color:var(--muted)}
.list{list-style:none;margin:0;padding:0;display:flex;flex-direction:column;gap:8px}
.list li a{color:var(--text);text-decoration:none}
.list li a:hover{color:var(--brand)}
.content{min-width:0}
@media (max-width: 860px){.layout{grid-template-columns:1fr}.sidebar{position:static}}
/* pipeline dots */
.pipe{display:inline-flex;align-items:center;gap:6px}
.dot{display:inline-block;width:10px;height:10px;border-radius:50%;background:#3b82f6;box-shadow:0 0 0 1px var(--border)}
.dot[data-status="success"]{background:#22c55e}
.dot[data-status="failed"]{background:#ef4444}
</style>
<div class="container">
  <div class="header">
    <div class="brand">
      <div class="logo"></div>
      <h1>GitLab dashboard</h1>
    </div>
    <div class="small">Ingelogd als <strong>{{.User}}</strong></div>
  </div>
  <div class="topline">Host: {{.Base}} • Auto-refresh elke 60s</div>

  <div class="layout">
    <aside class="sidebar">
      <h2>Team MR’s</h2>
      {{if .TeamMRs}}
        <ul class="list">
        {{range .TeamMRs}}
          <li>
            <a target="_blank" rel="noopener noreferrer" href="{{.WebURL}}">{{.Title}}</a>
            <div class="small">{{.References.Full}} • {{.Author.Name}}</div>
            {{if .HeadPipeline}}
              <a class="pipe" target="_blank" rel="noopener noreferrer" href="{{.HeadPipeline.WebURL}}" title="pipeline: {{.HeadPipeline.Status}}">
                <span class="dot" data-status="{{.HeadPipeline.Status}}"></span>
              </a>
            {{end}}
          </li>
        {{end}}
        </ul>
      {{else}}
        <div class="empty">Geen team-MR’s.</div>
      {{end}}
      <hr class="sep"/>
      <div class="small">Bron: auteurs of assignees uit <code>TEAMMATE_USERNAMES</code></div>
    </aside>

    <main class="content">
      <div class="section">
        <h2>Open Merge Requests <span class="small">(assignee + reviewer)</span></h2>
        {{if .MRs}}
          <div class="grid">
          {{range .MRs}}
            <div class="card">
              <div class="title"><a target="_blank" rel="noopener noreferrer" href="{{.WebURL}}">{{.Title}}</a></div>
              <div class="meta">
                <span class="badge">{{.References.Full}}</span>
                <span>door {{.Author.Name}}</span>
                {{if .HeadPipeline}}
                  <a class="pipe" target="_blank" rel="noopener noreferrer" href="{{.HeadPipeline.WebURL}}" title="pipeline: {{.HeadPipeline.Status}}">
                    <span class="dot" data-status="{{.HeadPipeline.Status}}"></span>
                  </a>
                {{end}}
                <span>•</span>
                <span>laatst geüpdatet</span>
                <time class="timeago" datetime="{{.UpdatedAt.Format "2006-01-02T15:04:05Z07:00"}}"></time>
              </div>
            </div>
          {{end}}
          </div>
        {{else}}
          <div class="empty">Geen open MR’s.</div>
        {{end}}
      </div>

      <div class="section">
        <h2>Todos</h2>
        {{if .Todos}}
          <div class="grid">
          {{range .Todos}}
            <div class="card">
              <div class="title"><a target="_blank" rel="noopener noreferrer" href="{{.Target.WebURL}}">{{.Target.Title}}</a></div>
              <div class="meta">
                <span class="badge">{{.Project.Name}}</span>
                <span class="badge">{{.TargetType}}</span>
                <span class="badge">{{.ActionName}}</span>
                <span>• aangemaakt</span>
                <time class="timeago" datetime="{{.CreatedAt.Format "2006-01-02T15:04:05Z07:00"}}"></time>
              </div>
            </div>
          {{end}}
          </div>
        {{else}}
          <div class="empty">Geen open todos.</div>
        {{end}}
      </div>
    </main>
  </div>

  <footer>Tip: klik op een kaart om in een nieuw tabblad te openen.</footer>
</div>

<script>
function timeago(dt){
  const rtf = new Intl.RelativeTimeFormat(navigator.language || 'nl-NL', {numeric:'auto'});
  const diff = (new Date(dt) - new Date()) / 1000;
  const abs = Math.abs(diff);
  const units = [['year',31536000],['month',2592000],['week',604800],['day',86400],['hour',3600],['minute',60],['second',1]];
  for (const [unit, sec] of units){
    if (abs >= sec || unit === 'second'){ return rtf.format(Math.round(diff / sec), unit); }
  }
}
function refreshTimes(){
  document.querySelectorAll('time.timeago').forEach(t=>{
    const dt = t.getAttribute('datetime');
    if (dt) t.textContent = timeago(dt);
  });
}
refreshTimes(); setInterval(refreshTimes, 30000); setTimeout(()=>location.reload(), 60000);
</script>
`))

func handler(w http.ResponseWriter, r *http.Request) {
	base := os.Getenv("GITLAB_BASE") // e.g., https://gitlab.com
	token := os.Getenv("GITLAB_TOKEN")
	user := os.Getenv("GITLAB_USERNAME")
	teamEnv := os.Getenv("TEAMMATE_USERNAMES")
	teamUsers := splitUsers(teamEnv)

	if base == "" || token == "" || user == "" {
		http.Error(w, "Set env vars: GITLAB_BASE, GITLAB_TOKEN, GITLAB_USERNAME", 500)
		return
	}

	// My MRs
	var assignee []MR
	var reviewer []MR
	_ = apiGet(fmt.Sprintf("%s/api/v4/merge_requests?scope=all&state=opened&assignee_username=%s&per_page=100&include=head_pipeline", base, user), token, &assignee)
	_ = apiGet(fmt.Sprintf("%s/api/v4/merge_requests?scope=all&state=opened&reviewer_username=%s&per_page=100&include=head_pipeline", base, user), token, &reviewer)
	all := uniqMRs(append(assignee, reviewer...))
	all = attachPipelines(base, token, all)

	// Team MRs
	teamMRs := collectTeammateMRs(base, token, teamUsers)
	teamMRs = attachPipelines(base, token, teamMRs)

	// Todos
	var todos []Todo
	_ = apiGet(fmt.Sprintf("%s/api/v4/todos?state=pending&per_page=100", base), token, &todos)

	_ = page.Execute(w, map[string]any{
		"User":    user,
		"Base":    base,
		"MRs":     all,
		"Todos":   todos,
		"TeamMRs": teamMRs,
	})
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env found or failed to load")
	}
	http.HandleFunc("/", handler)
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("listening on :" + port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

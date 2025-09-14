package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// ====== Types ======
type Task struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Result      string `json:"result"`
	Command     string `json:"command"`
	Tasks       []Task `json:"tasks"`
}
type Workflow struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Tasks  []Task `json:"tasks"`
}

// ====== CLI flags ======
var (
	flagWorkflow = flag.String("workflow", "", "Path to workflow JSON")
	flagTarget   = flag.String("target", "", "Override the workflow target")
	flagOutDir   = flag.String("outdir", "out", "Root directory for outputs")
	flagDryRun   = flag.Bool("dry-run", false, "Print commands without executing")
	flagContinue = flag.Bool("continue-on-error", false, "Continue when a task fails")
	flagShell    = flag.String("shell", "", "Custom shell (default: sh -c on Unix, cmd /C on Windows)")
	flagServe    = flag.Bool("serve", false, "Start local web UI")
	flagPlan     = flag.Bool("plan", false, "Print the execution plan and exit")
	flagAddr     = flag.String("addr", ":8080", "HTTP listen address (for --serve)")
)

// ANSI colors
const (
	cReset = "\033[0m"
	cBold  = "\033[1m"
	cGreen = "\033[32m"
	cRed   = "\033[31m"
	cCyan  = "\033[36m"
	cGrey  = "\033[90m"
)

func main() {
	flag.Parse()

	// Web UI mode
	if *flagServe {
		if err := startServer(*flagAddr); err != nil {
			fail("serve: %v", err)
		}
		return
	}

	if *flagWorkflow == "" {
		fail("usage: asr-runner --workflow ../workflows/attack-surface-recon.json --target example.com")
	}

	wf, err := loadWorkflow(*flagWorkflow)
	check(err, "load workflow")

	if *flagTarget != "" {
		wf.Target = *flagTarget
	}
	if strings.TrimSpace(wf.Target) == "" {
		fail("empty target: set in JSON or pass --target")
	}

	runDir := filepath.Join(*flagOutDir,
		fmt.Sprintf("%s-%s", sanitize(wf.Name), time.Now().Format("20060102-150405")))
	check(os.MkdirAll(runDir, 0o755), "create run dir")

	fmt.Printf("%s▶ %s%s\n", cBold, wf.Name, cReset)
	fmt.Printf("  %starget%s : %s\n", cCyan, cReset, wf.Target)
	fmt.Printf("  %soutput%s : %s\n", cCyan, cReset, runDir)
	if *flagDryRun {
		fmt.Printf("  %smode%s   : DRY RUN\n", cCyan, cReset)
	}
	fmt.Println()

	if *flagPlan {
		printPlan(wf, runDir)
		return
	}

	ctx := context.Background()
	start := time.Now()
	for i := range wf.Tasks {
		if err := execTask(ctx, &wf.Tasks[i], wf.Target, "", runDir); err != nil {
			if *flagContinue {
				fmt.Printf("%s✗ task failed, continuing:%s %v\n\n", cRed, cReset, err)
				continue
			}
			fail("stopped on error: %v", err)
		}
	}
	fmt.Printf("%s✅ done%s in %s\n", cGreen, cReset, time.Since(start).Truncate(time.Millisecond))
}

// ====== Core runner ======
func loadWorkflow(path string) (*Workflow, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wf Workflow
	if err := json.Unmarshal(b, &wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

func execTask(ctx context.Context, t *Task, target, parentResult, runDir string) error {
	if t == nil {
		return errors.New("nil task")
	}
	var resultPath string
	if t.Result != "" {
		resultPath = filepath.Join(runDir, filepath.Clean(t.Result))
		check(os.MkdirAll(filepath.Dir(resultPath), 0o755), "ensure result dir")
	}
	cmdStr := interpolate(t.Command, map[string]string{
		"{target}":        target,
		"{result}":        resultPath,
		"{parent_result}": parentResult,
		"{outdir}":        runDir,
	})

	fmt.Printf("%s• %s%s\n", cBold, t.Name, cReset)
	if t.Description != "" {
		fmt.Printf("  - %s\n", t.Description)
	}
	if resultPath != "" {
		fmt.Printf("  - result: %s\n", resultPath)
	}
	fmt.Printf("  - cmd   : %s%s%s\n", cGrey, cmdStr, cReset)

	start := time.Now()
	var err error
	if !*flagDryRun {
		ctx2, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- runShell(ctx2, cmdStr) }()
		go spinner(ctx2, "running...")
		err = <-done
		cancel()
	}
	if err != nil {
		fmt.Printf("  %s✗ error:%s %v\n\n", cRed, cReset, err)
		return err
	}
	fmt.Printf("  %s✓ done%s : %s\n\n", cGreen, cReset, time.Since(start).Truncate(time.Millisecond))

	for i := range t.Tasks {
		if err := execTask(ctx, &t.Tasks[i], target, resultPath, runDir); err != nil {
			if *flagContinue {
				fmt.Printf("    ↪ continue after error in %q: %v\n", t.Tasks[i].Name, err)
				continue
			}
			return err
		}
	}
	return nil
}

// ====== Helpers ======
func spinner(ctx context.Context, msg string) {
	frames := []rune{'|', '/', '-', '\\'}
	t := time.NewTicker(120 * time.Millisecond)
	defer t.Stop()
	i := 0
	for {
		select {
		case <-ctx.Done():
			fmt.Print("\r")
			return
		case <-t.C:
			fmt.Printf("\r%s %s", string(frames[i%len(frames)]), msg)
			i++
		}
	}
}

func runShell(ctx context.Context, command string) error {
	sh, args := defaultShell()
	if *flagShell != "" {
		sh = *flagShell
		args = []string{"-c"}
	}
	cmd := exec.CommandContext(ctx, sh, append(args, command)...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	go ioCopyLines(os.Stdout, stdout)
	go ioCopyLines(os.Stderr, stderr)
	return cmd.Wait()
}

func ioCopyLines(dst *os.File, r io.Reader) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		fmt.Fprintln(dst, sc.Text())
	}
}

func defaultShell() (string, []string) {
	if runtime.GOOS == "windows" {
		return "cmd", []string{"/C"}
	}
	return "sh", []string{"-c"}
}

func interpolate(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, k, v)
	}
	return s
}

func sanitize(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.ReplaceAll(s, " ", "-")
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, s)
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "ERR: "+format+"\n", a...)
	os.Exit(1)
}

func check(err error, when string) {
	if err != nil {
		fail("%s: %v", when, err)
	}
}

// ====== Plan mode (preview) ======
func printPlan(wf *Workflow, runDir string) {
	fmt.Println("Execution plan:")
	for _, t := range wf.Tasks {
		printPlanTask(t, 0, wf.Target, "", runDir)
	}
}

func printPlanTask(t Task, level int, target, parentResult, runDir string) {
	indent := strings.Repeat("  ", level)
	var resultPath string
	if t.Result != "" {
		resultPath = filepath.Join(runDir, filepath.Clean(t.Result))
	}
	cmd := interpolate(t.Command, map[string]string{
		"{target}":        target,
		"{result}":        resultPath,
		"{parent_result}": parentResult,
		"{outdir}":        runDir,
	})
	fmt.Printf("%s- %s\n", indent, t.Name)
	if t.Description != "" {
		fmt.Printf("%s  desc  : %s\n", indent, t.Description)
	}
	if resultPath != "" {
		fmt.Printf("%s  result: %s\n", indent, resultPath)
	}
	fmt.Printf("%s  cmd   : %s\n", indent, cmd)
	for _, st := range t.Tasks {
		printPlanTask(st, level+1, target, resultPath, runDir)
	}
}

// ====== Web UI ======
type runJob struct {
	id     string
	outdir string
	logs   chan string
	done   chan struct{}
}

var (
	runs   = make(map[string]*runJob)
	runsMu sync.RWMutex
)

func startServer(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", uiIndex)
	mux.HandleFunc("/run", uiRun)
	mux.HandleFunc("/stream", uiStream)

	fmt.Printf("Serving asr-runner UI on http://localhost%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func uiIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, indexHTML)
}

func uiRun(w http.ResponseWriter, r *http.Request) {
	type req struct {
		Target   string `json:"target"`
		Workflow string `json:"workflow"` // JSON text
	}
	var in req
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	var wf Workflow
	if err := json.Unmarshal([]byte(in.Workflow), &wf); err != nil {
		http.Error(w, "json: "+err.Error(), 400)
		return
	}
	if in.Target != "" {
		wf.Target = in.Target
	}
	if strings.TrimSpace(wf.Target) == "" {
		http.Error(w, "target required", 400)
		return
	}
	outdir := filepath.Join("out", fmt.Sprintf("%s-%s", sanitize(wf.Name), time.Now().Format("20060102-150405")))
	_ = os.MkdirAll(outdir, 0o755)

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	job := &runJob{id: id, outdir: outdir, logs: make(chan string, 1024), done: make(chan struct{})}

	runsMu.Lock()
	runs[id] = job
	runsMu.Unlock()

	go func() {
		defer close(job.done)
		ctx := context.Background()
		job.logs <- fmt.Sprintf("▶ %s\n  target: %s\n  outdir: %s\n\n", wf.Name, wf.Target, outdir)
		for i := range wf.Tasks {
			job.logs <- fmt.Sprintf("• %s\n", wf.Tasks[i].Name)
			if err := execTaskWeb(ctx, &wf.Tasks[i], wf.Target, "", outdir, job.logs); err != nil {
				job.logs <- fmt.Sprintf("✗ error: %v\n", err)
				break
			}
		}
		job.logs <- "✅ done\n"
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"run_id": id, "outdir": outdir})
}

func execTaskWeb(ctx context.Context, t *Task, target, parentResult, runDir string, logs chan<- string) error {
	var resultPath string
	if t.Result != "" {
		resultPath = filepath.Join(runDir, filepath.Clean(t.Result))
		_ = os.MkdirAll(filepath.Dir(resultPath), 0o755)
	}
	cmdStr := interpolate(t.Command, map[string]string{
		"{target}": target, "{result}": resultPath, "{parent_result}": parentResult, "{outdir}": runDir,
	})
	if t.Description != "" {
		logs <- "  - " + t.Description + "\n"
	}
	if resultPath != "" {
		logs <- "  - result: " + resultPath + "\n"
	}
	logs <- "  - cmd   : " + cmdStr + "\n"

	sh, args := defaultShell()
	cmd := exec.CommandContext(ctx, sh, append(args, cmdStr)...)
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			logs <- sc.Text() + "\n"
		}
	}()
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			logs <- sc.Text() + "\n"
		}
	}()
	if err := cmd.Wait(); err != nil {
		return err
	}
	logs <- "  ✓ done\n\n"

	for i := range t.Tasks {
		if err := execTaskWeb(ctx, &t.Tasks[i], target, resultPath, runDir, logs); err != nil {
			return err
		}
	}
	return nil
}

func uiStream(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")

	runsMu.RLock()
	job := runs[id]
	runsMu.RUnlock()

	if job == nil {
		http.Error(w, "unknown run id", 404)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)

	for {
		select {
		case line := <-job.logs:
			fmt.Fprintf(w, "data: %s\n\n", strings.ReplaceAll(line, "\n", "\\n"))
			flusher.Flush()
		case <-job.done:
			fmt.Fprintf(w, "data: [run complete]\\n\n\n")
			flusher.Flush()
			runsMu.Lock()
			delete(runs, id) // free memory; new streams will 404
			runsMu.Unlock()
			return
		case <-r.Context().Done():
			return
		}
	}
}

const indexHTML = `<!doctype html><html><head><meta charset="utf-8"/>
<title>asr-runner UI</title>
<style>
body{font-family:system-ui,Segoe UI,Arial,sans-serif;margin:32px;max-width:1000px}
textarea{width:100%;height:260px}
pre{background:#0f172a;color:#e2e8f0;padding:12px;border-radius:8px;white-space:pre-wrap}
input,button{font:inherit;padding:8px 12px}
label{display:block;margin:12px 0 6px}
.row{display:flex;gap:12px;align-items:center}
</style></head><body>
<h2>asr-runner</h2>
<div class="row">
  <label>Target: <input id="target" placeholder="example.com"/></label>
  <button id="run">Run</button>
</div>
<label>Workflow (JSON):</label>
<textarea id="wf"></textarea>
<h3>Logs</h3>
<pre id="logs"></pre>
<script>
const sample = {
  "name":"attack-surface-recon",
  "target":"example.com",
  "tasks":[
    {"name":"Subdomain Finder","description":"enumerate subdomains","result":"subdomains.txt",
     "command":"subfinder -d {target} -silent -o {result}",
     "tasks":[
        {"name":"Probe HTTP(S)","description":"which hosts are alive","result":"http_result.txt",
         "command":"httpx -l {parent_result} -silent -follow-redirects -mc 200,301,302,401,403 -o {result}",
         "tasks":[
            {"name":"Collect URLs","description":"crawl endpoints from live hosts","result":"urls.txt",
             "command":"katana -list {parent_result} -d 1 -rl 50 -silent -o {result}",
             "tasks":[] } ]} ]} ]};
document.getElementById('wf').value = JSON.stringify(sample,null,2);
document.getElementById('run').onclick = async () => {
  const wf = document.getElementById('wf').value;
  const target = document.getElementById('target').value;
  const resp = await fetch('/run',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({workflow:wf,target})});
  const {run_id,outdir} = await resp.json();
  const es = new EventSource('/stream?id='+run_id);
  const logs = document.getElementById('logs');
  logs.textContent = 'outdir: '+outdir+'\\n\\n';
  es.onmessage = (e)=>{
    if(e.data === '[run complete]'){
      logs.textContent += 'run complete\\n';
      es.close(); // stop reconnect loop
      return;
    }
    logs.textContent += e.data.replaceAll('\\n','\\n') + '\\n';
    logs.scrollTop = logs.scrollHeight;
  };
};
</script>
</body></html>`

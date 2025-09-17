# 🔎 ASR-Runner

*A lightweight **Attack Surface Reconnaissance - runner** built in Go, wrapping [subfinder](https://github.com/projectdiscovery/subfinder), [httpx](https://github.com/projectdiscovery/httpx), and [katana](https://github.com/projectdiscovery/katana).*

---

[![Watch the video](https://img.youtube.com/vi/i9A7b6rXYOU/0.jpg)](https://youtu.be/i9A7b6rXYOU)

---

## 🚀 What is this?

`asr-runner` is a simple orchestration tool for security researchers and pentesters.  
It automates **attack surface reconnaissance** in a workflow-like fashion:

1. **Subdomain Enumeration** → Find subdomains of a target  
2. **HTTP Probing** → Check which subdomains are alive  
3. **URL Collection** → Crawl endpoints from those live hosts  
4. **AI Report Generation** → Generate security analysis reports using OpenAI

It runs tasks sequentially, saving results into clean output files — no copy-pasting between tools needed.

---

## ✨ Features

- 📝 **Workflow-driven** – Define tasks in JSON, chain them together
- ⚡ **Orchestrates ProjectDiscovery tools** – subfinder → httpx → katana
- 🌐 **Web UI** – Start with `--serve` and manage runs from a browser
- 🖥 **CLI mode** – Run directly with `--workflow` + `--target`
- 🤖 **AI-powered reports** – Generate security analysis with OpenAI integration
- 🔄 **Continue on error** and **dry-run** modes
- 📂 **Output management** – Each run gets its own timestamped folder

---

## 📦 Installation

Clone and build:

```bash
git clone https://github.com/JosephDavisC/ASR-Runner.git
cd asr-runner
go build ./cmd/asr-runner
```

Install required tools:

```bash
# ProjectDiscovery tools
go install -v github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest
go install -v github.com/projectdiscovery/httpx/cmd/httpx@latest  
go install -v github.com/projectdiscovery/katana/cmd/katana@latest

# Python dependencies for AI reports (optional)
pip3 install openai
```

Make sure tools are in your `$PATH`:

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

---

## 🛠 Usage

### Web UI Mode (Recommended)

```bash
./asr-runner --serve --addr :8090
```

Then open [http://localhost:8090](http://localhost:8090) in your browser.

**Features:**
- Real-time log streaming
- Interactive workflow editing
- **AI report generation** with live preview
- Side-by-side logs and report view

### CLI Mode

Run a workflow JSON:

```bash
./asr-runner --workflow ./workflows/attack-surface-recon.json --target example.com
```

Options:

- `--outdir` : Where to store results (default: `out/`)
- `--dry-run` : Print commands without executing
- `--continue-on-error` : Skip failed tasks and keep running
- `--plan` : Show execution plan and exit

---

## 🤖 AI Report Generation

Set up OpenAI API key:

```bash
export OPENAI_API_KEY="your-api-key-here"
```

The AI feature automatically:
- Analyzes discovered subdomains, live hosts, and URLs
- Generates concise security reports with actionable insights
- Identifies top opportunities and follow-up actions
- Saves reports as `ai_draft.md` in the output directory

**Web UI:** Check "Generate AI Report" before running
**CLI:** Reports can be generated manually using `make_ai_report.py`

---

## 📝 Example Workflow

Save as `workflows/attack-surface-recon.json`:

```json
{
  "name": "attack-surface-recon",
  "target": "example.com",
  "tasks": [
    {
      "name": "Subdomain Finder",
      "description": "enumerate subdomains",
      "result": "subdomains.txt",
      "command": "subfinder -d {target} -silent -o {result}",
      "tasks": [
        {
          "name": "Probe HTTP(S)",
          "description": "which hosts are alive",
          "result": "http_result.txt",
          "command": "httpx -l {parent_result} -silent -follow-redirects -mc 200,301,302,401,403 -o {result}",
          "tasks": [
            {
              "name": "Collect URLs",
              "description": "crawl endpoints from live hosts",
              "result": "urls.txt",
              "command": "katana -list {parent_result} -d 1 -rl 50 -silent -o {result}",
              "tasks": []
            }
          ]
        }
      ]
    }
  ]
}
```

---

## 📂 Output

Each run creates a folder under `out/`, e.g.:

```
out/
  attack-surface-recon-20250913-204803/
    subdomains.txt
    http_result.txt
    urls.txt
    ai_draft.md          # Generated AI report
```

---

## ⚠️ Disclaimer

This tool is intended **only for educational and authorized security testing purposes.**  

---

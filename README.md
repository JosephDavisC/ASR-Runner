# 🔎 asr-runner

*A lightweight **Attack Surface Reconnaissance - runner** built in Go, wrapping [subfinder](https://github.com/projectdiscovery/subfinder), [httpx](https://github.com/projectdiscovery/httpx), and [katana](https://github.com/projectdiscovery/katana).*

---

[![Watch the video](https://img.youtube.com/vi/TlSJXNlGdAQ/0.jpg)](https://youtu.be/TlSJXNlGdAQ)
---

## 🚀 What is this?

`asr-runner` is a simple orchestration tool for security researchers and pentesters.  
It automates **attack surface reconnaissance** in a workflow-like fashion:

1. **Subdomain Enumeration** → Find subdomains of a target  
2. **HTTP Probing** → Check which subdomains are alive  
3. **URL Collection** → Crawl endpoints from those live hosts  

It runs tasks sequentially, saving results into clean output files — no copy-pasting between tools needed.

---

## ✨ Features

- 📝 **Workflow-driven** – Define tasks in JSON, chain them together
- ⚡ **Orchestrates ProjectDiscovery tools** – subfinder → httpx → katana
- 🌐 **Web UI** – Start with `--serve` and manage runs from a browser
- 🖥 **CLI mode** – Run directly with `--workflow` + `--target`
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

Make sure [subfinder](https://github.com/projectdiscovery/subfinder), [httpx](https://github.com/projectdiscovery/httpx), and [katana](https://github.com/projectdiscovery/katana) are installed and in your `$PATH`.

---

## 🛠 Usage

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

### Web UI Mode

```bash
./asr-runner --serve --addr :8090
```

Then open [http://localhost:8090](http://localhost:8090) in your browser.  
You can paste/edit workflows and watch logs in real time.

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
```

---

## ⚠️ Disclaimer

This tool is intended **only for educational and authorized security testing purposes.**  

---


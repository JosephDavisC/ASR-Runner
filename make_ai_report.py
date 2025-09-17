#!/usr/bin/env python3
import argparse
from pathlib import Path
from openai import OpenAI

def read_lines(p: Path):
    if not p or not p.exists():
        return []
    return [
        ln.strip()
        for ln in p.read_text(encoding="utf-8", errors="ignore").splitlines()
        if ln.strip()
    ]

def sample_block(lines, n=10):
    if not lines:
        return "(none)"
    seen, uniq = set(), []
    for ln in lines:
        if ln not in seen:
            seen.add(ln)
            uniq.append(ln)
        if len(uniq) >= n:
            break
    return "\n".join(uniq)

def locate(base: Path, filename: str) -> Path | None:
    """Return a Path to filename under base (direct or any subfolder)."""
    base = base.resolve()
    direct = base / filename
    if direct.exists():
        return direct
    for p in base.rglob(filename):
        return p  # first match is fine
    return None

def main():
    ap = argparse.ArgumentParser(description="Make an AI draft report from EWE artifacts")
    ap.add_argument("-t", "--target", required=True, help="Target name (e.g., vulnweb.com)")
    ap.add_argument("-i", "--input", default="out", help="Folder containing artifacts (can have subfolders)")
    ap.add_argument("-o", "--out", default=None, help="Output markdown path (default: <artifact_dir>/ai_draft.md)")
    ap.add_argument("--model", default="gpt-4o-mini", help="OpenAI model (default: gpt-4o-mini)")
    ap.add_argument("--host-sample", type=int, default=10, help="How many live hosts to show")
    ap.add_argument("--url-sample", type=int, default=10, help="How many URLs to show")
    args = ap.parse_args()

    base = Path(args.input)

    # 🔎 Find artifacts anywhere under base (handles timestamped folders)
    sub_path  = locate(base, "subdomains.txt")
    http_path = locate(base, "http_result.txt")
    urls_path = locate(base, "urls.txt")

    # Choose an artifact directory for default output
    artifact_dir = (sub_path or http_path or urls_path).parent if (sub_path or http_path or urls_path) else base

    subdomains = read_lines(sub_path)
    live_hosts = read_lines(http_path)
    urls       = read_lines(urls_path)

    stats = {
        "subdomains": len(subdomains),
        "live_hosts": len(live_hosts),
        "urls": len(urls),
    }

    hosts_sample = sample_block(live_hosts, n=args.host_sample)
    urls_sample  = sample_block(urls, n=args.url_sample)

    # --- Prompt ---
    messages = [
      {"role": "system", "content":
       "You are a security report writer for Sector. "
       "You write concise, evidence-based security briefs. No speculation."
      },
      {"role": "user", "content": f"""
Target: {args.target}

Stats: subdomains={stats['subdomains']}, live_hosts={stats['live_hosts']}, urls={stats['urls']}

Live hosts (sample):
{hosts_sample}

URLs (sample):
{urls_sample}

Write under 350 words with these sections:

### Summary
2–4 sentences on what we scanned and overall exposure.

### Top Opportunities (3–5)
For each:
- Title
- Evidence (host/URL/tech)
- Why it matters (1 sentence)
- Next steps (specific command/check)

### Follow-ups
5–7 concrete analyst actions.
"""}
    ]

    client = OpenAI()
    resp = client.chat.completions.create(
        model=args.model,
        messages=messages,
        temperature=0.2,
    )
    text = resp.choices[0].message.content.strip()

    out_path = Path(args.out) if args.out else (artifact_dir / "ai_draft.md")
    out_path.write_text(text, encoding="utf-8")

    print(f"✅ Wrote {out_path}")
    print(f"   Used: subdomains={sub_path}, http_result={http_path}, urls={urls_path}")
    print(f"   Counts: {stats}")

if __name__ == "__main__":
    main()
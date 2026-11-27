package web

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
)

const layoutStart = `<!doctype html>
<html lang="en"><head><meta charset="UTF-8"><meta name="viewport" content="width=device-width, initial-scale=1"><meta name="theme-color" content="#0b1110"><meta name="description" content="AgentClean helps repository owners review and remove optional coding-agent attribution from Git commit messages."><title>{{.Title}}</title>
<style>
:root{font-family:Inter,ui-sans-serif,system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;color:#edf3ef;background:#0b1110;font-synthesis:none}*{box-sizing:border-box}body{margin:0;min-height:100vh;background:radial-gradient(ellipse at 50% -20%,#18382b 0,transparent 55%),#0b1110}button,input{font:inherit}button{cursor:pointer}button:disabled{cursor:not-allowed;opacity:.45}a{color:inherit}.wrap{max-width:1000px;margin:auto;padding:0 25px}.top{height:83px;border-bottom:1px solid #25362c;display:flex;align-items:center;justify-content:space-between}.brand{display:flex;align-items:center;gap:11px;text-decoration:none;font-size:19px;font-weight:750;letter-spacing:-.6px}.logo{display:grid;place-items:center;background:#8be1ac;color:#092018;width:35px;height:35px;border-radius:5px;font-weight:900}.by{font-size:12px;color:#869c91}.toplinks{display:flex;gap:22px;align-items:center;font-size:13px;color:#a7b7ae}.toplinks a{text-decoration:none}.toplinks a:hover{color:#e9f7ef}.hero{text-align:center;padding:78px 0 48px}.eyebrow{font-size:12px;letter-spacing:2.3px;font-weight:800;color:#90dcae;text-transform:uppercase}.hero h1{font-size:clamp(38px,6vw,65px);line-height:1.09;letter-spacing:-2.5px;margin:20px auto;max-width:760px}.hero h1 span{color:#8be1ac}.hero p{color:#aabeb2;line-height:1.7;max-width:565px;margin:auto;font-size:16px}.wizard{background:#111a16;border:1px solid #314239;border-radius:10px;box-shadow:0 30px 100px #0005;margin:12px auto 90px;max-width:830px;overflow:hidden}.steps{display:grid;grid-template-columns:repeat(4,1fr);padding:25px 28px;gap:12px;border-bottom:1px solid #283a30}.step{display:flex;align-items:center;gap:10px;color:#657d6f;font-size:13px;font-weight:650}.step .num{height:28px;width:28px;border:1px solid #425248;border-radius:50%;display:grid;place-items:center;font-size:12px}.step.active{color:#eafbef}.step.active .num{color:#10241a;background:#8be1ac;border-color:#8be1ac}.step.done{color:#a5d8b8}.step.done .num{border-color:#8be1ac}.body{padding:34px}.body h2{margin:0 0 10px;font-size:25px;letter-spacing:-.6px}.sub{color:#9caf9f;line-height:1.65;font-size:14px;margin:0 0 28px}.field{margin:19px 0}.field label{display:block;color:#d9e9dd;font-size:13px;font-weight:650;margin-bottom:9px}input[type=text],input[type=password]{width:100%;background:#0d1511;border:1px solid #42564a;border-radius:5px;color:#ebf7ef;padding:14px 15px;outline:none}input:focus{border-color:#93dda9;box-shadow:0 0 0 3px #7dde9f20}.help{font-size:12px;color:#8ca498;line-height:1.6;margin:9px 0}.notice{border:1px solid #344d3b;background:#18281e;padding:15px;border-radius:5px;color:#bcd3c2;font-size:13px;line-height:1.7}.notice strong{color:#f2fbf4}.btn{border:1px solid #526457;border-radius:5px;background:#18251e;color:#ebf7ef;padding:12px 18px;font-size:13px;font-weight:700}.primary{background:#96eab5;color:#0c2317;border-color:#96eab5}.btn:hover:not(:disabled){filter:brightness(1.11)}.foot{display:flex;justify-content:space-between;gap:12px;align-items:center;margin-top:31px;padding-top:22px;border-top:1px solid #2d3f34}.options{display:grid;grid-template-columns:1fr 1fr;gap:12px}.option{border:1px solid #3b4d42;border-radius:6px;padding:15px;background:#101914;display:flex;gap:12px;cursor:pointer}.option:has(input:checked){border-color:#87dba5;background:#172b1e}.option input{accent-color:#8be1ac}.option strong{display:block;font-size:14px}.option small{display:block;color:#91a899;margin-top:6px;line-height:1.5}.statgrid{display:grid;grid-template-columns:repeat(3,1fr);gap:10px;margin:0 0 25px}.stat{padding:17px;background:#19261e;border:1px solid #314838;border-radius:6px}.stat b{display:block;font-size:25px;margin-bottom:4px}.stat span{color:#9ab3a2;font-size:12px}.commit{padding:15px;border-bottom:1px solid #2c3e32;display:flex;align-items:flex-start;gap:12px}.commit:last-child{border:0}.commit input{margin-top:5px;accent-color:#8be1ac}.commit-title{font-size:13px;font-weight:700;overflow-wrap:anywhere}.meta{color:#8ca497;font-size:12px;margin-top:5px}.pill{display:inline-block;background:#20392a;color:#a1e2b4;border:1px solid #376043;border-radius:3px;padding:3px 6px;font-size:11px;margin:8px 5px 0 0}.code{font-family:ui-monospace,SFMono-Regular,Consolas,monospace;font-size:12px;white-space:pre-wrap;word-break:break-word;background:#0a120e;border:1px solid #304234;padding:15px;border-radius:5px;color:#afcbb4;max-height:250px;overflow:auto}.warning{border:1px solid #856e40;background:#2b2317;color:#edcea0;padding:15px;font-size:13px;line-height:1.7;border-radius:5px;margin:20px 0}.error{border:1px solid #a74e4e;background:#321c1c;color:#ffd0d0;padding:14px;border-radius:5px;margin-top:18px;font-size:13px;overflow-wrap:anywhere}.success{background:#142e20;border:1px solid #4c9464;padding:20px;border-radius:5px;line-height:1.7}.muted{color:#91a699}.hidden{display:none!important}.inlinecheck{display:flex;gap:10px;align-items:flex-start;margin:15px 0;color:#d2e3d5;font-size:13px;line-height:1.55}.inlinecheck input{accent-color:#8be1ac;margin-top:4px}.footer{text-align:center;color:#769083;font-size:12px;padding:0 25px 30px} @media(max-width:600px){.top{height:65px}.by,.toplinks a:nth-child(2){display:none}.hero{padding:55px 0 28px}.hero h1{letter-spacing:-1.5px}.wizard{margin-bottom:55px}.steps{padding:16px;gap:3px}.step{flex-direction:column;font-size:10px;text-align:center;gap:6px}.body{padding:22px 18px}.options{grid-template-columns:1fr}.statgrid{grid-template-columns:1fr 1fr}.foot{flex-wrap:wrap}}
</style><link rel="stylesheet" href="/styles/generated-tokens.css"><link rel="stylesheet" href="/styles/semantic-theme.css"><link rel="stylesheet" href="/styles/agentclean.css"><style>.steps{grid-template-columns:repeat(5,minmax(0,1fr));gap:8px}.repo-results{max-height:360px;overflow:auto;border:1px solid var(--border)}.repo-item{display:flex;width:100%;align-items:center;justify-content:space-between;gap:16px;text-align:left;padding:14px 16px;background:var(--background);border:0;border-bottom:1px solid var(--border);color:var(--foreground);cursor:pointer}.repo-item:last-child{border-bottom:0}.repo-item:hover,.repo-item:focus-visible{background:var(--cai-color-surface-container-low);outline:none}.repo-item strong{display:block;font-size:13px;font-weight:600;overflow-wrap:anywhere}.repo-item small{display:block;margin-top:4px;color:var(--muted-foreground);font-size:12px}.repo-arrow{color:var(--cai-color-text-action);font-size:18px}@media(max-width:600px){.steps{gap:2px;padding:14px 5px}.step{font-size:9px}.step .num{height:23px;width:23px}}</style><script src="/app.js" defer></script></head>
<body>
<div class="wrap"><header class="top"><a class="brand" href="/"><img src="https://sequence-flow.canadian-ai.app/icon-black.svg" alt="Canadian AI" width="30" height="30"> <span>AgentClean</span></a><nav class="toplinks"><span class="by">by Canadian AI Solutions</span><a href="https://github.com/canadian-ai/AgentClean" target="_blank" rel="noopener">GitHub ↗</a></nav></header>
<section class="hero"><div class="eyebrow">Canadian AI / Developer tools</div><h1>Clean up attribution,<br><span>without the busywork.</span></h1><p>Review and remove optional coding-agent signatures from your repository in a few guided steps. No installation required.</p></section>
`

const layoutEnd = `</div><div class="footer"><a href="https://canadian-ai.ca" target="_blank" rel="noopener">Canadian AI</a> · Open source from Canadian AI. Built in Montréal by Canadian AI.</div>
<footer aria-label="Canadian AI open-source attribution" style="margin:32px auto 0;padding:24px 20px;border-top:1px solid rgba(128,128,128,.3);max-width:1180px;display:flex;gap:18px;align-items:center;justify-content:space-between;flex-wrap:wrap;font:13px/1.5 system-ui,sans-serif;color:inherit">
  <div style="display:flex;align-items:center;gap:12px"><img src="https://sequence-flow.canadian-ai.app/icon-black.svg" alt="Canadian AI" width="22" height="22" style="background:white;padding:2px;border-radius:3px"/><div>Open source from Canadian AI.<div style="font-size:12px;opacity:.7">Built in Montréal by Canadian AI.</div></div></div>
  <a href="https://canadian-ai.ca" target="_blank" rel="noopener noreferrer" style="color:inherit;text-decoration:underline;text-underline-offset:3px">Canadian AI</a>
</footer>`

var homeTemplate = template.Must(template.New("home").Parse(layoutStart + wizardMain + layoutEnd + wizardTemplates + "</body></html>"))

var homePage = struct {
	Title string
	Steps []struct {
		Number int
		Label  string
	}
	Agents []struct{ ID, Name string }
}{
	Title: "AgentClean — Your code. Your commits.",
	Steps: []struct {
		Number int
		Label  string
	}{{1, "Connect"}, {2, "Repository"}, {3, "Scan"}, {4, "Review"}, {5, "Clean"}},
	Agents: []struct{ ID, Name string }{
		{"claude", "Claude Code"}, {"claude-session", "Claude session markers"}, {"codex", "OpenAI Codex"}, {"cursor", "Cursor"},
		{"gemini", "Gemini CLI"}, {"codesmith", "Codesmith"}, {"opencode", "OpenCode"},
	},
}

func (s *Server) Home(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body bytes.Buffer
	if err := homeTemplate.Execute(&body, homePage); err != nil {
		log.Printf("render AgentClean: %v", err)
		http.Error(w, "Unable to render AgentClean.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method == http.MethodGet {
		_, _ = body.WriteTo(w)
	}
}

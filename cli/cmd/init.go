package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/spf13/cobra"
)

type templateFile struct {
	Path    string
	Content string
}

func initCommand(ctx *commandContext) *cobra.Command {
	var site string
	cmd := &cobra.Command{
		Use:   "init [template] [path]",
		Short: "Create a static Flink prototype from a template",
		Args:  cobra.RangeArgs(0, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := "blank"
			if len(args) >= 1 {
				name = args[0]
			}
			files, ok := templates[name]
			if !ok {
				return fmt.Errorf("unknown template %q; available: %s", name, availableTemplates())
			}
			target := "."
			if len(args) == 2 {
				target = args[1]
			}
			slug := slugify(firstNonEmpty(site, filepath.Base(cleanLocalPath(target))))
			if slug == "" {
				slug = name
			}
			for _, file := range files {
				path := filepath.Join(target, filepath.FromSlash(file.Path))
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					return err
				}
				if _, err := os.Stat(path); err == nil {
					return fmt.Errorf("%s already exists", path)
				}
				if err := os.WriteFile(path, []byte(file.Content), 0644); err != nil {
					return err
				}
			}
			config := ctx.resolveConfig()
			if err := writeProjectConfig(target, flinkConfig{Site: slug, Server: config.Server, Tenant: config.Tenant}); err != nil {
				return err
			}
			if ctx.wantsJSON() {
				return ctx.writeJSON(cmd, map[string]any{"template": name, "path": target, "site": slug, "files": len(files)})
			}
			printSections(cmd.OutOrStdout(), "Template created",
				outputSection{Title: "Project", Rows: []outputRow{
					row("Template", name),
					row("Path", target),
					row("Site", slug),
					row("Files", len(files)),
				}},
				outputSection{Title: "Next", Rows: []outputRow{
					row("Publish", "flink publish"),
					row("Open", "flink open "+slug),
				}},
			)
			return nil
		},
	}
	cmd.Flags().StringVar(&site, "site", "", "site slug to save in .flink/site.json")
	return cmd
}

func availableTemplates() string {
	names := make([]string, 0, len(templates))
	for name := range templates {
		names = append(names, name)
	}
	sort.Strings(names)
	return fmt.Sprint(names)
}

var templates = map[string][]templateFile{
	"blank": {{Path: "index.html", Content: baseTemplate("Blank", `<main><h1>Blank Flink site</h1><p>Edit this file and run <code>flink publish</code>.</p></main>`, "")}},
	"todo": {{Path: "index.html", Content: baseTemplate("Todo", `<main><h1>Todo</h1><form id="form"><input id="text" placeholder="Task" autocomplete="off"><button>Add</button></form><ul id="list"></ul></main>`, `
const state = await flink.get("todos").catch(() => []);
function draw() { list.innerHTML = state.map((item, i) => '<li><label><input type="checkbox" data-i="'+i+'" '+(item.done?'checked':'')+'> '+item.text+'</label></li>').join(""); }
form.onsubmit = async (event) => { event.preventDefault(); if (!text.value.trim()) return; state.push({ text: text.value.trim(), done: false }); text.value = ""; await flink.set("todos", state); draw(); };
list.onclick = async (event) => { const box = event.target.closest("input"); if (!box) return; state[Number(box.dataset.i)].done = box.checked; await flink.set("todos", state); draw(); };
draw();`)}},
	"chat": {{Path: "index.html", Content: baseTemplate("Chat", `<main><h1>Chat</h1><ul id="log"></ul><form id="form"><input id="msg" autocomplete="off" autofocus><button>Send</button></form></main>`, `
const room = flink.room("chat", add);
function add(value) { const text = typeof value === "string" ? value : value.text; log.insertAdjacentHTML("beforeend", '<li>'+text+'</li>'); }
form.onsubmit = (event) => { event.preventDefault(); const text = msg.value.trim(); if (!text) return; msg.value = ""; add("me: " + text); room.send({ text: "peer: " + text }); };`)}},
	"dashboard": {{Path: "index.html", Content: baseTemplate("Dashboard", `<main><h1>Dashboard</h1><button id="refresh">Refresh</button><pre id="out"></pre></main>`, `
async function draw() { out.textContent = JSON.stringify(await flink.all().catch(() => ({})), null, 2); }
refresh.onclick = draw;
draw();`)}},
	"upload-gallery": {{Path: "index.html", Content: baseTemplate("Upload Gallery", `<main><h1>Upload Gallery</h1><input id="file" type="file" multiple><div id="gallery" class="gallery"></div></main>`, `
const saved = await flink.get("uploads").catch(() => []);
function draw() { gallery.innerHTML = saved.map(url => '<img src="'+url+'" alt="">').join(""); }
file.onchange = async () => { for (const item of file.files) { const uploaded = await flink.upload(item); saved.push(uploaded.url); } await flink.set("uploads", saved); draw(); };
draw();`)}},
	"ai-tool": {{Path: "index.html", Content: baseTemplate("AI Tool", `<main><h1>AI Tool</h1><textarea id="prompt" placeholder="Ask for a prototype idea"></textarea><button id="run">Run</button><pre id="out"></pre></main>`, `
run.onclick = async () => { out.textContent = "Working..."; const res = await flink.ai(prompt.value || "Suggest a tiny useful prototype"); out.textContent = res.text; };`)}},
	"multiplayer": {{Path: "index.html", Content: baseTemplate("Multiplayer", `<main><h1>Multiplayer Pointers</h1><div id="stage"></div></main>`, `
const id = Math.random().toString(16).slice(2);
const room = flink.room("pointers", ({ id: other, x, y }) => { if (other === id) return; let dot = document.getElementById(other); if (!dot) { dot = document.createElement("div"); dot.id = other; dot.className = "dot"; stage.append(dot); } dot.style.transform = 'translate('+x+'px,'+y+'px)'; });
stage.onpointermove = (event) => room.send({ id, x: event.offsetX, y: event.offsetY });`)}},
}

func baseTemplate(title, body, script string) string {
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>` + title + `</title>
  <script src="/flink.js"></script>
  <style>
    body { margin: 0; font-family: ui-sans-serif, system-ui, sans-serif; background: #f4f1ea; color: #191714; }
    main { width: min(760px, calc(100vw - 32px)); margin: 40px auto; display: grid; gap: 16px; }
    input, textarea, button { font: inherit; border: 1px solid #b8b1a5; border-radius: 6px; padding: 10px 12px; }
    button { background: #0f766e; color: white; border-color: #0f766e; font-weight: 650; cursor: pointer; }
    form { display: flex; gap: 8px; }
    form input { flex: 1; }
    pre, ul, #stage { background: white; border: 1px solid #d6d0c4; border-radius: 8px; padding: 16px; min-height: 120px; }
    textarea { min-height: 140px; resize: vertical; }
    .gallery { display: grid; grid-template-columns: repeat(auto-fill, minmax(160px, 1fr)); gap: 12px; }
    .gallery img { width: 100%; aspect-ratio: 1; object-fit: cover; border-radius: 8px; }
    #stage { position: relative; height: 380px; overflow: hidden; touch-action: none; }
    .dot { position: absolute; width: 18px; height: 18px; border-radius: 99px; background: #e11d48; pointer-events: none; }
  </style>
</head>
<body>
` + body + `
  <script type="module">
` + script + `
  </script>
</body>
</html>
`
}

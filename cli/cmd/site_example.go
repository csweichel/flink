package cmd

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func siteExampleCommand(serverURL, username, password *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "example [slug] [name]",
		Short: "Publish a built-in example site",
		Long: `Publish a built-in example site.

Run without arguments to see the available examples. To publish one, pass the
target site slug and example name.`,
		Example: `  flink site example
  flink site example my-chat chat
  flink site example my-gallery upload`,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || len(args) == 2 {
				return nil
			}
			return fmt.Errorf("expected no arguments to list examples, or <slug> <name> to publish one")
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}

			slug := args[0]
			name := strings.ToLower(strings.TrimSpace(args[1]))
			content, ok := exampleSites[name]
			if !ok {
				return fmt.Errorf("unknown example %q", args[1])
			}
			c, err := newClient(*serverURL, *username, *password)
			if err != nil {
				return err
			}
			var meta siteMeta
			if err := c.doJSON(http.MethodPost, "/api/sites", map[string]string{"slug": slug}, &meta); err != nil {
				return err
			}
			path := fmt.Sprintf("/api/sites/%s/files?path=%s", url.PathEscape(slug), url.QueryEscape("index.html"))
			var out map[string]string
			if err := c.doBytes(http.MethodPut, path, []byte(content), "text/html; charset=utf-8", &out); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "published %s example to %s\n", name, c.siteURL(slug))
			return nil
		},
	}
	cmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		cmd.Root().HelpFunc()(cmd, args)
		printAvailableExamples(cmd)
	})
	return cmd
}

func printAvailableExamples(cmd *cobra.Command) {
	fmt.Fprintln(cmd.OutOrStdout())
	fmt.Fprintln(cmd.OutOrStdout(), "Available examples:")
	for _, name := range exampleNames() {
		fmt.Fprintf(cmd.OutOrStdout(), "  %-8s %s\n", name, exampleDescriptions[name])
	}
}

func exampleNames() []string {
	names := make([]string, 0, len(exampleSites))
	for name := range exampleSites {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

var exampleDescriptions = map[string]string{
	"chat":    "Realtime chat over Flink WebSockets",
	"data":    "Shared counter using Flink JSON storage",
	"library": "Browser library import plus the AI endpoint",
	"upload":  "File upload, saved URLs, and image display",
}

var exampleSites = map[string]string{
	"upload": `<!doctype html>
<html>
<head><meta name="viewport" content="width=device-width, initial-scale=1"><script src="/flink.js"></script></head>
<body>
  <h1>Upload and display</h1>
  <input id="file" type="file">
  <div id="gallery"></div>
  <script>
    file.onchange = async () => {
      const uploaded = await flink.upload(file.files[0]);
      const saved = await flink.get("uploads").catch(() => []);
      saved.push(uploaded.url);
      await flink.set("uploads", saved);
      render(saved);
    };
    function render(urls) {
      gallery.innerHTML = urls.map(url => '<img src="'+url+'" style="max-width:220px;margin:8px">').join("");
    }
    flink.get("uploads").then(render).catch(() => render([]));
  </script>
</body>
</html>`,
	"data": `<!doctype html>
<html>
<head><meta name="viewport" content="width=device-width, initial-scale=1"><script src="/flink.js"></script></head>
<body>
  <h1>Shared counter</h1>
  <button id="plus">Increment</button>
  <pre id="out"></pre>
  <script>
    async function draw() {
      out.textContent = JSON.stringify(await flink.get("counter").catch(() => ({ count: 0 })), null, 2);
    }
    plus.onclick = async () => {
      const state = await flink.get("counter").catch(() => ({ count: 0 }));
      state.count++;
      state.updatedAt = new Date().toISOString();
      await flink.set("counter", state);
      draw();
    };
    draw();
  </script>
</body>
</html>`,
	"chat": `<!doctype html>
<html>
<head><meta name="viewport" content="width=device-width, initial-scale=1"><script src="/flink.js"></script></head>
<body>
  <h1>Realtime chat</h1>
  <form id="form"><input id="msg" autocomplete="off" autofocus><button>Send</button></form>
  <ul id="log"></ul>
  <script>
    const room = flink.room("chat", add);
    function add(m) { log.innerHTML += "<li>" + (m.text || m) + "</li>"; }
    form.onsubmit = (e) => {
      e.preventDefault();
      const text = msg.value;
      msg.value = "";
      add({ text: "me: " + text });
      room.send({ text: "peer: " + text });
    };
  </script>
</body>
</html>`,
	"library": `<!doctype html>
<html>
<head><meta name="viewport" content="width=device-width, initial-scale=1"><script src="/flink.js"></script></head>
<body>
  <h1>Shared library import</h1>
  <button id="run">Run AI placeholder</button>
  <pre id="out"></pre>
  <script type="module">
    import confetti from "https://cdn.jsdelivr.net/npm/canvas-confetti@1.9.3/+esm";
    run.onclick = async () => {
      confetti();
      out.textContent = (await flink.ai("Give me a prototype idea")).text;
    };
  </script>
</body>
</html>`,
}

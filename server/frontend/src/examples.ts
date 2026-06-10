export const examples: Record<string, string> = {
  upload: `<!doctype html>
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
  data: `<!doctype html>
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
  chat: `<!doctype html>
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
  library: `<!doctype html>
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
</html>`
};

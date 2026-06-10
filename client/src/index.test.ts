import assert from "node:assert/strict";
import test from "node:test";
import { createFlinkClient, FlinkError } from "./index";

interface FetchCall {
  url: string;
  init?: RequestInit;
}

test("storage APIs call tenant site public endpoints with credentials", async () => {
  const calls: FetchCall[] = [];
  const fetchMock = async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ url: String(input), init });
    return jsonResponse({ text: "saved" });
  };

  const flink = createFlinkClient({
    tenant: "alice",
    site: "demo",
    baseUrl: "https://flink.internal",
    fetch: fetchMock as typeof fetch,
  });

  const value = await flink.storage.set("note", { text: "saved" });

  assert.deepEqual(value, { text: "saved" });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].url, "https://flink.internal/api/public/demo/data/note");
  assert.equal(calls[0].init?.method, "PUT");
  assert.equal(calls[0].init?.credentials, "same-origin");
  assert.equal(calls[0].init?.body, JSON.stringify({ text: "saved" }));
  assert.equal(new Headers(calls[0].init?.headers).get("content-type"), "application/json");
  assert.equal(flink.files.url("index.html"), "https://flink.internal/t/alice/s/demo/index.html");
});

test("uploads post multipart data and expose upload helpers", async () => {
  const calls: FetchCall[] = [];
  const fetchMock = async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ url: String(input), init });
    if (String(input).endsWith("/uploads")) {
      assert.equal(init?.method, "POST");
      assert.ok(init?.body instanceof FormData);
      return jsonResponse({ url: "/uploads/alice/demo/file.txt", name: "file.txt" });
    }
    return new Response("uploaded text");
  };
  const flink = createFlinkClient({
    tenant: "alice",
    site: "demo",
    baseUrl: "https://flink.internal",
    fetch: fetchMock as typeof fetch,
  });

  const uploaded = await flink.upload(new Blob(["hello"], { type: "text/plain" }), { filename: "file.txt" });
  const text = await flink.uploads.text(uploaded);

  assert.deepEqual(uploaded, { url: "/uploads/alice/demo/file.txt", name: "file.txt" });
  assert.equal(text, "uploaded text");
  assert.equal(calls[0].url, "https://flink.internal/api/public/demo/uploads");
  assert.equal(calls[1].url, "https://flink.internal/uploads/alice/demo/file.txt");
});

test("file APIs list, write, and delete site files", async () => {
  const calls: FetchCall[] = [];
  const fetchMock = async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push({ url: String(input), init });
    if (String(input).includes("prefix=assets")) {
      return jsonResponse([{ path: "assets/app.css", size: 15 }]);
    }
    if (init?.method === "DELETE") {
      return jsonResponse({ deleted: true });
    }
    return jsonResponse({ path: "assets/app.css" });
  };
  const flink = createFlinkClient({
    site: "demo",
    baseUrl: "https://flink.internal",
    fetch: fetchMock as typeof fetch,
  });

  const files = await flink.files.list("assets");
  await flink.files.write("assets/app.css", "body{color:red}");
  const deleted = await flink.files.delete("assets/app.css");

  assert.deepEqual(files, [{ path: "assets/app.css", size: 15 }]);
  assert.deepEqual(deleted, { deleted: true });
  assert.equal(calls[0].url, "https://flink.internal/api/public/demo/files?prefix=assets");
  assert.equal(calls[1].url, "https://flink.internal/api/public/demo/files?path=assets%2Fapp.css");
  assert.equal(calls[1].init?.method, "PUT");
  assert.equal(calls[1].init?.body, "body{color:red}");
  assert.equal(calls[2].url, "https://flink.internal/api/public/demo/files?path=assets%2Fapp.css");
  assert.equal(calls[2].init?.method, "DELETE");
});

test("AI accepts string prompt and returns typed response", async () => {
  let requestBody = "";
  const fetchMock = async (_input: RequestInfo | URL, init?: RequestInit) => {
    requestBody = String(init?.body);
    return jsonResponse({ text: "idea", model: "mock", configured: true });
  };
  const flink = createFlinkClient({ site: "demo", fetch: fetchMock as typeof fetch });

  const response = await flink.ai("Give me an idea");

  assert.deepEqual(JSON.parse(requestBody), { prompt: "Give me an idea" });
  assert.deepEqual(response, { text: "idea", model: "mock", configured: true });
});

test("failed API responses throw FlinkError with status and body", async () => {
  const fetchMock = async () => jsonResponse({ error: "nope" }, { status: 403, statusText: "Forbidden" });
  const flink = createFlinkClient({ site: "demo", fetch: fetchMock as typeof fetch });

  await assert.rejects(() => flink.get("secret"), (err) => {
    assert.ok(err instanceof FlinkError);
    assert.equal(err.status, 403);
    assert.deepEqual(err.body, { error: "nope" });
    assert.equal(err.message, "nope");
    return true;
  });
});

test("realtime rooms create WebSocket URLs, parse messages, and send JSON", () => {
  const sockets: MockSocket[] = [];
  class TestSocket extends MockSocket {
    constructor(url: string) {
      super(url);
      sockets.push(this);
    }
  }
  const received: unknown[] = [];
  const flink = createFlinkClient({
    site: "demo",
    baseUrl: "https://flink.internal",
    fetch: (async () => jsonResponse({})) as typeof fetch,
    WebSocket: TestSocket as unknown as typeof WebSocket,
  });

  const room = flink.room("chat", (message) => received.push(message));
  room.send({ text: "hi" });
  sockets[0].emit("message", { data: `{"text":"peer"}` } as MessageEvent);

  assert.equal(sockets[0].url, "wss://flink.internal/ws/demo/chat");
  assert.deepEqual(sockets[0].sent, [`{"text":"hi"}`]);
  assert.deepEqual(received, [{ text: "peer" }]);
});

test("infers tenant and site from canonical browser URL", () => {
  const previousLocation = globalThis.location;
  Object.defineProperty(globalThis, "location", {
    configurable: true,
    value: new URL("https://flink.internal/t/alice/s/demo/index.html"),
  });
  try {
    const flink = createFlinkClient({ fetch: (async () => jsonResponse({})) as typeof fetch });
    assert.equal(flink.tenant, "alice");
    assert.equal(flink.site, "demo");
    assert.equal(flink.files.url(), "/t/alice/s/demo/index.html");
  } finally {
    Object.defineProperty(globalThis, "location", { configurable: true, value: previousLocation });
  }
});

function jsonResponse(body: unknown, init: ResponseInit = {}) {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
    ...init,
  });
}

class MockSocket {
  readonly url: string;
  readonly sent: string[] = [];
  private handlers = new Map<string, Set<EventListener>>();

  constructor(url: string) {
    this.url = url;
  }

  send(value: string) {
    this.sent.push(value);
  }

  close() {}

  addEventListener(type: string, handler: EventListener) {
    if (!this.handlers.has(type)) {
      this.handlers.set(type, new Set());
    }
    this.handlers.get(type)?.add(handler);
  }

  emit(type: string, event: MessageEvent) {
    this.handlers.get(type)?.forEach((handler) => handler(event));
  }
}

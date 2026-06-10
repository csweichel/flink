import React, { FormEvent, useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { examples } from "./examples";
import "./index.css";

const defaultPath = "index.html";

interface SiteMeta {
  slug: string;
  title: string;
  createdAt: string;
  updatedAt: string;
}

interface SiteFile {
  path: string;
  content: string;
}

interface Tenant {
  username: string;
  status: string;
}

type RequestOptions = RequestInit & {
  headers?: HeadersInit;
};

function App() {
  const [sites, setSites] = useState<SiteMeta[]>([]);
  const [current, setCurrent] = useState("");
  const [slug, setSlug] = useState("");
  const [filePath, setFilePath] = useState(defaultPath);
  const [content, setContent] = useState("");
  const [status, setStatus] = useState("");
  const [tenant, setTenant] = useState<Tenant | null>(null);

  const currentSite = useMemo(() => sites.find((site) => site.slug === current), [sites, current]);

  useEffect(() => {
    refresh().catch((err: Error) => setStatus(err.message));
  }, []);

  async function api<T>(url: string, options: RequestOptions = {}): Promise<T> {
    const headers = new Headers({ "content-type": "application/json" });
    new Headers(options.headers).forEach((value, key) => headers.set(key, value));
    const res = await fetch(url, {
      ...options,
      headers,
    });
    if (!res.ok) {
      const error = (await res.json().catch(() => ({ error: res.statusText }))) as { error?: string };
      throw new Error(error.error || res.statusText);
    }
    return (await res.json()) as T;
  }

  async function refresh() {
    const me = await api<Tenant>("/api/auth/me");
    setTenant(me);
    const nextSites = (await api<SiteMeta[] | null>("/api/sites")) ?? [];
    const safeSites = Array.isArray(nextSites) ? nextSites : [];
    setSites(safeSites);
    if (!current && safeSites[0]) {
      await selectSite(safeSites[0].slug);
    }
  }

  async function createSite(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const clean = slug.trim();
    if (!clean) {
      return;
    }
    await api<SiteMeta>("/api/sites", { method: "POST", body: JSON.stringify({ slug: clean }) });
    setSlug("");
    await selectSite(clean);
    await refresh();
  }

  async function selectSite(nextSlug: string) {
    setCurrent(nextSlug);
    const p = filePath || defaultPath;
    const out = await api<SiteFile>(`/api/sites/${nextSlug}/files?path=${encodeURIComponent(p)}`);
    setContent(out.content);
    setStatus(`Loaded ${p}`);
  }

  async function loadFile() {
    if (!current) {
      return;
    }
    const p = filePath || defaultPath;
    const out = await api<SiteFile>(`/api/sites/${current}/files?path=${encodeURIComponent(p)}`);
    setContent(out.content);
    setStatus(`Loaded ${p}`);
  }

  async function saveFile() {
    if (!current) {
      return;
    }
    const p = filePath || defaultPath;
    await api<{ path: string }>(`/api/sites/${current}/files?path=${encodeURIComponent(p)}`, {
      method: "PUT",
      body: JSON.stringify({ content }),
    });
    setStatus(`Saved. Live at ${siteURL(current)}`);
    await refresh();
  }

  function loadExample(key: string) {
    if (!key) {
      return;
    }
    setFilePath(defaultPath);
    setContent(examples[key] ?? "");
    setStatus("Example loaded. Save to publish.");
  }

  function siteURL(siteSlug: string) {
    return tenant ? `/t/${tenant.username}/s/${siteSlug}/` : `/s/${siteSlug}/`;
  }

  return (
    <div className="min-h-screen bg-stone-100 text-neutral-950">
      <header className="sticky top-0 z-10 flex h-14 items-center justify-between border-b border-stone-300 bg-white/90 px-4 backdrop-blur">
        <a className="flex min-w-0 items-center gap-2" href="/_flink" aria-label="Flink dashboard">
          <img className="h-9 w-9 shrink-0 object-contain" src="/flink-logo.png" alt="" />
          <span className="text-lg font-semibold">Flink</span>
        </a>
        <div className="flex items-center gap-3 text-sm">
          <span className="font-medium text-neutral-600">{tenant?.username}</span>
          <a className="font-medium text-teal-700 hover:text-teal-900" href={current ? siteURL(current) : "#"} target="_blank" rel="noreferrer">
            Open site
          </a>
          <a className="font-medium text-neutral-600 hover:text-neutral-950" href="/_flink/logout">
            Sign out
          </a>
        </div>
      </header>

      <main className="grid min-h-[calc(100vh-3.5rem)] grid-cols-1 md:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="border-b border-stone-300 p-3 md:border-b-0 md:border-r">
          <form className="flex gap-2" onSubmit={createSite}>
            <input
              className="min-w-0 flex-1 rounded-md border border-stone-300 bg-white px-3 py-2 text-sm outline-none focus:border-teal-700"
              value={slug}
              pattern="[a-z0-9-]+"
              placeholder="new-site"
              onChange={(event) => setSlug(event.target.value)}
              required
            />
            <button className="rounded-md border border-teal-700 bg-teal-700 px-3 py-2 text-sm font-medium text-white" type="submit">
              Create
            </button>
          </form>
          <p className="mt-3 text-xs leading-5 text-neutral-600">
            Tenant hosting works at <code>/t/tenant/s/site-name/</code>. Signed-in shorthand works at <code>/s/site-name/</code>.
          </p>
          <div className="mt-4 space-y-2">
            {sites.map((site) => (
              <button
                key={site.slug}
                className={`block w-full rounded-md border bg-white p-3 text-left text-sm ${
                  site.slug === current ? "border-teal-700 shadow-[inset_3px_0_0_#0f766e]" : "border-stone-300"
                }`}
                type="button"
                onClick={() => selectSite(site.slug).catch((err: Error) => setStatus(err.message))}
              >
                <span className="block font-semibold">{site.slug}</span>
                <span className="block text-xs text-neutral-600">{new Date(site.updatedAt).toLocaleString()}</span>
              </button>
            ))}
          </div>
        </aside>

        <section className="grid min-h-[640px] grid-rows-[auto_minmax(0,1fr)_auto] gap-3 p-3">
          <div className="flex flex-wrap items-center gap-2">
            <strong className="mr-2 text-sm">{currentSite?.slug || "No site selected"}</strong>
            <input
              className="w-44 rounded-md border border-stone-300 bg-white px-3 py-2 text-sm outline-none focus:border-teal-700"
              value={filePath}
              onChange={(event) => setFilePath(event.target.value)}
            />
            <button className="rounded-md border border-stone-300 bg-white px-3 py-2 text-sm" type="button" onClick={() => loadFile().catch((err: Error) => setStatus(err.message))}>
              Load
            </button>
            <button className="rounded-md border border-teal-700 bg-teal-700 px-3 py-2 text-sm font-medium text-white" type="button" onClick={() => saveFile().catch((err: Error) => setStatus(err.message))}>
              Save
            </button>
            <select className="rounded-md border border-stone-300 bg-white px-3 py-2 text-sm" defaultValue="" onChange={(event) => loadExample(event.target.value)}>
              <option value="">Examples...</option>
              <option value="upload">File upload</option>
              <option value="data">Data save/load</option>
              <option value="chat">Realtime chat</option>
              <option value="library">Shared library import</option>
            </select>
          </div>
          <textarea
            className="h-full min-h-[520px] resize-none rounded-md border border-stone-300 bg-neutral-950 p-4 font-mono text-sm leading-6 text-stone-50 outline-none"
            value={content}
            spellCheck={false}
            onChange={(event) => setContent(event.target.value)}
          />
          <div className="min-h-6 text-sm text-neutral-600">{status}</div>
        </section>
      </main>
    </div>
  );
}

const root = document.getElementById("root");
if (!root) {
  throw new Error("missing root element");
}

createRoot(root).render(<App />);

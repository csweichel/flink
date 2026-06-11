import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";

interface SiteMeta {
  slug: string;
  title: string;
  createdAt: string;
  updatedAt: string;
}

interface SiteFileInfo {
  path: string;
  size: number;
}

interface UploadInfo {
  name: string;
  url: string;
  size: number;
}

interface Tenant {
  username: string;
  status: string;
}

interface SiteDetails {
  files: SiteFileInfo[];
  data: Record<string, unknown>;
  uploads: UploadInfo[];
}

type RequestOptions = RequestInit & {
  headers?: HeadersInit;
};

function App() {
  const [tenant, setTenant] = useState<Tenant | null>(null);
  const [sites, setSites] = useState<SiteMeta[]>([]);
  const [selected, setSelected] = useState("");
  const [details, setDetails] = useState<SiteDetails | null>(null);
  const [status, setStatus] = useState("");

  const selectedSite = useMemo(() => sites.find((site) => site.slug === selected), [sites, selected]);

  useEffect(() => {
    refresh().catch((err: Error) => setStatus(err.message));
  }, []);

  useEffect(() => {
    if (!selected) {
      setDetails(null);
      return;
    }
    loadDetails(selected).catch((err: Error) => setStatus(err.message));
  }, [selected]);

  async function api<T>(url: string, options: RequestOptions = {}): Promise<T> {
    const headers = new Headers();
    if (options.body && !(options.body instanceof FormData)) {
      headers.set("content-type", "application/json");
    }
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
    if (safeSites.length === 0) {
      setSelected("");
      setStatus("No sites published yet. Use the Flink CLI to publish one.");
      return;
    }
    if (!selected || !safeSites.some((site) => site.slug === selected)) {
      setSelected(safeSites[0].slug);
    }
  }

  async function loadDetails(siteSlug: string) {
    setStatus(`Loading ${siteSlug}...`);
    const [files, data, uploads] = await Promise.all([
      api<SiteFileInfo[]>(`/api/sites/${siteSlug}/files`),
      api<Record<string, unknown>>(`/api/sites/${siteSlug}/data/`),
      api<UploadInfo[]>(`/api/sites/${siteSlug}/uploads`),
    ]);
    setDetails({
      files: Array.isArray(files) ? files : [],
      data: data ?? {},
      uploads: Array.isArray(uploads) ? uploads : [],
    });
    setStatus(`Loaded ${siteSlug}.`);
  }

  async function deleteSite(siteSlug: string) {
    if (!window.confirm(`Delete "${siteSlug}" and all of its files, state, and uploads?`)) {
      return;
    }
    await api<{ deleted: true }>(`/api/sites/${siteSlug}`, { method: "DELETE" });
    const remaining = sites.filter((site) => site.slug !== siteSlug);
    setSites(remaining);
    setSelected(remaining[0]?.slug ?? "");
    setStatus(`Deleted ${siteSlug}.`);
  }

  async function deleteUpload(upload: UploadInfo) {
    if (!selected || !window.confirm(`Delete upload "${upload.name}"?`)) {
      return;
    }
    await api<{ deleted: true }>(`/api/sites/${selected}/uploads?name=${encodeURIComponent(upload.name)}`, { method: "DELETE" });
    await loadDetails(selected);
    setStatus(`Deleted upload ${upload.name}.`);
  }

  function siteURL(siteSlug: string) {
    return tenant ? `/t/${tenant.username}/s/${siteSlug}/` : `/s/${siteSlug}/`;
  }

  function siteFileURL(siteSlug: string, path: string) {
    if (path === "index.html") {
      return siteURL(siteSlug);
    }
    return siteURL(siteSlug) + path.split("/").map(encodeURIComponent).join("/");
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
          <a className="font-medium text-neutral-600 hover:text-neutral-950" href="/_flink/logout">
            Sign out
          </a>
        </div>
      </header>

      <main className="mx-auto grid w-full max-w-7xl gap-5 p-4 md:p-6">
        <section className="grid gap-3">
          <div className="flex flex-wrap items-end justify-between gap-3">
            <div>
              <h1 className="text-xl font-semibold">Sites</h1>
              <p className="mt-1 text-sm text-neutral-600">Published sites for this tenant. Use the CLI to create or update content.</p>
            </div>
            <button className="rounded-md border border-stone-300 bg-white px-3 py-2 text-sm font-medium" type="button" onClick={() => refresh().catch((err: Error) => setStatus(err.message))}>
              Refresh
            </button>
          </div>

          <div className="overflow-hidden rounded-md border border-stone-300 bg-white">
            <table className="w-full border-collapse text-left text-sm">
              <thead className="bg-stone-50 text-xs uppercase text-neutral-500">
                <tr>
                  <th className="px-3 py-2 font-semibold">Site</th>
                  <th className="px-3 py-2 font-semibold">Updated</th>
                  <th className="px-3 py-2 font-semibold">Actions</th>
                </tr>
              </thead>
              <tbody>
                {sites.map((site) => (
                  <tr key={site.slug} className={site.slug === selected ? "bg-teal-50" : "border-t border-stone-200"}>
                    <td className="px-3 py-3">
                      <button className="font-semibold text-teal-800 hover:text-teal-950" type="button" onClick={() => setSelected(site.slug)}>
                        {site.slug}
                      </button>
                    </td>
                    <td className="px-3 py-3 text-neutral-600">{formatDate(site.updatedAt)}</td>
                    <td className="px-3 py-3">
                      <div className="flex flex-wrap gap-2">
                        <button className="rounded-md border border-stone-300 bg-white px-3 py-1.5 font-medium" type="button" onClick={() => setSelected(site.slug)}>
                          Inspect
                        </button>
                        <a className="rounded-md border border-stone-300 bg-white px-3 py-1.5 font-medium" href={siteURL(site.slug)} target="_blank" rel="noreferrer">
                          Visit
                        </a>
                        <a className="rounded-md border border-stone-300 bg-white px-3 py-1.5 font-medium" href={`/api/sites/${site.slug}/archive`}>
                          Download
                        </a>
                        <button className="rounded-md border border-red-200 bg-white px-3 py-1.5 font-medium text-red-700" type="button" onClick={() => deleteSite(site.slug).catch((err: Error) => setStatus(err.message))}>
                          Delete
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
                {sites.length === 0 ? (
                  <tr>
                    <td className="px-3 py-8 text-center text-neutral-600" colSpan={3}>
                      No sites published yet.
                    </td>
                  </tr>
                ) : null}
              </tbody>
            </table>
          </div>
        </section>

        <section className="grid gap-4">
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h2 className="text-lg font-semibold">{selectedSite?.slug ?? "No site selected"}</h2>
              {selectedSite ? <p className="text-sm text-neutral-600">Created {formatDate(selectedSite.createdAt)}</p> : null}
            </div>
            {selectedSite ? (
              <div className="flex flex-wrap gap-2">
                <a className="rounded-md border border-stone-300 bg-white px-3 py-2 text-sm font-medium" href={siteURL(selectedSite.slug)} target="_blank" rel="noreferrer">
                  Visit site
                </a>
                <a className="rounded-md border border-teal-700 bg-teal-700 px-3 py-2 text-sm font-medium text-white" href={`/api/sites/${selectedSite.slug}/archive`}>
                  Download
                </a>
              </div>
            ) : null}
          </div>

          {selectedSite && details ? (
            <div className="grid gap-4">
              <FilesTable files={details.files} siteSlug={selectedSite.slug} fileURL={siteFileURL} />
              <DataTable data={details.data} />
              <UploadsTable uploads={details.uploads} onDelete={(upload) => deleteUpload(upload).catch((err: Error) => setStatus(err.message))} />
            </div>
          ) : null}
        </section>

        <div className="min-h-6 text-sm text-neutral-600" role="status">
          {status}
        </div>
      </main>
    </div>
  );
}

function FilesTable({ files, siteSlug, fileURL }: { files: SiteFileInfo[]; siteSlug: string; fileURL: (siteSlug: string, path: string) => string }) {
  return (
    <Table title="Hosted Files" empty="No hosted files.">
      {files.map((file) => (
        <tr key={file.path} className="border-t border-stone-200">
          <td className="px-3 py-2 font-mono text-xs">{file.path}</td>
          <td className="px-3 py-2 text-right text-neutral-600">{formatBytes(file.size)}</td>
          <td className="px-3 py-2 text-right">
            <a className="font-medium text-teal-700 hover:text-teal-950" href={fileURL(siteSlug, file.path)} target="_blank" rel="noreferrer">
              Download
            </a>
          </td>
        </tr>
      ))}
    </Table>
  );
}

function DataTable({ data }: { data: Record<string, unknown> }) {
  const entries = Object.entries(data).sort(([a], [b]) => a.localeCompare(b));
  return (
    <Table title="State / DB" empty="No state stored.">
      {entries.map(([key, value]) => (
        <tr key={key} className="border-t border-stone-200 align-top">
          <td className="w-56 px-3 py-2 font-mono text-xs">{key}</td>
          <td className="px-3 py-2">
            <pre className="max-h-44 overflow-auto whitespace-pre-wrap break-words rounded bg-stone-50 p-2 font-mono text-xs leading-5 text-neutral-800">{JSON.stringify(value, null, 2)}</pre>
          </td>
          <td className="px-3 py-2 text-right text-neutral-500">{formatBytes(JSON.stringify(value).length)}</td>
        </tr>
      ))}
    </Table>
  );
}

function UploadsTable({ uploads, onDelete }: { uploads: UploadInfo[]; onDelete: (upload: UploadInfo) => void }) {
  return (
    <Table title="Uploads" empty="No uploaded files.">
      {uploads.map((upload) => (
        <tr key={upload.name} className="border-t border-stone-200">
          <td className="px-3 py-2 font-mono text-xs">{upload.name}</td>
          <td className="px-3 py-2 text-right text-neutral-600">{formatBytes(upload.size)}</td>
          <td className="px-3 py-2 text-right">
            <div className="flex justify-end gap-3">
              <a className="font-medium text-teal-700 hover:text-teal-950" href={upload.url} target="_blank" rel="noreferrer">
                Download
              </a>
              <button className="font-medium text-red-700 hover:text-red-900" type="button" onClick={() => onDelete(upload)}>
                Delete
              </button>
            </div>
          </td>
        </tr>
      ))}
    </Table>
  );
}

function Table({ title, empty, children }: { title: string; empty: string; children: React.ReactNode }) {
  const rows = React.Children.count(children);
  return (
    <div className="overflow-hidden rounded-md border border-stone-300 bg-white">
      <div className="border-b border-stone-200 bg-stone-50 px-3 py-2 text-sm font-semibold">{title}</div>
      <table className="w-full border-collapse text-left text-sm">
        <tbody>
          {rows > 0 ? (
            children
          ) : (
            <tr>
              <td className="px-3 py-5 text-center text-neutral-600">{empty}</td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function formatDate(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString();
}

function formatBytes(value: number) {
  if (!Number.isFinite(value)) {
    return "-";
  }
  if (value < 1024) {
    return `${value} B`;
  }
  if (value < 1024 * 1024) {
    return `${(value / 1024).toFixed(1)} KB`;
  }
  return `${(value / 1024 / 1024).toFixed(1)} MB`;
}

const root = document.getElementById("root");
if (!root) {
  throw new Error("missing root element");
}

createRoot(root).render(<App />);

import React, { useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";

interface SiteMeta {
  slug: string;
  title: string;
  auth: SiteAuthPolicy;
  createdAt: string;
  updatedAt: string;
}

interface SiteAuthPolicy {
  mode: "owner" | "none" | "tenants";
  tenants?: string[];
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
  baseHost?: string;
  dropTenantDomainPrefix?: boolean;
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
  const [setupOpen, setSetupOpen] = useState(false);

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
    if (!tenant) {
      return `/s/${siteSlug}/`;
    }
    if (tenant.baseHost) {
      if (tenant.dropTenantDomainPrefix !== false) {
        return `${window.location.protocol}//${siteSlug}.${tenant.baseHost}/`;
      }
      return `${window.location.protocol}//${tenant.username}--${siteSlug}.${tenant.baseHost}/`;
    }
    return `/t/${tenant.username}/s/${siteSlug}/`;
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
          <button className="rounded-md border border-stone-300 bg-white px-3 py-1.5 font-medium text-neutral-700 hover:text-neutral-950" type="button" onClick={() => setSetupOpen(true)}>
            Agent setup
          </button>
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
                  <th className="px-3 py-2 font-semibold">Access</th>
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
                    <td className="px-3 py-3 text-neutral-600">{formatAuth(site.auth)}</td>
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
                    <td className="px-3 py-8 text-center text-neutral-600" colSpan={4}>
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
      {setupOpen && tenant ? <AgentSetupModal tenant={tenant} onClose={() => setSetupOpen(false)} /> : null}
    </div>
  );
}

function AgentSetupModal({ tenant, onClose }: { tenant: Tenant; onClose: () => void }) {
  const [active, setActive] = useState<"plugin" | "mcp" | "skill">("plugin");
  const [copied, setCopied] = useState("");
  const origin = window.location.origin;
  const mcpURL = `${origin}/mcp`;
  const snippets = setupSnippets(origin, mcpURL, tenant.username);

  async function copy(label: string, value: string) {
    await navigator.clipboard.writeText(value);
    setCopied(label);
    window.setTimeout(() => setCopied(""), 1800);
  }

  const panels = {
    mcp: {
      title: "MCP server",
      text: "Connect Codex or another MCP client directly to this Flink tenant.",
      value: snippets.mcp,
      label: "MCP config",
    },
    skill: {
      title: "Codex skill",
      text: "Save this as a Flink skill so Codex knows when and how to use the server.",
      value: snippets.skill,
      label: "Skill",
    },
    plugin: {
      title: "Plugin starter",
      text: "Use this as a small local plugin scaffold that bundles the skill and MCP setup notes.",
      value: snippets.plugin,
      label: "Plugin",
    },
  };
  const panel = panels[active];

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/35 p-4" role="dialog" aria-modal="true" aria-labelledby="agent-setup-title">
      <div className="grid max-h-[min(760px,calc(100vh-32px))] w-full max-w-3xl overflow-hidden rounded-md border border-stone-300 bg-white shadow-xl">
        <div className="flex items-start justify-between gap-4 border-b border-stone-200 px-4 py-3">
          <div>
            <h2 id="agent-setup-title" className="text-lg font-semibold">
              Agent setup
            </h2>
            <p className="mt-1 text-sm text-neutral-600">Tenant {tenant.username} on {origin}</p>
          </div>
          <button className="rounded-md border border-stone-300 bg-white px-3 py-1.5 text-sm font-medium" type="button" onClick={onClose} autoFocus>
            Close
          </button>
        </div>

        <div className="grid gap-4 overflow-auto p-4">
          <div className="flex flex-wrap gap-2" role="tablist" aria-label="Agent setup options">
            <SetupTab active={active === "plugin"} onClick={() => setActive("plugin")}>
              Plugin
            </SetupTab>
            <SetupTab active={active === "mcp"} onClick={() => setActive("mcp")}>
              MCP
            </SetupTab>
            <SetupTab active={active === "skill"} onClick={() => setActive("skill")}>
              Skill
            </SetupTab>
          </div>

          <section className="grid gap-3">
            <div className="flex flex-wrap items-start justify-between gap-3">
              <div>
                <h3 className="text-base font-semibold">{panel.title}</h3>
                <p className="mt-1 max-w-2xl text-sm text-neutral-600">{panel.text}</p>
              </div>
              <button className="rounded-md border border-teal-700 bg-teal-700 px-3 py-2 text-sm font-medium text-white" type="button" onClick={() => copy(panel.label, panel.value)}>
                {copied === panel.label ? "Copied" : "Copy"}
              </button>
            </div>
            <pre className="max-h-[420px] overflow-auto whitespace-pre-wrap break-words rounded-md border border-stone-300 bg-stone-950 p-3 font-mono text-xs leading-5 text-stone-50">{panel.value}</pre>
          </section>
        </div>
      </div>
    </div>
  );
}

function SetupTab({ active, onClick, children }: { active: boolean; onClick: () => void; children: React.ReactNode }) {
  return (
    <button className={active ? "rounded-md border border-teal-700 bg-teal-700 px-3 py-2 text-sm font-medium text-white" : "rounded-md border border-stone-300 bg-white px-3 py-2 text-sm font-medium text-neutral-700"} type="button" role="tab" aria-selected={active} onClick={onClick}>
      {children}
    </button>
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

function setupSnippets(origin: string, mcpURL: string, tenant: string) {
  const mcp = `Flink MCP endpoint
${mcpURL}

Authentication
HTTP Basic Auth
username: ${tenant}
password: <your Flink tenant password>

For MCP clients that accept remote HTTP servers, configure:

{
  "mcpServers": {
    "flink": {
      "type": "http",
      "url": "${mcpURL}",
      "headers": {
        "Authorization": "Basic <base64 of ${tenant}:your-password>"
      }
    }
  }
}

Generate the Authorization header locally:

printf '%s' '${tenant}:<your-password>' | base64

Available tools include:
- flink_list_sites
- flink_get_site
- flink_publish_site
- flink_read_file
- flink_write_file
- flink_delete_file
- flink_set_site_auth
- flink_get_site_data
- flink_set_site_data
- flink_delete_site_data
- flink_list_publishes
- flink_rollback_site`;

  const skill = `# Flink

Use this skill when the user asks to publish, inspect, update, configure, or rollback Flink sites on this server.

## Server

- Flink server: ${origin}
- MCP endpoint: ${mcpURL}
- Tenant: ${tenant}
- Auth: HTTP Basic Auth with tenant username and password.

## Rules

- Ask for the tenant password if it is not already available in a secure local configuration.
- Never put tenant passwords, Basic Auth headers, API keys, or other secrets into hosted browser files.
- Prefer the Flink MCP tools for site operations.
- If you need a CLI fallback, use an existing flink on PATH or install it once into $HOME/.local/bin/flink; do not download CLI archives into per-site or per-deployment directories.
- Keep sites owner-only unless the user explicitly asks to share them.
- Use flink_publish_site for new publishes, then verify the returned URL.
- Use flink_get_site and flink_read_file before editing an existing site.
- Use flink_set_site_auth only when the user asks to change access.

## Common Flows

Publish a site:
1. Prepare static files.
2. Call \`flink_publish_site\` with the site slug and file list.
3. Open or fetch the returned URL to verify the page loads.

Update a site:
1. Call \`flink_get_site\`.
2. Read the files you need.
3. Write or publish updated files.
4. Verify the live URL.

Configure access:
- owner: only this tenant can view.
- none: anonymous viewers can view and use allowed browser APIs.
- tenants: approved tenants can view, optionally restricted to a tenant allow-list.`;

  const plugin = `# Assumes FLINK_PASSWORD is already exported in this shell.
curl -fsSL ${shellQuote(`${origin}/_flink/codex-plugin.sh`)} | FLINK_TENANT=${shellQuote(tenant)} sh`;

  return { mcp, skill, plugin };
}

function shellQuote(value: string) {
  return `'${value.replace(/'/g, `'\\''`)}'`;
}

function formatDate(value: string) {
  if (!value) {
    return "-";
  }
  return new Date(value).toLocaleString();
}

function formatAuth(auth?: SiteAuthPolicy) {
  if (!auth || auth.mode === "owner") {
    return "Owner";
  }
  if (auth.mode === "tenants") {
    const tenants = auth.tenants ?? [];
    return tenants.length > 0 ? `Tenants: ${tenants.join(", ")}` : "Tenants: any";
  }
  return "Public";
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

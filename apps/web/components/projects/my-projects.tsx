"use client";

import { useEffect, useState } from "react";

type Project = { id: string; title: string; summary: string; visibility: string; lifecycle: string; owner?: string | null; repository?: string | null; access_state?: string | null; sync_health?: string | null };
type Envelope<T> = { data?: T; error?: { message?: string } };

export function MyProjects() {
  const [items,setItems]=useState<Project[]>([]);const [loading,setLoading]=useState(true);const [problem,setProblem]=useState("");
  useEffect(()=>{void fetch("/api/v1/my/projects",{cache:"no-store"}).then(async r=>{const v=await r.json() as Envelope<{items:Project[]}>;if(!r.ok)throw new Error(v.error?.message??"Projects could not be loaded.");setItems(v.data?.items??[])}).catch(e=>setProblem(e instanceof Error?e.message:"Projects could not be loaded.")).finally(()=>setLoading(false))},[]);
  return <section className="content account-content" aria-labelledby="my-projects-title"><p className="eyebrow">Your work</p><h1 id="my-projects-title">Projects</h1><p>Projects start by importing a GitHub repository you administer.</p><p><a className="primary-link" href="/projects/import">Import a GitHub repository</a></p>{problem&&<p className="form-error" role="alert">{problem}</p>}{loading?<p role="status">Loading your projects…</p>:items.length?<div className="project-grid">{items.map(item=><article className="project-card" key={item.id}><div><p className="eyebrow">{item.visibility} · {item.lifecycle}</p><h2>{item.title}</h2><p>{item.summary}</p>{item.repository?<p>GitHub: {item.owner}/{item.repository} · {item.sync_health??item.access_state}</p>:<p>Legacy project without a repository connection</p>}</div><a className="secondary-button" href={`/projects/${item.id}`}>Open project</a></article>)}</div>:<div className="empty-state"><h2>No projects yet</h2><p>Connect your GitHub App and import a repository to create your first PeerGit project.</p></div>}</section>;
}

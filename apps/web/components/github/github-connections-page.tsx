"use client";

import { useEffect, useState } from "react";
import { GitHubImports } from "./github-imports";
import { AccountNavigation } from "@/components/account/account-navigation";
import { SiteHeader } from "@/components/layout/site-header";

export function GitHubConnectionsPage() {
  const [csrf,setCSRF]=useState("");const [problem,setProblem]=useState("");
  useEffect(()=>{void fetch("/api/v1/session",{cache:"no-store"}).then(async r=>{const v=await r.json() as {data?:{authenticated:boolean;csrf_token?:string}};if(!v.data?.authenticated)throw new Error("Sign in to manage GitHub connections.");setCSRF(v.data.csrf_token??"")}).catch(e=>setProblem(e instanceof Error?e.message:"Account session could not be loaded."))},[]);
  return <><SiteHeader /><div className="account-layout"><AccountNavigation /><main>{problem?<section className="content" role="alert"><p className="form-error">{problem}</p><a href="/login">Log in with GitHub</a></section>:csrf?<GitHubImports csrf={csrf}/>:<section className="content" role="status">Loading account…</section>}</main></div></>;
}

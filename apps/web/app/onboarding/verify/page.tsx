"use client";

import { FormEvent, useEffect, useState } from "react";

type Envelope<T> = { data?: T; error?: { message: string } };

export default function VerifyCampusPage() {
  const [csrf,setCSRF]=useState("");const [token,setToken]=useState("");const [otp,setOTP]=useState("");const [challengeID,setChallengeID]=useState("");const [problem,setProblem]=useState("");const [complete,setComplete]=useState(false);
  useEffect(()=>{const fragment=new URLSearchParams(location.hash.slice(1));const value=fragment.get("token")??"";setToken(value);setChallengeID(new URLSearchParams(location.search).get("challenge_id")??"");history.replaceState(null,"",location.pathname+location.search);void fetch("/api/v1/session",{cache:"no-store"}).then(r=>r.json()).then((v:Envelope<{authenticated:boolean;csrf_token?:string}>)=>{if(!v.data?.authenticated){location.assign("/login");return}setCSRF(v.data.csrf_token??"")}).catch(()=>setProblem("Could not load your session. Please sign in again."))},[]);
  async function confirm(event:FormEvent<HTMLFormElement>){event.preventDefault();setProblem("");try{const method=token?"link":"otp";const response=await fetch("/api/v1/me/campus-verification/confirm",{method:"POST",headers:{"Content-Type":"application/json","X-CSRF-Token":csrf},body:JSON.stringify(method==="link"?{method,token}:{method,challenge_id:challengeID,otp})});const result=await response.json() as Envelope<{verified:boolean}>;if(!response.ok||result.error)throw new Error(result.error?.message??"Verification could not be completed.");setComplete(true);setToken("");}catch(e){setProblem(e instanceof Error?e.message:"Verification could not be completed.")}}
  return <main className="account-shell">
    <a className="skip-link" href="#verify-content">Skip to content</a>
    <header className="topbar"><a className="brand" href="/">PeerGit</a><nav aria-label="Account"><a href="/onboarding">Back to onboarding</a></nav></header>
    <section id="verify-content" className="account-card" aria-labelledby="verify-title">
      <p className="eyebrow">Campus verification</p><h1 id="verify-title">Check your campus email</h1>
      <p>Both verification methods expire ten minutes after they are issued. Opening this page does not complete verification; submit the form to confirm.</p>
      {complete?<p className="form-success" role="status">Campus email verified. <a href="/onboarding">Continue to onboarding</a>.</p>:<form className="account-form" onSubmit={confirm}>{token?<p>A campus email link is ready to confirm for your signed-in account.</p>:<><label htmlFor="verification-code">Six-digit code</label><input id="verification-code" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9]{6}" maxLength={6} required value={otp} onChange={e=>setOTP(e.target.value)} /></>}<button className="primary-button" type="submit" disabled={!csrf||(!token&&!challengeID)}>Confirm campus email</button></form>}
      {problem&&<p className="form-error" role="alert">{problem}</p>}
    </section>
  </main>;
}

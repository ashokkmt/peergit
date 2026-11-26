import Link from "next/link";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

export default function PrivacyPage() {
  return (
    <>
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#policy"
      >
        Skip to privacy notice
      </a>
      <SiteHeader />
      <main className="flex-1 w-[min(860px,calc(100%-32px))] mx-auto pt-8 pb-16 leading-[1.7]">
        <Link href="/">← Back to PeerGit</Link>
        <article
          id="policy"
          className="mt-5 bg-surface border border-hairline rounded-2xl p-[clamp(24px,5vw,48px)]"
        >
          <p className="mt-0 mb-1.5 text-link text-xs font-semibold tracking-[0.02em]">
            Draft for institutional review
          </p>
          <h1 className="font-display text-[clamp(1.75rem,4vw,2.75rem)] leading-[1.15] font-bold mt-0 mb-4">
            PeerGit Privacy Notice
          </h1>
          <p>
            This draft describes the intended one-campus data flow. The institution must approve
            the final notice, retention schedule, processors, residency, and rights-request
            procedure before production onboarding.
          </p>
          <h2 className="mt-7 mb-2.5 text-[18px] font-semibold">Information used</h2>
          <p>
            GitHub provides an immutable account identifier, verified primary contact email, and
            profile name for sign-in. Campus email is collected separately and verified through a
            one-time challenge. PeerGit stores campus verification, profile fields and skills you
            submit, organization and collaboration records, consent choices, and security/audit
            records. Uploaded profile images are decoded and re-encoded before storage.
          </p>
          <h2 className="mt-7 mb-2.5 text-[18px] font-semibold">
            Visibility and repository evidence
          </h2>
          <p>
            Campus profiles and project information are access-controlled. Optional profile
            discovery and any portfolio or recruiter visibility require separate consent. Private
            repository source is not sent to search, analytics, logs, email, or recommendations.
            Repository evidence is fetched only under the GitHub permissions granted to PeerGit.
          </p>
          <h2 className="mt-7 mb-2.5 text-[18px] font-semibold">Storage and retention</h2>
          <p>
            Local development uses a local database and S3-compatible object service with synthetic
            data. Production storage, location, backup, deletion, and retention rules are separate
            release gates. The production policy must describe what is retained and how requests
            are handled.
          </p>
          <h2 className="mt-7 mb-2.5 text-[18px] font-semibold">Your choices</h2>
          <p>
            You can revoke optional discovery consent in your account. Access changes take effect
            for new requests. Contact and deletion/export workflows must be published by the campus
            before release.
          </p>
          <p>
            <strong>Status:</strong> draft-1. This notice is not a statement of approved legal
            compliance.
          </p>
        </article>
      </main>
      <SiteFooter />
    </>
  );
}

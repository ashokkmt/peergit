import Link from "next/link";
import { SiteHeader } from "@/components/layout/site-header";
import { SiteFooter } from "@/components/layout/site-footer";

export default function TermsPage() {
  return (
    <>
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#policy"
      >
        Skip to terms
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
            PeerGit Terms of Use
          </h1>
          <p>
            This draft explains the expected use of the development release. It is not an approved
            campus agreement and must be reviewed and replaced by the institution before production
            onboarding.
          </p>
          <h2 className="mt-7 mb-2.5 text-[18px] font-semibold">Use of the service</h2>
          <p>
            Use PeerGit to discover and coordinate campus projects, share work you are authorized to
            share, and treat other members respectfully. Do not upload material you do not have
            permission to provide or use PeerGit to harass, impersonate, spam, or bypass access
            controls.
          </p>
          <h2 className="mt-7 mb-2.5 text-[18px] font-semibold">
            Project and repository content
          </h2>
          <p>
            Git hosting remains with GitHub for the first release. A repository connection does not
            transfer ownership of a project or grant access to private code beyond the permissions
            the member and repository owner approved. Immutable repository snapshots are created
            only for PBL checkpoints and follow the retention notice shown for that submission.
          </p>
          <h2 className="mt-7 mb-2.5 text-[18px] font-semibold">Safety and enforcement</h2>
          <p>
            PeerGit may restrict accounts or content to respond to a credible safety, security, or
            policy concern. The campus must publish its final reporting, moderation, appeal,
            retention, and contact procedures before launch.
          </p>
          <h2 className="mt-7 mb-2.5 text-[18px] font-semibold">Account and data choices</h2>
          <p>
            You are responsible for keeping your account secure and for choosing whether optional
            profile or portfolio discovery is enabled. You can change optional consent
            in your account settings.
          </p>
          <p>
            <strong>Status:</strong> draft-1. Production use is blocked until campus ownership,
            support contacts, retention rules, and final legal language are approved.
          </p>
        </article>
      </main>
      <SiteFooter />
    </>
  );
}

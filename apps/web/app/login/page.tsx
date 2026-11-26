import { StackedBrandLogo } from "@/components/layout/brand-logo";
import { AppearanceControl } from "@/components/layout/appearance-control";

export default function LoginPage() {
  return (
    <div className="min-h-screen grid grid-cols-1 lg:grid-cols-[7fr_5fr]">
      <a
        className="fixed z-100 left-4 -top-20 px-4 py-2.5 bg-primary text-on-primary rounded-lg font-semibold no-underline focus:top-3"
        href="#login-content"
      >
        Skip to content
      </a>

      <main id="login-content" className="flex flex-col justify-center px-4 py-8 md:p-12">
        <div className="w-[min(448px,100%)] mx-auto flex flex-col gap-5">
          <div className="flex items-center justify-between gap-3">
            <a className="text-[13px] font-medium leading-[18px] text-link no-underline hover:underline" href="/">
              ← Back to PeerGit
            </a>
            <div className="flex items-center gap-3">
              <nav aria-label="Account">
                <a className="text-[13px] font-medium leading-[18px] text-link" href="/signup">
                  Join PeerGit
                </a>
              </nav>
              <AppearanceControl />
            </div>
          </div>

          <section
            className="min-w-0 max-w-full bg-surface border border-hairline rounded-[20px] p-8 flex flex-col gap-5"
            aria-labelledby="login-title"
          >
            <div className="flex flex-col gap-1.5">
              <p className="m-0 text-link text-xs font-semibold tracking-[0.02em]">Welcome back</p>
              <h1
                id="login-title"
                className="font-display text-[clamp(1.5rem,2.5vw,2rem)] font-semibold leading-[1.19] tracking-[-0.015em] m-0"
              >
                Log in to PeerGit
              </h1>
              <p className="text-sm leading-[21px] text-muted m-0">
                Use the GitHub account linked to your PeerGit profile.
              </p>
            </div>

            <a
              className="inline-flex w-full min-h-12 items-center justify-center gap-2 rounded-lg px-[18px] py-2 text-sm font-medium no-underline whitespace-nowrap border border-primary bg-primary text-on-primary hover:opacity-90 transition-opacity duration-100"
              href="/api/v1/auth/github"
            >
              Continue with GitHub <span aria-hidden="true">→</span>
            </a>

            <div className="flex flex-col gap-1.5 p-4 border border-hairline rounded-xl bg-raised text-muted text-[13px] leading-[19px]">
              <p className="text-[13px] font-medium leading-[18px] m-0 text-on-surface">
                How identity and campus access stay separate
              </p>
              <p className="text-[13px] leading-[19px] text-muted m-0">
                GitHub signs you in and attributes your commits. Campus discovery and repository
                imports depend on your verified campus membership.
              </p>
            </div>

            <p className="text-[13px] leading-[19px] text-muted m-0">
              New to PeerGit? <a href="/signup">Create an account</a>. By continuing, you agree to
              the <a href="/terms">Terms of Service</a> and <a href="/privacy">Privacy Policy</a>.
            </p>
          </section>

          <div className="min-w-0 max-w-full bg-surface border border-hairline rounded-2xl px-[18px] py-3.5 flex flex-wrap items-center justify-between gap-3">
            <span className="text-[13px] leading-[19px] text-muted">
              Missing verified GitHub email or identity conflict?
            </span>
            <a className="text-[13px] font-medium leading-[18px]" href="/privacy">
              Review account notice
            </a>
          </div>
        </div>
      </main>

      <aside
        className="bg-surface border-t lg:border-t-0 lg:border-l border-hairline flex flex-col items-center justify-center py-12 px-6 gap-6 text-center"
        aria-label="PeerGit overview"
      >
        <StackedBrandLogo />
        <div className="max-w-sm flex flex-col gap-2">
          <h2 className="text-[22px] font-semibold leading-7 tracking-[-0.01em] m-0">
            Your campus projects, connected.
          </h2>
          <p className="text-sm leading-[21px] text-muted m-0">
            Keep your GitHub remote, collaborate with verified campus teammates, and capture
            permanent submission receipts.
          </p>
        </div>
      </aside>
    </div>
  );
}

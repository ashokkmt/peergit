export default function SignupPage() {
  return <main className="account-shell">
    <a className="skip-link" href="#signup-content">Skip to content</a>
    <header className="topbar"><a className="brand" href="/">PeerGit</a><nav aria-label="Account"><a href="/login">Log in</a></nav></header>
    <section id="signup-content" className="account-card" aria-labelledby="signup-title">
      <p className="eyebrow">Create your account</p><h1 id="signup-title">Start with GitHub</h1>
      <p>GitHub confirms your account and contact email. You can explore public pages before verifying a campus email.</p>
      <a className="primary-link" href="/api/v1/auth/github">Continue with GitHub <span aria-hidden="true">→</span></a>
      <p className="muted">Campus features unlock after you verify an approved campus email. GitHub repository access is requested separately, when you connect a project.</p>
      <p>Already have an account? <a href="/login">Log in</a>.</p>
    </section>
  </main>;
}

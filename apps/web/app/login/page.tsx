export default function LoginPage() {
  return <main className="account-shell">
    <a className="skip-link" href="#login-content">Skip to content</a>
    <header className="topbar"><a className="brand" href="/">PeerGit</a><nav aria-label="Account"><a href="/signup">Create account</a></nav></header>
    <section id="login-content" className="account-card" aria-labelledby="login-title">
      <p className="eyebrow">Welcome back</p><h1 id="login-title">Log in to PeerGit</h1>
      <p>Use the GitHub account linked to your PeerGit profile.</p>
      <a className="primary-link" href="/api/v1/auth/github">Continue with GitHub <span aria-hidden="true">→</span></a>
      <p>New to PeerGit? <a href="/signup">Create an account</a>.</p>
    </section>
  </main>;
}

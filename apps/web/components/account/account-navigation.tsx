export function AccountNavigation() {
  return <nav className="account-navigation" aria-label="Account navigation">
    <details className="account-mobile-nav"><summary>Account menu</summary><a href="/account/profile">Profile</a><a href="/projects">Projects</a><a href="/account/github">GitHub connections</a><a href="/account/settings">Settings</a></details>
    <div className="account-sidebar"><a href="/account/profile">Profile</a><a href="/projects">Projects</a><a href="/account/github">GitHub connections</a><a href="/account/settings">Settings</a></div>
  </nav>;
}

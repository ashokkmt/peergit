export function BrandLogo() {
  return (
    <a className="brand" href="/" aria-label="PeerGit home">
      <picture>
        <source media="(prefers-color-scheme: dark)" srcSet="/brand/wordmark-600x300-dark.png" />
        <img src="/brand/wordmark-600x300-light.png" alt="" />
      </picture>
    </a>
  );
}

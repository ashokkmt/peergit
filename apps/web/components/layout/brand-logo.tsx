export function BrandLogo({ mark = false }: { mark?: boolean }) {
  const asset = mark ? "mark-512" : "wordmark-600x300";
  return (
    <a className={mark ? "brand brand-monogram" : "brand"} href="/" aria-label="PeerGit home">
      <img className="wordmark-light" src={`/brand/${asset}-light.png`} alt="" />
      <img className="wordmark-dark" src={`/brand/${asset}-dark.png`} alt="" />
    </a>
  );
}

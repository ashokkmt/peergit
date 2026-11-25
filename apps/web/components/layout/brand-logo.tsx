function BrandMarkSvg({ width, height }: { width: number; height: number }) {
  return (
    <svg
      width={width}
      height={height}
      viewBox="0 0 24 28"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      aria-hidden="true"
    >
      <path
        d="M2 2H13C19.0751 2 23 5.92487 23 11.5C23 17.0751 19.0751 21 13 21H9V24.5L2 27.5V2Z"
        style={{ fill: "var(--brand-mark-bg)" }}
      />
      <path
        d="M9.5 15.5V12.5L15 7.5"
        style={{ stroke: "var(--brand-mark-cutout)" }}
        strokeWidth="2.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="9.5" cy="16" r="2.8" style={{ fill: "var(--brand-mark-cutout)" }} />
      <circle cx="15.5" cy="7.5" r="3.2" fill="#0153FC" />
    </svg>
  );
}

function BrandWordmark({ stacked = false }: { stacked?: boolean }) {
  return (
    <>
      <span className={stacked ? "stacked-brand-name" : "brand-wordmark"} aria-hidden="true">
        peerg
        <span className="brand-i-wrap">
          ı<span className="brand-i-dot" />
        </span>
        t
      </span>
      <span className="sr-only">PeerGit</span>
    </>
  );
}

export function BrandLogo({ mark = false }: { mark?: boolean }) {
  return (
    <a className={mark ? "brand brand-monogram" : "brand"} href="/" aria-label="PeerGit home">
      <BrandMarkSvg width={24} height={28} />
      {!mark && <BrandWordmark />}
    </a>
  );
}

export function StackedBrandLogo() {
  return (
    <div className="stacked-brand" aria-label="PeerGit">
      <BrandMarkSvg width={88} height={104} />
      <BrandWordmark stacked />
    </div>
  );
}

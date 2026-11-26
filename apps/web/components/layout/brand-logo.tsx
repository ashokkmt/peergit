function BrandMarkSvg({ width, height }: { width: number; height: number }) {
  return (
    <svg
      width={width}
      height={height}
      viewBox="0 0 24 28"
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
      className="block shrink-0"
      aria-hidden="true"
    >
      <path
        d="M2 2H13C19.0751 2 23 5.92487 23 11.5C23 17.0751 19.0751 21 13 21H9V24.5L2 27.5V2Z"
        className="[fill:var(--brand-mark-bg)]"
      />
      <path
        d="M9.5 15.5V12.5L15 7.5"
        className="[stroke:var(--brand-mark-cutout)]"
        strokeWidth="2.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <circle cx="9.5" cy="16" r="2.8" className="[fill:var(--brand-mark-cutout)]" />
      <circle cx="15.5" cy="7.5" r="3.2" fill="#0153FC" />
    </svg>
  );
}

function BrandWordmark({ stacked = false }: { stacked?: boolean }) {
  return (
    <>
      <span
        className={
          stacked
            ? "font-display text-[32px] leading-[38px] font-bold tracking-[-0.02em] text-on-surface"
            : "font-display text-[20px] leading-[26px] min-[420px]:text-[22px] min-[420px]:leading-[28px] font-bold tracking-[-0.02em] text-on-surface"
        }
        aria-hidden="true"
      >
        peerg
        <span className="relative inline-block">
          ı
          <span className="absolute left-1/2 -translate-x-1/2 top-[0.14em] w-[0.23em] h-[0.23em] rounded-full bg-brand" />
        </span>
        t
      </span>
      <span className="sr-only">PeerGit</span>
    </>
  );
}

export function BrandLogo({ mark = false }: { mark?: boolean }) {
  return (
    <a
      className="inline-flex flex-none items-center gap-2 no-underline rounded-sm"
      href="/"
      aria-label="PeerGit home"
    >
      <BrandMarkSvg width={24} height={28} />
      {!mark && <BrandWordmark />}
    </a>
  );
}

export function StackedBrandLogo() {
  return (
    <div className="flex flex-col items-center gap-3" aria-label="PeerGit">
      <BrandMarkSvg width={88} height={104} />
      <BrandWordmark stacked />
    </div>
  );
}

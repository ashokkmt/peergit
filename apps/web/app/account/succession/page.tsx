import { SuccessionManager } from "@/components/portfolio/portfolio-pages";

export default async function SuccessionPage({
  searchParams,
}: {
  searchParams: Promise<{ resource?: string }>;
}) {
  const { resource } = await searchParams;
  return <SuccessionManager initialResource={resource} />;
}

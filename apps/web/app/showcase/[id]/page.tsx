import { PublicProjectPage } from "./showcase-detail";
export default async function Page({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <PublicProjectPage projectID={id} />;
}

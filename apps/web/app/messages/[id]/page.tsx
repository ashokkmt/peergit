import { SiteFooter } from "@/components/layout/site-footer";
import { SiteHeader } from "@/components/layout/site-header";
import { ConversationRoom } from "@/components/collaboration/conversation-room";

export default async function ConversationPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  return (
    <>
      <a
        className="fixed left-4 -top-20 z-100 rounded-lg bg-primary px-4 py-2.5 font-semibold text-on-primary focus:top-3"
        href="#main-content"
      >
        Skip to content
      </a>
      <SiteHeader workspace />
      <main
        id="main-content"
        className="mx-auto flex w-[min(1240px,calc(100%-32px))] flex-1 flex-col gap-6 py-8 md:w-[min(1240px,calc(100%-64px))] md:py-12"
      >
        <ConversationRoom conversationID={id} />
      </main>
      <SiteFooter />
    </>
  );
}

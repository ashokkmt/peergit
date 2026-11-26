import { SiteFooter } from "@/components/layout/site-footer";
import { SiteHeader } from "@/components/layout/site-header";
import { MessagesInbox } from "@/components/collaboration/messages-inbox";

export default function MessagesPage() {
  return (
    <>
      <SiteHeader workspace />
      <main className="mx-auto flex w-[min(1120px,calc(100%-32px))] flex-1 flex-col gap-6 py-8 md:w-[min(1120px,calc(100%-64px))] md:py-12">
        <MessagesInbox />
      </main>
      <SiteFooter />
    </>
  );
}

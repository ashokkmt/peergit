import { MentorshipConversation } from "@/components/academic/mentorship-conversation";

export default async function MentorshipConversationPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  return <MentorshipConversation conversationID={id} />;
}

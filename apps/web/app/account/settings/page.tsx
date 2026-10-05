import { AccountNavigation } from "@/components/account/account-navigation";
import { AccountPanel } from "@/components/account/account-panel";
import { SiteHeader } from "@/components/layout/site-header";

export default function SettingsPage() {
  return <><SiteHeader /><div className="account-layout"><AccountNavigation /><main><AccountPanel section="settings" /></main></div></>;
}

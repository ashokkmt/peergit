import { AppearanceControl } from "@/components/layout/appearance-control";
import { AccountPanel } from "@/components/account/account-panel";

export default function SettingsPage() {
  return <><AccountPanel section="settings" /><section className="panel"><h2>Appearance</h2><p>Choose the theme for this device.</p><AppearanceControl /></section></>;
}

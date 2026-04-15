import { NavLink, Outlet, useNavigate } from "react-router-dom";
import {
  LayoutDashboard,
  FolderTree,
  Server,
  Package,
  Rocket,
  KeyRound,
  Download,
  Settings,
  LogOut,
  BookOpen,
  ExternalLink,
  type LucideIcon,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { logout } from "@/api/client";
import { cn } from "@/lib/utils";
import { KairosLogo } from "@/components/KairosLogo";
import { Toaster } from "@/components/ui/toaster";

// NavItem is either a client-side route (the common case) or an
// `external: true` link that renders as a plain <a target="_blank">
// — used for server-rendered pages like /api/docs that aren't part of
// the React router.
type NavItem = {
  to: string;
  icon: LucideIcon;
  label: string;
  external?: boolean;
};
type NavSection = { label?: string; items: NavItem[] };

const navSections: NavSection[] = [
  {
    items: [
      { to: "/", icon: LayoutDashboard, label: "Dashboard" },
    ],
  },
  {
    label: "Fleet",
    items: [
      { to: "/nodes", icon: Server, label: "Nodes" },
      { to: "/groups", icon: FolderTree, label: "Groups" },
    ],
  },
  {
    label: "Build",
    items: [
      { to: "/artifacts", icon: Package, label: "Artifacts" },
    ],
  },
  {
    label: "Deploy",
    items: [
      { to: "/deployments", icon: Rocket, label: "Deployments" },
      { to: "/import", icon: Download, label: "Import" },
    ],
  },
  {
    label: "Admin",
    items: [
      { to: "/certificates", icon: KeyRound, label: "Certificates" },
      { to: "/settings", icon: Settings, label: "Settings" },
    ],
  },
  {
    label: "Developer",
    items: [
      { to: "/api/docs", icon: BookOpen, label: "API docs", external: true },
    ],
  },
];

export function Layout() {
  const navigate = useNavigate();

  function handleLogout() {
    logout();
    navigate("/login");
  }

  return (
    <div className="flex h-screen">
      {/* Sidebar */}
      <aside className="flex w-60 flex-col bg-sidebar-bg text-sidebar-fg">
        <div className="flex flex-col items-center gap-3 px-6 pt-6 pb-4">
          <KairosLogo className="h-14 w-auto" />
          <div className="text-center">
            <span className="font-bold text-base text-white block leading-tight">AuroraBoot</span>
          </div>
        </div>
        <div className="mx-4 border-t border-white/10" />
        <nav className="flex-1 overflow-y-auto p-3">
          {navSections.map((section, si) => (
            <div key={si}>
              {section.label && (
                <span className="text-[10px] uppercase tracking-wider text-sidebar-fg/40 px-3 pt-4 pb-1 block">
                  {section.label}
                </span>
              )}
              <div className="space-y-1">
                {section.items.map((item) =>
                  item.external ? (
                    <a
                      key={item.to}
                      href={item.to}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors text-sidebar-fg opacity-80 hover:opacity-100 hover:bg-sidebar-muted"
                    >
                      <item.icon className="h-4 w-4" />
                      <span className="flex-1">{item.label}</span>
                      <ExternalLink className="h-3 w-3 opacity-60" />
                    </a>
                  ) : (
                    <NavLink
                      key={item.to}
                      to={item.to}
                      end={item.to === "/"}
                      className={({ isActive }) =>
                        cn(
                          "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
                          isActive
                            ? "bg-sidebar-accent text-white"
                            : "text-sidebar-fg opacity-80 hover:opacity-100 hover:bg-sidebar-muted"
                        )
                      }
                    >
                      <item.icon className="h-4 w-4" />
                      {item.label}
                    </NavLink>
                  ),
                )}
              </div>
            </div>
          ))}
        </nav>
        <div className="mx-4 border-t border-white/10" />
        <div className="p-3 pb-5">
          <Button
            variant="ghost"
            className="w-full justify-start gap-3 text-sidebar-fg opacity-70 hover:opacity-100 hover:bg-sidebar-muted hover:text-sidebar-fg"
            onClick={handleLogout}
          >
            <LogOut className="h-4 w-4" />
            Logout
          </Button>
        </div>
      </aside>

      {/* Main content */}
      <main className="flex-1 overflow-auto">
        <div className="p-8">
          <Outlet />
        </div>
        <Toaster />
      </main>
    </div>
  );
}

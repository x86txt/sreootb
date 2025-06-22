'use client';

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { cn } from "@/lib/utils";
import { 
  Activity, 
  BarChart3, 
  Globe, 
  Settings, 
  HelpCircle, 
  Server, 
  ChevronDown, 
  ChevronRight,
  Monitor,
  Clock,
  Search,
  Network,
  Shield,
  Pi,
} from "lucide-react";
import { useAuth } from "@/context/AuthContext";
import { LoginDialog } from "@/components/LoginDialog";

const navigation = [
  {
    name: "Dashboard",
    href: "/",
    icon: BarChart3,
  },
  {
    name: "Resources",
    href: "/resources",
    icon: Globe,
    children: [
      {
        name: "HTTP(S)",
        href: "/resources/http",
        icon: Monitor,
        description: "Web services and APIs"
      },
      {
        name: "Ping Latency",
        href: "/resources/ping",
        icon: Clock,
        description: "Network connectivity tests"
      },
      {
        name: "DNS",
        href: "/resources/dns",
        icon: Search,
        description: "Domain name resolution"
      },
      {
        name: "TCP/UDP",
        href: "/resources/ports",
        icon: Network,
        description: "Port connectivity tests"
      },
      {
        name: "SSL/TLS",
        href: "/resources/ssl",
        icon: Shield,
        description: "Certificate monitoring"
      }
    ]
  },
  {
    name: "Agents",
    href: "/agents",
    icon: Server,
  },
];

const bottomNavigation = [
  {
    name: "Settings",
    href: "/settings",
    icon: Settings,
  },
  {
    name: "Help",
    href: "/help",
    icon: HelpCircle,
  },
];

interface SidebarProps {}

export function Sidebar({}: SidebarProps) {
  const pathname = usePathname();
  const [expandedItems, setExpandedItems] = useState<Set<string>>(new Set());
  const { isAuthenticated, logout } = useAuth();
  const [showLoginDialog, setShowLoginDialog] = useState(false);

  const toggleExpanded = (itemName: string) => {
    const newExpanded = new Set(expandedItems);
    if (newExpanded.has(itemName)) {
      newExpanded.delete(itemName);
    } else {
      newExpanded.add(itemName);
    }
    setExpandedItems(newExpanded);
  };

  const isItemActive = (href: string) => {
    if (href === "/") {
      return pathname === "/";
    }
    return pathname.startsWith(href);
  };

  return (
    <div className="flex h-full w-64 flex-col bg-card border-r">
      {/* Logo */}
      <div className="flex h-16 items-center border-b px-6">
        <Activity className="h-6 w-6 text-primary" />
        <span className="ml-2 text-lg font-semibold">SRE - OoB</span>
      </div>

      {/* Navigation */}
      <nav className="flex-1 space-y-1 px-4 py-2">
        <div className="space-y-1">
          {navigation.map((item) => {
            const isActive = isItemActive(item.href);
            const isExpanded = expandedItems.has(item.name);
            const hasChildren = item.children && item.children.length > 0;

            return (
              <div key={item.name}>
                {/* Main navigation item */}
                {hasChildren ? (
                  // Items with children - clicking toggles dropdown
                  <button
                    onClick={() => toggleExpanded(item.name)}
                    className={cn(
                      "group w-full flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors text-left",
                      isActive
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                    )}
                  >
                    <item.icon className="mr-3 h-4 w-4" />
                    {item.name}
                    <div className="ml-auto">
                      {isExpanded ? (
                        <ChevronDown className="h-4 w-4" />
                      ) : (
                        <ChevronRight className="h-4 w-4" />
                      )}
                    </div>
                  </button>
                ) : (
                  // Items without children - clicking navigates
                  <Link
                    href={item.href}
                    className={cn(
                      "group flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors",
                      isActive
                        ? "bg-primary text-primary-foreground"
                        : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                    )}
                  >
                    <item.icon className="mr-3 h-4 w-4" />
                    {item.name}
                  </Link>
                )}

                {/* Submenu items */}
                {hasChildren && isExpanded && (
                  <div className="ml-6 mt-1 space-y-1">
                    {item.children.map((child) => {
                      const isChildActive = isItemActive(child.href);
                      return (
                        <Link
                          key={child.name}
                          href={child.href}
                          className={cn(
                            "group flex items-center rounded-md px-3 py-2 text-sm transition-colors",
                            isChildActive
                              ? "bg-primary/10 text-primary border-l-2 border-primary"
                              : "text-muted-foreground hover:bg-accent/50 hover:text-accent-foreground"
                          )}
                          title={child.description}
                        >
                          <child.icon className="mr-3 h-3 w-3" />
                          <div>
                            <div className="font-medium">{child.name}</div>
                            <div className="text-xs text-muted-foreground">
                              {child.description}
                            </div>
                          </div>
                        </Link>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      </nav>

      {/* Bottom Navigation */}
      <div className="border-t p-4">
        <div className="space-y-1">
          {bottomNavigation.map((item) => {
            const isActive = isItemActive(item.href);
            return (
              <Link
                key={item.name}
                href={item.href}
                className={cn(
                  "group flex items-center rounded-md px-3 py-2 text-sm font-medium transition-colors",
                  isActive
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:bg-accent hover:text-accent-foreground"
                )}
              >
                <item.icon className="mr-3 h-4 w-4" />
                {item.name}
              </Link>
            );
          })}
        </div>
      </div>

      {/* User Section */}
      <div className="border-t p-4">
        <div 
          className="flex items-center cursor-pointer group"
          onClick={() => !isAuthenticated && setShowLoginDialog(true)}
        >
          <div className="h-8 w-8 rounded-full bg-primary text-primary-foreground flex items-center justify-center text-sm font-medium group-hover:bg-primary/90 transition-colors">
            {isAuthenticated ? 'A' : <Pi className="h-5 w-5" />}
          </div>
          <div className="ml-3">
            <p className="text-sm font-medium">
              {isAuthenticated ? 'Admin Mode' : 'Anonymous Mode'}
            </p>
            {isAuthenticated ? (
              <button onClick={logout} className="text-xs text-muted-foreground hover:text-foreground">
                Logout
              </button>
            ) : (
              <p className="text-xs text-muted-foreground">Click to login</p>
            )}
          </div>
        </div>
      </div>

      <LoginDialog open={showLoginDialog} onOpenChange={setShowLoginDialog} />
    </div>
  );
} 
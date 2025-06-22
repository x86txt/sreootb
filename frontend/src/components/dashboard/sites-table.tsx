'use client';

import { useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { MoreHorizontal, ExternalLink, Trash2, RefreshCw, CheckCircle, XCircle, Clock } from "lucide-react";
import { type SiteStatus } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useAuth } from "@/context/AuthContext";

interface SitesTableProps {
  sites: SiteStatus[];
  onDeleteSite: (id: number) => void;
  onCheckSite: (id: number) => void;
  isChecking: boolean;
}

export function SitesTable({ sites, onDeleteSite, onCheckSite, isChecking }: SitesTableProps) {
  const [sortBy, setSortBy] = useState<'name' | 'status' | 'response_time' | 'url'>('name');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc');
  const { isAuthenticated } = useAuth();

  const handleSort = (column: typeof sortBy) => {
    if (sortBy === column) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(column);
      setSortOrder('asc');
    }
  };

  const sortedSites = [...sites].sort((a, b) => {
    let aValue: string | number, bValue: string | number;
    
    switch (sortBy) {
      case 'status':
        aValue = a.status || 'unknown';
        bValue = b.status || 'unknown';
        break;
      case 'response_time':
        aValue = a.response_time || 0;
        bValue = b.response_time || 0;
        break;
      case 'url':
        aValue = a.url || '';
        bValue = b.url || '';
        break;
      default:
        aValue = a.name.toLowerCase();
        bValue = b.name.toLowerCase();
    }

    if (aValue < bValue) return sortOrder === 'asc' ? -1 : 1;
    if (aValue > bValue) return sortOrder === 'asc' ? 1 : -1;
    return 0;
  });

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'up':
        return <Badge className="bg-green-100 text-green-800 hover:bg-green-100">Online</Badge>;
      case 'down':
        return <Badge variant="destructive">Down</Badge>;
      case 'unknown':
        return <Badge variant="secondary">Unknown</Badge>;
      default:
        return <Badge variant="outline">{status}</Badge>;
    }
  };

  const formatResponseTime = (time: number | null) => {
    if (time === null) return 'N/A';
    return `${Math.round(time * 1000)}ms`;
  };

  const formatLastChecked = (timestamp?: string) => {
    if (!timestamp) return 'Never';
    const date = new Date(timestamp);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    
    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}h ago`;
    const diffDays = Math.floor(diffHours / 24);
    return `${diffDays}d ago`;
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Monitored Resources</CardTitle>
        <CardDescription>
          Websites and services being monitored for uptime and performance
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className="rounded-md border">
          <div className="grid grid-cols-12 gap-4 p-4 font-medium text-sm bg-muted/50 border-b">
            <div className="col-span-3">
              <button
                onClick={() => handleSort('name')}
                className="flex items-center hover:text-foreground"
              >
                Site Name
                {sortBy === 'name' && (
                  <span className="ml-1">{sortOrder === 'asc' ? '↑' : '↓'}</span>
                )}
              </button>
            </div>
            <div className="col-span-3">
              <button
                onClick={() => handleSort('url')}
                className="flex items-center hover:text-foreground"
              >
                URL
                {sortBy === 'url' && (
                  <span className="ml-1">{sortOrder === 'asc' ? '↑' : '↓'}</span>
                )}
              </button>
            </div>
            <div className="col-span-1">
              <button
                onClick={() => handleSort('status')}
                className="flex items-center hover:text-foreground"
              >
                Status
                {sortBy === 'status' && (
                  <span className="ml-1">{sortOrder === 'asc' ? '↑' : '↓'}</span>
                )}
              </button>
            </div>
            <div className="col-span-2">
              <button
                onClick={() => handleSort('response_time')}
                className="flex items-center hover:text-foreground"
              >
                Response Time
                {sortBy === 'response_time' && (
                  <span className="ml-1">{sortOrder === 'asc' ? '↑' : '↓'}</span>
                )}
              </button>
            </div>
            <div className="col-span-2">Last Checked</div>
            <div className="col-span-1">Actions</div>
          </div>

          {sortedSites.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              No sites configured yet. Add your first site to start monitoring.
            </div>
          ) : (
            <div className="divide-y">
              {sortedSites.map((site) => (
                <div key={site.id} className="grid grid-cols-12 gap-4 p-4 items-center hover:bg-muted/50">
                  <div className="col-span-3">
                    <div className="font-medium">{site.name}</div>
                    <div className="text-sm text-muted-foreground">
                      {site.scan_interval && `Scanned every ${site.scan_interval}`}
                    </div>
                  </div>
                  <div className="col-span-3">
                    <div className="flex items-center">
                      <span className="truncate">{site.url}</span>
                      <a
                        href={site.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="ml-2 text-primary hover:text-primary/80 flex-shrink-0"
                      >
                        <ExternalLink className="h-3 w-3" />
                      </a>
                    </div>
                  </div>
                  <div className="col-span-1">
                    {getStatusBadge(site.status || 'unknown')}
                  </div>
                  <div className="col-span-2">
                    <span className={cn(
                      "font-medium",
                      site.response_time && site.response_time > 0.5 ? "text-red-600" : "text-green-600"
                    )}>
                      {formatResponseTime(site.response_time ?? null)}
                    </span>
                    {site.status_code && (
                      <div className="text-xs text-muted-foreground">
                        Status: {site.status_code}
                      </div>
                    )}
                  </div>
                  <div className="col-span-2 text-sm text-muted-foreground">
                    {formatLastChecked(site.checked_at)}
                  </div>
                  <div className="col-span-1">
                    <div className="flex items-center gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => onCheckSite(site.id)}
                        disabled={isChecking || !isAuthenticated}
                        title="Check now"
                      >
                        <RefreshCw className={cn(
                          "h-4 w-4", 
                          isChecking && "animate-spin"
                        )} />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => onDeleteSite(site.id)}
                        className="text-red-600 hover:text-red-700 hover:bg-red-50"
                        disabled={!isAuthenticated}
                        title="Delete site"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>

        {sites.length > 0 && (
          <div className="flex items-center justify-between px-2 py-4">
            <div className="text-sm text-muted-foreground">
              {sites.length} site{sites.length !== 1 ? 's' : ''} total
              {sites.filter(s => s.status === 'up').length > 0 && (
                <span className="ml-2">
                  • {sites.filter(s => s.status === 'up').length} online
                </span>
              )}
              {sites.filter(s => s.status === 'down').length > 0 && (
                <span className="ml-2 text-red-600">
                  • {sites.filter(s => s.status === 'down').length} down
                </span>
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
} 
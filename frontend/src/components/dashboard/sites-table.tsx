'use client';

import { useState } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { MoreHorizontal, ExternalLink, Trash2, RefreshCw, Lock, Unlock, Shield, Crown, Server } from "lucide-react";
import { type SiteStatus, refreshAgentSecurity } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useAuth } from "@/context/AuthContext";

interface SitesTableProps {
  sites: SiteStatus[];
  onDeleteSite: (id: number) => void;
  onCheckSite: (id: number) => void;
  isChecking: boolean;
}

export function SitesTable({ sites, onDeleteSite, onCheckSite, isChecking }: SitesTableProps) {
  const [sortBy, setSortBy] = useState<'name' | 'status' | 'response_time' | 'ip_address' | 'protocol'>('name');
  const [sortOrder, setSortOrder] = useState<'asc' | 'desc'>('asc');
  const [refreshingSecurity, setRefreshingSecurity] = useState<number | null>(null);
  const { isAuthenticated } = useAuth();

  // Filter to show only agents (not regular resources)
  const agents = sites.filter(site => {
    const url = site.url || '';
    const name = site.name || '';
    return (
      url.includes(':8081') || 
      url.toLowerCase().includes('agent') || 
      name.toLowerCase().includes('agent') ||
      site.connection_type === 'agent'
    );
  });

  // Create master/controller node if no agents exist
  const masterNode = {
    id: -1, // Special ID for master
    name: 'Controller (Master)',
    url: window.location.origin,
    connection_type: 'controller' as const,
    status: 'up',
    response_time: 0.001,
    hostname: window.location.hostname,
    ip_address: '127.0.0.1', // Localhost for master
    protocol: window.location.protocol.replace(':', ''),
    is_encrypted: window.location.protocol === 'https:',
    connection_status: 'connected' as const,
    checked_at: new Date().toISOString(),
    total_up: 1,
    total_down: 0,
    created_at: new Date().toISOString(),
    // Security properties
    tls_version: window.location.protocol === 'https:' ? 'TLS 1.3' : undefined,
    cipher_suite: window.location.protocol === 'https:' ? 'TLS_AES_256_GCM_SHA384' : undefined,
    key_strength: window.location.protocol === 'https:' ? 256 : undefined,
    http_version: 'HTTP/2',
    agent_port: undefined,
    fallback_used: false
  };

  // Show agents, or master if no agents exist
  const displayNodes = agents.length > 0 ? agents : [masterNode];

  const handleSort = (column: typeof sortBy) => {
    if (sortBy === column) {
      setSortOrder(sortOrder === 'asc' ? 'desc' : 'asc');
    } else {
      setSortBy(column);
      setSortOrder('asc');
    }
  };

  const handleRefreshSecurity = async (siteId: number) => {
    if (siteId === -1) return; // Don't refresh security for master node
    
    try {
      setRefreshingSecurity(siteId);
      await refreshAgentSecurity(siteId);
      // Trigger a refresh of the sites data
      onCheckSite(siteId);
    } catch (error) {
      console.error('Failed to refresh security info:', error);
    } finally {
      setRefreshingSecurity(null);
    }
  };

  const sortedNodes = [...displayNodes].sort((a, b) => {
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
      case 'ip_address':
        aValue = a.ip_address || '';
        bValue = b.ip_address || '';
        break;
      case 'protocol':
        aValue = a.protocol || '';
        bValue = b.protocol || '';
        break;
      default:
        aValue = a.name.toLowerCase();
        bValue = b.name.toLowerCase();
    }

    if (aValue < bValue) return sortOrder === 'asc' ? -1 : 1;
    if (aValue > bValue) return sortOrder === 'asc' ? 1 : -1;
    return 0;
  });

  const getStatusBadge = (status: string, connectionStatus?: string) => {
    if (connectionStatus === 'connected') {
      return <Badge className="bg-green-100 text-green-800 hover:bg-green-100">Connected</Badge>;
    } else if (connectionStatus === 'disconnected') {
      return <Badge variant="secondary">Disconnected</Badge>;
    } else if (connectionStatus === 'failed' || connectionStatus === 'error') {
      return <Badge variant="destructive">Failed</Badge>;
    }
    
    // Fallback to original status logic
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

  const getNodeIcon = (node: any) => {
    if (node.connection_type === 'controller' || node.id === -1) {
      return <Crown className="h-4 w-4 text-yellow-600" />;
    } else {
      return <Server className="h-4 w-4 text-blue-600" />;
    }
  };

  const getProtocolBadge = (site: SiteStatus) => {
    const protocol = site.protocol?.toUpperCase() || 'UNKNOWN';
    const isEncrypted = site.is_encrypted;
    
    return (
      <div className="relative group flex items-center">
        <div className="flex items-center space-x-1">
          <Badge 
            variant={isEncrypted ? "default" : "secondary"}
            className={cn(
              "text-xs",
              isEncrypted && "bg-green-100 text-green-800 border-green-300"
            )}
          >
            {protocol}
          </Badge>
          {isEncrypted && (
            <Lock className="h-3 w-3 text-yellow-500" />
          )}
          {site.fallback_used && (
            <Shield className="h-3 w-3 text-orange-500" />
          )}
        </div>
        
        {/* Security Details Tooltip */}
        {isEncrypted && (
          <div className="absolute bottom-full left-0 mb-2 px-3 py-2 bg-popover text-popover-foreground text-xs rounded-md border shadow-lg opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none z-50 min-w-max">
            <div className="space-y-1">
              <div className="font-medium text-green-600">🔒 Encrypted Connection</div>
              {site.tls_version && (
                <div><span className="text-muted-foreground">TLS:</span> {site.tls_version}</div>
              )}
              {site.cipher_suite && (
                <div><span className="text-muted-foreground">Cipher:</span> {site.cipher_suite}</div>
              )}
              {site.key_strength && (
                <div><span className="text-muted-foreground">Key:</span> {site.key_strength}-bit</div>
              )}
              {site.http_version && (
                <div><span className="text-muted-foreground">HTTP:</span> {site.http_version}</div>
              )}
              {site.fallback_used && (
                <div className="text-orange-600 text-xs mt-1">⚠️ Fallback protocol used</div>
              )}
            </div>
            {/* Arrow */}
            <div className="absolute top-full left-4 border-l-4 border-r-4 border-t-4 border-l-transparent border-r-transparent border-t-border"></div>
          </div>
        )}
      </div>
    );
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
        <CardTitle>Remote Agents</CardTitle>
        <CardDescription>
          Secure WebSocket connections to distributed monitoring agents
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
                Agent Name
                {sortBy === 'name' && (
                  <span className="ml-1">{sortOrder === 'asc' ? '↑' : '↓'}</span>
                )}
              </button>
            </div>
            <div className="col-span-2">
              <button
                onClick={() => handleSort('ip_address')}
                className="flex items-center hover:text-foreground"
              >
                IP Address
                {sortBy === 'ip_address' && (
                  <span className="ml-1">{sortOrder === 'asc' ? '↑' : '↓'}</span>
                )}
              </button>
            </div>
            <div className="col-span-2">
              <button
                onClick={() => handleSort('protocol')}
                className="flex items-center hover:text-foreground"
              >
                Protocol
                {sortBy === 'protocol' && (
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
            <div className="col-span-2">Actions</div>
          </div>

          {sortedNodes.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              No sites configured yet. Add your first site to start monitoring.
            </div>
          ) : (
            <div className="divide-y">
              {sortedNodes.map((node) => (
                <div key={node.id} className="grid grid-cols-12 gap-4 p-4 items-center hover:bg-muted/50">
                  <div className="col-span-3">
                    <div className="flex items-center">
                      {getNodeIcon(node)}
                      <div className="ml-2">
                        <div className="font-medium">{node.name}</div>
                        <div className="text-sm text-muted-foreground flex items-center">
                          {node.url}
                          <a
                            href={node.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="ml-2 text-primary hover:text-primary/80"
                          >
                            <ExternalLink className="h-3 w-3" />
                          </a>
                        </div>
                      </div>
                    </div>
                  </div>
                  <div className="col-span-2">
                    {node.ip_address}
                  </div>
                  <div className="col-span-2">
                    {getProtocolBadge(node)}
                  </div>
                  <div className="col-span-1">
                    {getStatusBadge(node.status || 'unknown', node.connection_status)}
                  </div>
                  <div className="col-span-2">
                    <span className={cn(
                      "font-medium",
                      node.response_time && node.response_time > 0.5 ? "text-red-600" : "text-green-600"
                    )}>
                      {formatResponseTime(node.response_time ?? null)}
                    </span>
                  </div>
                  <div className="col-span-2 text-sm text-muted-foreground">
                    {formatLastChecked(node.checked_at)}
                  </div>
                  <div className="col-span-2">
                    <div className="flex items-center gap-2">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => handleRefreshSecurity(node.id)}
                        disabled={refreshingSecurity === node.id || !isAuthenticated}
                        title="Refresh security information"
                      >
                        <RefreshCw className={cn(
                          "h-4 w-4", 
                          refreshingSecurity === node.id && "animate-spin"
                        )} />
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => onCheckSite(node.id)}
                        disabled={isChecking || !isAuthenticated}
                        title="Test connection"
                        className="relative group"
                      >
                        <Shield className={cn(
                          "h-4 w-4", 
                          isChecking && "animate-pulse",
                          node.is_encrypted 
                            ? "text-yellow-500" // Gold for encrypted
                            : (node.hostname === 'localhost' || node.hostname === '127.0.0.1' || node.ip_address === '127.0.0.1')
                            ? "text-orange-500" // Orange for localhost unencrypted
                            : "text-muted-foreground" // Gray for other unencrypted
                        )} />
                        
                        {/* Security info tooltip for shield icon */}
                        <div className="absolute bottom-full left-1/2 transform -translate-x-1/2 mb-2 px-3 py-2 bg-popover text-popover-foreground text-xs rounded-md border shadow-lg opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none z-50 min-w-max">
                          <div className="space-y-1">
                            {node.is_encrypted ? (
                              <>
                                <div className="font-medium text-yellow-600">🛡️ Secure Connection</div>
                                <div><span className="text-muted-foreground">Protocol:</span> {node.protocol?.toUpperCase()}</div>
                                {node.tls_version && (
                                  <div><span className="text-muted-foreground">TLS:</span> {node.tls_version}</div>
                                )}
                                {node.cipher_suite && (
                                  <div><span className="text-muted-foreground">Cipher:</span> {node.cipher_suite}</div>
                                )}
                                {node.key_strength && (
                                  <div><span className="text-muted-foreground">Key:</span> {node.key_strength}-bit</div>
                                )}
                                {node.http_version && (
                                  <div><span className="text-muted-foreground">Version:</span> {node.http_version}</div>
                                )}
                              </>
                            ) : (
                              <>
                                <div className="font-medium text-orange-600">⚠️ Unencrypted Connection</div>
                                <div><span className="text-muted-foreground">Protocol:</span> {node.protocol?.toUpperCase() || 'HTTP'}</div>
                                {(node.hostname === 'localhost' || node.hostname === '127.0.0.1' || node.ip_address === '127.0.0.1') && (
                                  <div className="text-orange-600 text-xs mt-1">🏠 Localhost connection</div>
                                )}
                                <div className="text-muted-foreground text-xs mt-1">Consider using HTTPS/WSS</div>
                              </>
                            )}
                          </div>
                          {/* Arrow */}
                          <div className="absolute top-full left-1/2 transform -translate-x-1/2 border-l-4 border-r-4 border-t-4 border-l-transparent border-r-transparent border-t-border"></div>
                        </div>
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => onDeleteSite(node.id)}
                        className="text-red-600 hover:text-red-700 hover:bg-red-50"
                        title="Remove agent"
                        disabled={!isAuthenticated}
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

        {displayNodes.length > 0 && (
          <div className="flex items-center justify-between px-2 py-4">
            <div className="text-sm text-muted-foreground">
              {displayNodes.length} agent{displayNodes.length !== 1 ? 's' : ''} total
              {displayNodes.filter(s => s.connection_status === 'connected').length > 0 && (
                <span className="ml-2">
                  • {displayNodes.filter(s => s.connection_status === 'connected').length} connected
                </span>
              )}
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  );
} 
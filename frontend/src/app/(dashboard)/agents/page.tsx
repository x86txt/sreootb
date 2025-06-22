'use client';

import { useState, useEffect } from 'react';
import { getSitesStatus } from '@/lib/api';
import { type SiteStatus } from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Activity, Server, Crown, Lock, Shield, ExternalLink } from 'lucide-react';
import { cn } from '@/lib/utils';
import { AddAgentDialog } from '@/components/AddAgentDialog';

interface Agent {
  id: number;
  name: string;
  description?: string;
  status: string;
  last_seen?: string;
  created_at: string;
}

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchAgents = async () => {
    try {
      setLoading(true);
      setError(null);
      const response = await fetch('/api/agents');
      if (!response.ok) {
        throw new Error('Failed to fetch agents');
      }
      const agentData = await response.json();
      setAgents(agentData);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch agents');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchAgents();
  }, []);

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'online':
        return <Badge className="bg-green-100 text-green-800 hover:bg-green-100">Online</Badge>;
      case 'offline':
        return <Badge variant="destructive">Offline</Badge>;
      default:
        return <Badge variant="secondary">Unknown</Badge>;
    }
  };

  const formatLastSeen = (lastSeen?: string) => {
    if (!lastSeen) return 'Never';
    const date = new Date(lastSeen);
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

  const handleAgentAdded = () => {
    fetchAgents(); // Refresh the agents list
  };

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex items-center space-x-2">
          <Activity className="h-6 w-6 animate-pulse text-primary" />
          <span className="text-lg">Loading Agent Data...</span>
        </div>
      </div>
    );
  }

  return (
    <>
      <header className="border-b bg-card px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold">Agents</h1>
            <p className="text-muted-foreground">
              A list of your registered monitoring agents.
            </p>
          </div>
          <AddAgentDialog onAgentAdded={handleAgentAdded} />
        </div>
      </header>
      <main className="p-6 space-y-6">
        {error && (
          <Card className="border-destructive bg-destructive/10">
            <CardContent className="p-4"><p className="text-destructive">{error}</p></CardContent>
          </Card>
        )}
        <Card>
          <CardHeader>
            <CardTitle>Registered Agents</CardTitle>
            <CardDescription>
              These are the distributed agents reporting monitoring data.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-md border">
              <div className="grid grid-cols-12 gap-4 p-4 font-medium text-sm bg-muted/50 border-b">
                <div className="col-span-4">Agent Name</div>
                <div className="col-span-3">Description</div>
                <div className="col-span-2">Status</div>
                <div className="col-span-3">Last Seen</div>
              </div>
              {agents.length === 0 && !loading && (
                <div className="p-8 text-center text-muted-foreground">
                  No agents registered. Use the "Add Agent" button to register your first agent.
                </div>
              )}
              <div className="divide-y">
                {agents.map((agent) => (
                  <div key={agent.id} className="grid grid-cols-12 gap-4 p-4 items-center hover:bg-muted/50">
                    <div className="col-span-4 flex items-center">
                      <Server className="h-4 w-4 text-blue-600 mr-2" />
                      <div>
                        <div className="font-medium">{agent.name}</div>
                        <div className="text-sm text-muted-foreground">
                          ID: {agent.id}
                        </div>
                      </div>
                    </div>
                    <div className="col-span-3 text-sm text-muted-foreground">
                      {agent.description || 'No description'}
                    </div>
                    <div className="col-span-2">{getStatusBadge(agent.status)}</div>
                    <div className="col-span-3 text-sm text-muted-foreground">
                      {formatLastSeen(agent.last_seen)}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>
      </main>
    </>
  );
} 
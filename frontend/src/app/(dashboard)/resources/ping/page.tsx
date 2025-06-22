'use client';

import { useState, useEffect } from 'react';
import { getSitesStatus, getSiteHistory } from '@/lib/api';
import { type SiteStatus, type SiteCheck } from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Activity, AlertTriangle, CheckCircle, ExternalLink } from 'lucide-react';

export default function PingResourcesPage() {
  const [sites, setSites] = useState<SiteStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [history, setHistory] = useState<{ [key: number]: SiteCheck[] }>({});

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        setError(null);
        const allSites = await getSitesStatus();
        const pingSites = allSites.filter(site => site.url.startsWith('ping://'));
        setSites(pingSites);

        // Fetch history for each ping site
        const historyPromises = pingSites.map(site => getSiteHistory(site.id, 100));
        const histories = await Promise.all(historyPromises);
        
        const historyMap: { [key: number]: SiteCheck[] } = {};
        pingSites.forEach((site, index) => {
          historyMap[site.id] = histories[index];
        });
        setHistory(historyMap);

      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch Ping resources');
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, []);

  const calculateLatency = (siteId: number) => {
    const siteHistory = history[siteId] || [];
    const responseTimes = siteHistory
      .filter(h => h.status === 'up' && h.response_time)
      .map(h => h.response_time! * 1000); // in ms

    if (responseTimes.length === 0) {
      return { min: 'N/A', max: 'N/A', avg: 'N/A' };
    }

    const min = Math.min(...responseTimes).toFixed(0);
    const max = Math.max(...responseTimes).toFixed(0);
    const avg = (responseTimes.reduce((a, b) => a + b, 0) / responseTimes.length).toFixed(0);

    return { min, max, avg };
  };

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex items-center space-x-2">
          <Activity className="h-6 w-6 animate-pulse text-primary" />
          <span className="text-lg">Loading Ping Resources...</span>
        </div>
      </div>
    );
  }

  return (
    <>
      <header className="border-b bg-card px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold">Ping Latency</h1>
            <p className="text-muted-foreground">
              Latency metrics for your monitored ping resources.
            </p>
          </div>
        </div>
      </header>

      <main className="p-6 space-y-6">
        {error && (
          <Card className="border-destructive bg-destructive/10">
            <CardContent className="p-4">
              <p className="text-destructive">{error}</p>
            </CardContent>
          </Card>
        )}
        <Card>
          <CardHeader>
            <CardTitle>Ping Resources</CardTitle>
            <CardDescription>
              A list of all your monitored ping resources.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-md border">
              <div className="grid grid-cols-12 gap-2 p-4 font-medium text-sm bg-muted/50 border-b">
                <div className="col-span-5">Name</div>
                <div className="col-span-2">Status</div>
                <div className="col-span-1">Last (ms)</div>
                <div className="col-span-1">Min (ms)</div>
                <div className="col-span-1">Max (ms)</div>
                <div className="col-span-2">Avg (ms)</div>
              </div>
              {sites.length === 0 && !loading && (
                <div className="p-8 text-center text-muted-foreground">
                  No Ping resources found.
                </div>
              )}
              <div className="divide-y">
                {sites.map((site) => {
                  const { min, max, avg } = calculateLatency(site.id);
                  const last = site.response_time ? (site.response_time * 1000).toFixed(0) : 'N/A';
                  return (
                    <div key={site.id} className="grid grid-cols-12 gap-2 p-4 items-center hover:bg-muted/50">
                      <div className="col-span-5 font-medium">
                        <div className="flex items-center">
                          {site.name}
                        </div>
                        <div className="text-sm text-muted-foreground">{site.url.replace('ping://', '')}</div>
                      </div>
                      <div className="col-span-2">
                        {site.status === 'up' ? (
                          <Badge className="bg-green-100 text-green-800 hover:bg-green-100">
                            <CheckCircle className="h-3 w-3 mr-1" />
                            Up
                          </Badge>
                        ) : (
                          <Badge variant="destructive">
                            <AlertTriangle className="h-3 w-3 mr-1" />
                            Down
                          </Badge>
                        )}
                      </div>
                      <div className="col-span-1">{last}</div>
                      <div className="col-span-1">{min}</div>
                      <div className="col-span-1">{max}</div>
                      <div className="col-span-2">{avg}</div>
                    </div>
                  );
                })}
              </div>
            </div>
          </CardContent>
        </Card>
      </main>
    </>
  );
} 
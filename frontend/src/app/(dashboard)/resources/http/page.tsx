'use client';

import { useState, useEffect } from 'react';
import { getSitesStatus } from '@/lib/api';
import { type SiteStatus } from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import { Activity, AlertTriangle, CheckCircle, ExternalLink } from 'lucide-react';

export default function HttpResourcesPage() {
  const [sites, setSites] = useState<SiteStatus[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true);
        setError(null);
        const allSites = await getSitesStatus();
        const httpSites = allSites.filter(site => site.url.startsWith('http'));
        setSites(httpSites);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to fetch HTTP/S resources');
      } finally {
        setLoading(false);
      }
    };
    fetchData();
  }, []);

  const calculateRED = (site: SiteStatus) => {
    const totalChecks = site.total_up + site.total_down;
    // Assuming scan_interval is a string like "60s"
    const intervalMatch = site.scan_interval?.match(/(\d+)/);
    const intervalSeconds = intervalMatch ? parseInt(intervalMatch[1], 10) : 60;

    const rate = totalChecks > 0 ? (totalChecks / (totalChecks * intervalSeconds)) * 60 : 0; // Requests per minute
    const errorRate = totalChecks > 0 ? (site.total_down / totalChecks) * 100 : 0; // Percentage
    const duration = site.response_time ? site.response_time * 1000 : 0; // in ms

    return {
      rate: rate.toFixed(2),
      errorRate: errorRate.toFixed(2),
      duration: duration.toFixed(0),
    };
  };

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex items-center space-x-2">
          <Activity className="h-6 w-6 animate-pulse text-primary" />
          <span className="text-lg">Loading HTTP/S Resources...</span>
        </div>
      </div>
    );
  }

  return (
    <>
      <header className="border-b bg-card px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold">HTTP(S) Resources</h1>
            <p className="text-muted-foreground">
              Rate, Error, and Duration (RED) metrics for your web resources.
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
            <CardTitle>Monitored Sites</CardTitle>
            <CardDescription>
              A list of all your monitored HTTP and HTTPS sites.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-md border">
              <div className="grid grid-cols-6 gap-4 p-4 font-medium text-sm bg-muted/50 border-b">
                <div className="col-span-2">Name</div>
                <div>Status</div>
                <div>Rate (req/min)</div>
                <div>Error Rate (%)</div>
                <div>Duration (ms)</div>
              </div>
              {sites.length === 0 && !loading && (
                <div className="p-8 text-center text-muted-foreground">
                  No HTTP(S) resources found.
                </div>
              )}
              <div className="divide-y">
                {sites.map((site) => {
                  const { rate, errorRate, duration } = calculateRED(site);
                  return (
                    <div key={site.id} className="grid grid-cols-6 gap-4 p-4 items-center hover:bg-muted/50">
                      <div className="col-span-2 font-medium">
                        <div className="flex items-center">
                          {site.name}
                          <a
                            href={site.url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="ml-2 text-primary hover:text-primary/80"
                          >
                            <ExternalLink className="h-3 w-3" />
                          </a>
                        </div>
                        <div className="text-sm text-muted-foreground">{site.url}</div>
                      </div>
                      <div>
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
                      <div>{rate}</div>
                      <div>{errorRate}</div>
                      <div>{duration}</div>
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
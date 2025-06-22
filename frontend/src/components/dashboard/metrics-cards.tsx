'use client';

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Activity, Clock, TrendingUp, TrendingDown, CheckCircle, XCircle, Globe } from "lucide-react";
import { type MonitorStats, type SiteStatus } from "@/lib/api";
import { cn } from "@/lib/utils";

interface MetricsCardsProps {
  stats: MonitorStats | null;
  sites: SiteStatus[];
}

export function MetricsCards({ stats, sites }: MetricsCardsProps) {
  if (!stats) {
    return null;
  }

  const uptimePercentage = stats.total_sites > 0 
    ? ((stats.sites_up / stats.total_sites) * 100).toFixed(1)
    : "0";

  const avgResponseTimeMs = stats.average_response_time 
    ? Math.round(stats.average_response_time * 1000)
    : 0;

  // Calculate change indicators (you could enhance this with historical data)
  const uptimeChange = parseFloat(uptimePercentage) >= 95 ? "up" : "down";
  const responseChange = avgResponseTimeMs <= 500 ? "up" : "down";

  // Helper function to determine uptime icon color and animation
  const getUptimeIconColor = (uptime: number) => {
    if (uptime >= 99.99) return "text-green-500";
    if (uptime >= 99.95) return "text-yellow-500"; 
    return "text-red-500";
  };

  // Helper function to determine response time icon color
  const getResponseTimeIconColor = (responseTime: number) => {
    if (responseTime <= 200) return "text-green-500";
    if (responseTime <= 500) return "text-yellow-500";
    return "text-red-500";
  };

  // Helper function to determine sites online icon color
  const getSitesOnlineIconColor = (sitesUp: number, totalSites: number) => {
    if (totalSites === 0) return "text-muted-foreground";
    const percentage = (sitesUp / totalSites) * 100;
    if (percentage === 100) return "text-green-500";
    if (percentage >= 80) return "text-yellow-500";
    return "text-red-500";
  };

  const uptimeValue = parseFloat(uptimePercentage);

  return (
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
      {/* Total Sites */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Total Resources</CardTitle>
          <Globe className="h-4 w-4 text-green-500" />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{stats.total_sites}</div>
          <p className="text-xs text-muted-foreground">
            Resources being monitored
          </p>
        </CardContent>
      </Card>

      {/* Overall Uptime */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Overall Uptime</CardTitle>
          <Activity className={cn("h-4 w-4", getUptimeIconColor(uptimeValue))} />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">{uptimePercentage}%</div>
          <p className="text-xs text-muted-foreground flex items-center">
            {uptimeChange === "up" ? (
              <TrendingUp className="mr-1 h-3 w-3 text-green-500" />
            ) : (
              <TrendingDown className="mr-1 h-3 w-3 text-red-500" />
            )}
            {uptimeValue >= 99.99 ? "Excellent uptime" : 
             uptimeValue >= 99.95 ? "Good uptime" : "Needs attention"}
          </p>
        </CardContent>
      </Card>

      {/* Sites Online */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Resources Online</CardTitle>
          <CheckCircle className={cn(
            "h-4 w-4",
            getSitesOnlineIconColor(stats.sites_up, stats.total_sites)
          )} />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold text-green-600">{stats.sites_up}</div>
          <p className="text-xs text-muted-foreground">
            {stats.sites_down > 0 && (
              <span className="text-red-600">{stats.sites_down} down</span>
            )}
            {stats.sites_down === 0 && "All systems operational"}
          </p>
        </CardContent>
      </Card>

      {/* Average Response Time */}
      <Card>
        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
          <CardTitle className="text-sm font-medium">Avg Response Time</CardTitle>
          <Clock className={cn(
            "h-4 w-4",
            getResponseTimeIconColor(avgResponseTimeMs)
          )} />
        </CardHeader>
        <CardContent>
          <div className="text-2xl font-bold">
            {avgResponseTimeMs > 0 ? `${avgResponseTimeMs}ms` : "N/A"}
          </div>
          <p className="text-xs text-muted-foreground flex items-center">
            {responseChange === "up" ? (
              <TrendingUp className="mr-1 h-3 w-3 text-green-500" />
            ) : (
              <TrendingDown className="mr-1 h-3 w-3 text-red-500" />
            )}
            {avgResponseTimeMs <= 200 ? "Excellent response" :
             avgResponseTimeMs <= 500 ? "Good response" : "Slow response"}
          </p>
        </CardContent>
      </Card>
    </div>
  );
} 
'use client';

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { type MonitorStats, type SiteStatus } from "@/lib/api";
import { cn } from "@/lib/utils";
import { Activity, CheckCircle, Clock, Globe, TrendingDown, TrendingUp } from "lucide-react";

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

  // Use the same unit conversion logic as the chart component
  const avgResponseTimeMs = stats.average_response_time ? (() => {
    const time = stats.average_response_time;
    if (time >= 1) {
      // Definitely seconds (e.g., 1.234 seconds)
      return Math.round(time * 1000);
    } else if (time > 0.001) {
      // Likely seconds (e.g., 0.066 seconds = 66ms)
      return Math.round(time * 1000);
    } else {
      // Likely already in milliseconds or very fast response
      return Math.round(time);
    }
  })() : 0;

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

  // Danger level detection functions
  const getUptimeDangerLevel = (uptime: number) => {
    if (uptime < 95) return "critical";
    if (uptime < 99) return "warning";
    return "safe";
  };

  const getResponseTimeDangerLevel = (responseTime: number) => {
    if (responseTime > 2000) return "critical";
    if (responseTime > 1000) return "warning";
    return "safe";
  };

  const getSitesOnlineDangerLevel = (sitesUp: number, totalSites: number) => {
    if (totalSites === 0) return "safe";
    const percentage = (sitesUp / totalSites) * 100;
    if (percentage < 50) return "critical";
    if (percentage < 80) return "warning";
    return "safe";
  };

  // Animation class helper
  const getDangerAnimationClass = (dangerLevel: string) => {
    if (dangerLevel === "critical" || dangerLevel === "warning") {
      return "animate-glow-flash";
    }
    return "";
  };

  const uptimeValue = parseFloat(uptimePercentage);
  const uptimeDangerLevel = getUptimeDangerLevel(uptimeValue);
  const responseTimeDangerLevel = getResponseTimeDangerLevel(avgResponseTimeMs);
  const sitesOnlineDangerLevel = getSitesOnlineDangerLevel(stats.sites_up, stats.total_sites);

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
      <Card className={getDangerAnimationClass(uptimeDangerLevel)}>
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
             uptimeValue >= 99.95 ? "Good uptime" : 
             uptimeValue >= 95 ? "Needs attention" : "Critical - Immediate action required"}
          </p>
        </CardContent>
      </Card>

      {/* Sites Online */}
      <Card className={getDangerAnimationClass(sitesOnlineDangerLevel)}>
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
              <span className="text-red-600">
                {stats.sites_down} down
                {sitesOnlineDangerLevel === "critical" && " - Critical"}
                {sitesOnlineDangerLevel === "warning" && " - Warning"}
              </span>
            )}
            {stats.sites_down === 0 && "All systems operational"}
          </p>
        </CardContent>
      </Card>

      {/* Average Response Time */}
      <Card className={getDangerAnimationClass(responseTimeDangerLevel)}>
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
             avgResponseTimeMs <= 500 ? "Good response" : 
             avgResponseTimeMs <= 1000 ? "Slow response" :
             avgResponseTimeMs <= 2000 ? "Very slow - Needs attention" : "Critical - Performance degraded"}
          </p>
        </CardContent>
      </Card>
    </div>
  );
} 
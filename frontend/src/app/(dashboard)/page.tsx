'use client';

import { useState, useEffect } from 'react';
import { getSitesStatus, getMonitorStats, createSite, deleteSite, triggerManualCheck } from '@/lib/api';
import { type SiteStatus, type MonitorStats, type CreateSiteRequest } from '@/lib/api';
import { AddSiteDialog } from '@/components/AddSiteDialog';
import { DeleteSitesDialog } from '@/components/DeleteSitesDialog';
import { MetricsCards } from '@/components/dashboard/metrics-cards';
import { ResponseTimeChart } from '@/components/dashboard/response-time-chart';
import { SitesTable } from '@/components/dashboard/sites-table';
import { ThemeToggle } from '@/components/theme-toggle';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { Activity, Plus, RefreshCw, Trash2, Clock } from 'lucide-react';
import { useAuth } from '@/context/AuthContext';

export default function Home() {
  const [sites, setSites] = useState<SiteStatus[]>([]);
  const [stats, setStats] = useState<MonitorStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [isChecking, setIsChecking] = useState(false);
  const [showAddDialog, setShowAddDialog] = useState(false);
  const [showDeleteDialog, setShowDeleteDialog] = useState(false);
  const [autoRefreshInterval, setAutoRefreshInterval] = useState('30');
  const { isAuthenticated } = useAuth();

  const AUTO_REFRESH_OPTIONS = [
    { value: 'off', label: 'Off' },
    { value: '1', label: '1s' },
    { value: '5', label: '5s' },
    { value: '10', label: '10s' },
    { value: '30', label: '30s' },
    { value: '60', label: '1m' },
    { value: '300', label: '5m' },
    { value: '600', label: '10m' },
    { value: '1800', label: '30m' },
    { value: '3600', label: '1h' }
  ];

  const fetchData = async () => {
    try {
      setError(null);
      const [sitesData, statsData] = await Promise.all([
        getSitesStatus(),
        getMonitorStats()
      ]);
      setSites(sitesData);
      setStats(statsData);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch data');
    } finally {
      setLoading(false);
    }
  };

  const handleAddSite = async (siteData: CreateSiteRequest) => {
    try {
      await createSite(siteData);
      await fetchData();
    } catch (err) {
      throw err;
    }
  };

  const handleDeleteSite = async (id: number) => {
    try {
      await deleteSite(id);
      await fetchData();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete site');
    }
  };

  const handleManualCheck = async () => {
    setIsChecking(true);
    try {
      await triggerManualCheck();
      await fetchData();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to trigger manual check');
    } finally {
      setIsChecking(false);
    }
  };

  const handleCheckSite = async (id: number) => {
    // For now, trigger a full manual check
    // In a real app, you might want to check just this site
    await handleManualCheck();
  };

  const handleDeleteSelectedSites = async (siteIds: number[]) => {
    try {
      // Delete selected sites one by one
      for (const siteId of siteIds) {
        await deleteSite(siteId);
      }
      await fetchData();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete sites');
      throw err; // Re-throw so the dialog can handle it
    }
  };

  useEffect(() => {
    fetchData();
    
    if (autoRefreshInterval === 'off') {
      return; // No auto-refresh
    }
    
    const intervalMs = parseInt(autoRefreshInterval) * 1000;
    const interval = setInterval(fetchData, intervalMs);
    return () => clearInterval(interval);
  }, [autoRefreshInterval]);

  if (loading) {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="flex items-center space-x-2">
          <Activity className="h-6 w-6 animate-pulse text-primary" />
          <span className="text-lg">Loading monitoring data...</span>
        </div>
      </div>
    );
  }

  return (
    <>
      {/* Header */}
      <header className="border-b bg-card px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold">Dashboard</h1>
            <p className="text-muted-foreground">
              Website uptime monitoring overview
            </p>
          </div>
          <div className="flex items-center space-x-2">
            <Button
              onClick={() => setShowAddDialog(true)}
              size="sm"
              className="flex items-center space-x-2"
              disabled={!isAuthenticated}
            >
              <Plus className="h-4 w-4" />
              <span>Add Site</span>
            </Button>
            <Button
              onClick={() => setShowDeleteDialog(true)}
              variant="destructive"
              size="sm"
              disabled={sites.length === 0 || !isAuthenticated}
              className="flex items-center space-x-2"
            >
              <Trash2 className="h-4 w-4" />
              <span>Delete Sites</span>
            </Button>
            <Button
              onClick={handleManualCheck}
              variant="outline"
              size="sm"
              disabled={isChecking || !isAuthenticated}
              className="flex items-center space-x-2"
            >
              <RefreshCw className={`h-4 w-4 ${isChecking ? 'animate-spin' : ''}`} />
              <span>{isChecking ? 'Checking...' : 'Check Now'}</span>
            </Button>
            <div className="flex items-center space-x-2">
              <Clock className="h-4 w-4 text-muted-foreground" />
              <Select value={autoRefreshInterval} onValueChange={setAutoRefreshInterval}>
                <SelectTrigger className="w-20">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <div className="px-2 py-1.5 text-sm font-medium text-muted-foreground border-b">
                    Auto-refresh interval
                  </div>
                  {AUTO_REFRESH_OPTIONS.map(option => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <ThemeToggle />
          </div>
        </div>
      </header>

      {/* Content */}
      <main className="p-6 space-y-6">
        {error && (
          <Card className="border-destructive bg-destructive/10">
            <CardContent className="p-4">
              <p className="text-destructive">{error}</p>
            </CardContent>
          </Card>
        )}

        {/* Metrics Cards */}
        <MetricsCards stats={stats} sites={sites} />

        {/* Response Time Chart */}
        {sites.length > 0 && <ResponseTimeChart sites={sites} />}

        {/* Sites Table */}
        <SitesTable
          sites={sites}
          onDeleteSite={handleDeleteSite}
          onCheckSite={handleCheckSite}
          isChecking={isChecking}
        />

        {sites.length === 0 && !error && (
          <Card>
            <CardContent className="p-12 text-center">
              <Activity className="mx-auto h-12 w-12 text-muted-foreground mb-4" />
              <h3 className="text-lg font-medium mb-2">No sites configured yet</h3>
              <p className="text-muted-foreground mb-6">
                Start monitoring your websites by adding your first site.
              </p>
              <Button onClick={() => setShowAddDialog(true)} className="flex items-center space-x-2" disabled={!isAuthenticated}>
                <Plus className="h-4 w-4" />
                <span>Add Your First Site</span>
              </Button>
            </CardContent>
          </Card>
        )}
      </main>

      {/* Add Site Dialog */}
      <AddSiteDialog
        open={showAddDialog}
        onOpenChange={setShowAddDialog}
        onSiteAdded={fetchData}
      />

      {/* Delete Sites Dialog */}
      <DeleteSitesDialog
        open={showDeleteDialog}
        onOpenChange={setShowDeleteDialog}
        sites={sites}
        onDeleteSites={handleDeleteSelectedSites}
      />
    </>
  );
} 
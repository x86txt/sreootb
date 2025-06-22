"use client";

import { useState, useEffect } from 'react';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer, Legend, AreaChart, Area } from 'recharts';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select';
import { MultiSelect } from '@/components/ui/multi-select';
import { Badge } from '@/components/ui/badge';
import { Activity, TrendingUp, Trash2 } from 'lucide-react';
import { getSitesAnalytics, type SiteAnalytics, type SiteStatus } from '@/lib/api';
import { Input } from '@/components/ui/input';
import { Button } from '@/components/ui/button';

interface ResponseTimeChartProps {
  sites: SiteStatus[];
}

const TIME_RANGES = [
  { value: '0.000278', label: 'Last 1 second', custom: false },
  { value: '0.00139', label: 'Last 5 seconds', custom: false },
  { value: '0.00833', label: 'Last 30 seconds', custom: false },
  { value: '0.0167', label: 'Last 1 minute', custom: false },
  { value: '0.0833', label: 'Last 5 minutes', custom: false },
  { value: '0.25', label: 'Last 15 minutes', custom: false },
  { value: '1', label: 'Last 1 hour', custom: false },
  { value: '6', label: 'Last 6 hours', custom: false },
  { value: '24', label: 'Last 24 hours', custom: false },
  { value: '168', label: 'Last 7 days', custom: false },
  { value: 'custom', label: 'Custom...', custom: true }
];

const CHART_COLORS = [
  '#8884d8', '#82ca9d', '#ffc658', '#ff7c7c', '#8dd1e1', '#d084d0',
  '#87d068', '#ffb347', '#ff6b6b', '#4ecdc4', '#45b7d1', '#f39c12'
];

export function ResponseTimeChart({ sites }: ResponseTimeChartProps) {
  const [selectedSites, setSelectedSites] = useState<string[]>(['all']);
  const [timeRange, setTimeRange] = useState('0.0833');
  const [customTimeInput, setCustomTimeInput] = useState('');
  const [showCustomInput, setShowCustomInput] = useState(false);
  const [analyticsData, setAnalyticsData] = useState<SiteAnalytics | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const siteOptions = [
    { value: 'all', label: 'All Resources (Average)' },
    ...sites.map(site => ({
      value: site.id.toString(),
      label: site.name
    }))
  ];

  const formatResponseTime = (value: number | null | undefined): string => {
    if (value === null || value === undefined) return 'No data';
    if (value < 1) {
      return `${value.toFixed(2)}ms`;
    }
    return `${Math.round(value)}ms`;
  };

  const parseCustomTimeToHours = (timeStr: string): number | null => {
    try {
      const match = timeStr.trim().toLowerCase().match(/^(\d+(?:\.\d+)?)([smhd])$/);
      if (!match) return null;
      
      const value = parseFloat(match[1]);
      const unit = match[2];
      
      switch (unit) {
        case 's': return value / 3600; // seconds to hours
        case 'm': return value / 60;   // minutes to hours  
        case 'h': return value;        // hours
        case 'd': return value * 24;   // days to hours
        default: return null;
      }
    } catch {
      return null;
    }
  };

  const fetchAnalytics = async () => {
    try {
      setLoading(true);
      setError(null);
      
      const siteIds = selectedSites.includes('all') ? 'all' : selectedSites.map(id => parseInt(id));
      
      let hours: number;
      if (timeRange === 'custom') {
        if (!customTimeInput.trim()) {
          setError('Please enter a time range (e.g., "5m", "1h")');
          return;
        }
        const customHours = parseCustomTimeToHours(customTimeInput);
        console.log('Parsing custom input:', customTimeInput, '→', customHours, 'hours');
        if (customHours === null) {
          setError('Invalid time format. Use formats like "1s", "5m", "2h", "1d"');
          return;
        }
        hours = customHours;
      } else {
        hours = parseFloat(timeRange);
      }
      
      // Dynamic interval based on time range - always request fine intervals for interpolation
      const intervalMinutes = hours <= 0.01 ? 0.017 : hours <= 0.1 ? 0.083 : hours <= 1 ? 0.5 : hours <= 6 ? 1 : hours <= 24 ? 5 : 60;
      
      console.log('Fetching analytics:', { siteIds, hours, intervalMinutes, selectedSites, timeRange, customTimeInput });
      
      const data = await getSitesAnalytics(siteIds, hours, intervalMinutes);
      console.log('Analytics data received:', data);
      console.log('Data points:', data.data.length);
      console.log('Sample data:', data.data.slice(0, 3));
      
      // Check if we have valid data
      const hasValidData = data.data && data.data.length > 0 && data.data.some(point => {
        const keys = Object.keys(point).filter(k => k.startsWith('site_') || k === 'average');
        return keys.some(k => point[k] !== null);
      });
      
      console.log('Has valid data points:', hasValidData);
      
      if (!hasValidData && data.data.length > 0) {
        console.warn('All data points are null!', data.data);
      }
      
      setAnalyticsData(data);
    } catch (err) {
      console.error('Analytics fetch error:', err);
      setError(err instanceof Error ? err.message : 'Failed to fetch analytics');
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (sites.length > 0) {
      // Don't fetch if custom is selected but no input provided yet
      if (timeRange === 'custom' && !customTimeInput.trim()) {
        return;
      }
      fetchAnalytics();
    }
  }, [selectedSites, timeRange, customTimeInput, sites]);

  // Initialize with all sites if available
  useEffect(() => {
    if (sites.length > 0 && selectedSites.length === 1 && selectedSites[0] === 'all') {
      // Keep 'all' selected by default
    }
  }, [sites]);

  const getDisplayLines = () => {
    if (!analyticsData) return [];
    
    const lines = [];
    
    console.log('Building display lines...', { 
      selectedSites, 
      analyticsDataSites: analyticsData.sites,
      sampleData: analyticsData.data[0] 
    });
    
    if (selectedSites.includes('all') && analyticsData.sites.length >= 1) {
      // For single site when "all" is selected, use site_X key not "average"
      if (analyticsData.sites.length === 1) {
        const siteId = analyticsData.sites[0].id;
        lines.push({
          key: `site_${siteId}`,
          name: analyticsData.sites[0].name,
          color: '#8884d8',
          strokeWidth: 3
        });
        console.log(`Added single site line for "all": site_${siteId}`);
      } else {
        lines.push({
          key: 'average',
          name: 'Average (All Resources)',
          color: '#8884d8',
          strokeWidth: 3
        });
        console.log('Added average line for multiple sites');
      }
    }
    
    // Handle individual resource selections - always show individual lines
    if (!selectedSites.includes('all')) {
      selectedSites.forEach((siteId, index) => {
        const site = analyticsData.sites.find(s => s.id.toString() === siteId);
        if (site) {
          lines.push({
            key: `site_${siteId}`,
            name: site.name,
            color: CHART_COLORS[index % CHART_COLORS.length],
            strokeWidth: 2
          });
          console.log(`Added line for site: ${site.name} (site_${siteId})`);
        }
      });
    }
    
    console.log('Final display lines:', lines);
    return lines;
  };

  const handleSiteSelectionChange = (selected: string[]) => {
    console.log('Site selection change:', { selected, currentSelectedSites: selectedSites });
    
    if (selected.length === 0) {
      console.log('No selections, defaulting to all');
      setSelectedSites(['all']);
      return;
    }

    // If "all" was just added, clear individual selections
    if (selected.includes('all') && !selectedSites.includes('all')) {
      console.log('All option selected, clearing individual selections');
      setSelectedSites(['all']);
      return;
    }

    // If switching from "all" to individual selections, accept all the new selections
    if (selectedSites.includes('all') && !selected.includes('all')) {
      console.log('Switching from all to individual selections:', selected);
      setSelectedSites(selected);
      return;
    }

    // If "all" is being deselected, keep only individual sites
    if (!selected.includes('all') && selectedSites.includes('all')) {
      console.log('All deselected, keeping individual sites');
      setSelectedSites(selected.filter(site => site !== 'all'));
      return;
    }

    // Normal selection handling - accept whatever was selected
    console.log('Normal selection handling:', selected);
    setSelectedSites(selected);
  };

  const getAverageResponseTime = () => {
    if (!analyticsData?.data) return null;
    
    const validPoints = analyticsData.data.filter(point => {
      if (selectedSites.includes('all')) {
        return point.average !== null;
      } else {
        return selectedSites.some(siteId => point[`site_${siteId}`] !== null);
      }
    });
    
    if (validPoints.length === 0) return null;
    
    const sum = validPoints.reduce((acc, point) => {
      if (selectedSites.includes('all')) {
        return acc + (point.average as number || 0);
      } else {
        const siteValues = selectedSites
          .map(siteId => point[`site_${siteId}`] as number)
          .filter(val => val !== null);
        return acc + (siteValues.length > 0 ? siteValues.reduce((a, b) => a + b, 0) / siteValues.length : 0);
      }
    }, 0);
    
    return Math.round(sum / validPoints.length);
  };

  const averageResponseTime = getAverageResponseTime();

  const getTimeRangeDescription = () => {
    if (timeRange === 'custom' && customTimeInput) {
      return `Last ${customTimeInput}`;
    }
    const range = TIME_RANGES.find(r => r.value === timeRange);
    return range ? range.label.replace('Last ', '') : `${timeRange}h`;
  };

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <Activity className="h-5 w-5 text-muted-foreground" />
            <div>
              <CardTitle>Response Time</CardTitle>
              <CardDescription>
                {selectedSites.includes('all') 
                  ? `Average response time across all resources`
                  : `Response time for ${selectedSites.length} selected resource${selectedSites.length !== 1 ? 's' : ''}`}
              </CardDescription>
            </div>
          </div>
          
          {averageResponseTime !== null && (
            <div className="text-right">
              <div className="text-2xl font-bold">{averageResponseTime}ms</div>
              <div className="flex items-center text-sm text-muted-foreground">
                <TrendingUp className="h-4 w-4 mr-1" />
                {getTimeRangeDescription()}
              </div>
            </div>
          )}
        </div>
        
        <div className="flex flex-col sm:flex-row gap-4 mt-4">
          <div className="sm:w-80">
            <label className="text-sm font-medium mb-2 block">Resources</label>
            <div className="flex items-center gap-2">
              <MultiSelect
                options={siteOptions}
                selected={selectedSites}
                onChange={handleSiteSelectionChange}
                placeholder="Select resources to display..."
                className="flex-1"
              />
              {(selectedSites.length > 1 || (selectedSites.length === 1 && !selectedSites.includes('all'))) && (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setSelectedSites(['all'])}
                  className="text-muted-foreground hover:text-foreground px-2"
                  title="Clear all selections"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              )}
            </div>
          </div>
          <div className="flex-1"></div>
          <div className="sm:w-48">
            <label className="text-sm font-medium mb-2 block">Time Range</label>
            <Select value={timeRange} onValueChange={(value) => {
              setTimeRange(value);
              setShowCustomInput(value === 'custom');
              if (value !== 'custom') {
                setCustomTimeInput('');
              }
            }}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {TIME_RANGES.map(range => (
                  <SelectItem key={range.value} value={range.value}>
                    {range.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            
            {showCustomInput && (
              <div className="mt-2">
                <div className="flex space-x-2">
                  <Input
                    placeholder="e.g., 1s, 5m, 2h, 1d"
                    value={customTimeInput}
                    onChange={(e) => setCustomTimeInput(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        fetchAnalytics();
                      }
                    }}
                    className="text-sm flex-1"
                  />
                  <Button
                    size="sm"
                    onClick={fetchAnalytics}
                    disabled={!customTimeInput.trim() || loading}
                    className="px-3"
                  >
                    Fetch
                  </Button>
                </div>
                <p className="text-xs text-muted-foreground mt-1">
                  Format: number + unit (s=seconds, m=minutes, h=hours, d=days)
                </p>
              </div>
            )}
          </div>
        </div>
      </CardHeader>
      
      <CardContent>
        {loading ? (
          <div className="h-80 flex items-center justify-center">
            <div className="flex items-center space-x-2">
              <Activity className="h-5 w-5 animate-pulse" />
              <span>Loading chart data...</span>
            </div>
          </div>
        ) : error ? (
          <div className="h-80 flex items-center justify-center">
            <div className="text-center">
              <p className="text-red-500 mb-2">Failed to load chart data</p>
              <p className="text-sm text-muted-foreground">{error}</p>
            </div>
          </div>
        ) : !analyticsData?.data.length || !analyticsData.data.some(point => {
          const keys = Object.keys(point).filter(k => k.startsWith('site_') || k === 'average');
          return keys.some(k => point[k] !== null);
        }) ? (
          <div className="h-80 flex items-center justify-center">
            <div className="text-center">
              <Activity className="h-12 w-12 text-muted-foreground mx-auto mb-4" />
              <p className="text-muted-foreground">No data available for the selected time range</p>
              {(() => {
                const customHours = timeRange === 'custom' ? parseCustomTimeToHours(customTimeInput) : null;
                return (timeRange === '0.000278' || timeRange === '0.00139' || 
                  (timeRange === 'custom' && customHours !== null && customHours < 0.01));
              })() && (
                <p className="text-xs text-muted-foreground mt-2">
                  Sites are checked based on their scan interval. For very short time ranges,<br/>
                  there might not be any checks within the selected period.
                </p>
              )}
              <p className="text-xs text-muted-foreground mt-2">
                Debug: {analyticsData ? `${analyticsData.data.length} data points, time range: ${getTimeRangeDescription()}` : 'No analytics data'}
              </p>
            </div>
          </div>
        ) : (
          <div className="h-80">
            <ResponsiveContainer width="100%" height="100%">
              {getDisplayLines().length > 1 ? (
                // Area chart for multiple sites with shading
                <AreaChart data={analyticsData.data} margin={{ top: 5, right: 30, left: 20, bottom: 5 }}>
                  <CartesianGrid strokeDasharray="0" vertical={false} className="opacity-10" />
                  <XAxis 
                    dataKey="timestamp" 
                    tick={{ fontSize: 12 }}
                    tickLine={false}
                    axisLine={false}
                  />
                  <YAxis 
                    tick={{ fontSize: 12 }}
                    tickLine={false}
                    axisLine={false}
                    label={{ value: 'Response Time (ms)', angle: -90, position: 'insideLeft' }}
                  />
                  <Tooltip 
                    content={({ active, payload, label }) => {
                      if (active && payload && payload.length) {
                        return (
                          <div className="bg-background border rounded-lg p-3 shadow-lg max-w-sm">
                            <p className="font-medium text-sm mb-2">{label}</p>
                            {payload.map((entry, index) => {
                              // Find the site data for this entry
                              const siteIdMatch = entry.dataKey?.toString().match(/site_(\d+)/);
                              const siteId = siteIdMatch ? parseInt(siteIdMatch[1]) : null;
                              const siteData = siteId ? analyticsData?.sites.find(s => s.id === siteId) : null;
                              
                              return (
                                <div key={index} className="mb-2 last:mb-0">
                                  <div className="flex items-center space-x-2 text-sm mb-1">
                                    <div 
                                      className="w-3 h-3 rounded-full" 
                                      style={{ backgroundColor: entry.color }}
                                    />
                                    <span className="font-medium">{entry.name}:</span>
                                    <span>{formatResponseTime(entry.value as number)}</span>
                                  </div>
                                  {siteData && (
                                    <div className="ml-5 text-xs text-muted-foreground space-y-0.5">
                                      {siteData.hostname && (
                                        <div>
                                          <span className="font-medium">Host:</span> {siteData.hostname}
                                        </div>
                                      )}
                                      {siteData.ip_address && (
                                        <div>
                                          <span className="font-medium">IP:</span> {siteData.ip_address}
                                        </div>
                                      )}
                                      {siteData.last_checked_at && (
                                        <div>
                                          <span className="font-medium">Last check:</span>{' '}
                                          {new Date(siteData.last_checked_at).toLocaleString()}
                                        </div>
                                      )}
                                      {siteData.last_status && (
                                        <div>
                                          <span className="font-medium">Status:</span>{' '}
                                          <span className={siteData.last_status === 'up' ? 'text-green-600' : 'text-red-600'}>
                                            {siteData.last_status}
                                          </span>
                                          {siteData.last_status_code && ` (${siteData.last_status_code})`}
                                        </div>
                                      )}
                                    </div>
                                  )}
                                </div>
                              );
                            })}
                          </div>
                        );
                      }
                      return null;
                    }}
                  />
                  {getDisplayLines().map((line, index) => (
                    <Area
                      key={line.key}
                      type="basis"
                      dataKey={line.key}
                      stroke={line.color}
                      fill={line.color}
                      fillOpacity={0.1 + (index * 0.05)} // Slight opacity variation
                      strokeWidth={line.strokeWidth}
                      name={line.name}
                      connectNulls={true}
                      dot={false}
                    />
                  ))}
                </AreaChart>
              ) : (
                // Line chart for single site (cleaner look)
                <LineChart data={analyticsData.data} margin={{ top: 5, right: 30, left: 20, bottom: 5 }}>
                  <CartesianGrid strokeDasharray="0" vertical={false} className="opacity-10" />
                  <XAxis 
                    dataKey="timestamp" 
                    tick={{ fontSize: 12 }}
                    tickLine={false}
                    axisLine={false}
                  />
                  <YAxis 
                    tick={{ fontSize: 12 }}
                    tickLine={false}
                    axisLine={false}
                    label={{ value: 'Response Time (ms)', angle: -90, position: 'insideLeft' }}
                  />
                  <Tooltip 
                    content={({ active, payload, label }) => {
                      if (active && payload && payload.length) {
                        return (
                          <div className="bg-background border rounded-lg p-3 shadow-lg max-w-sm">
                            <p className="font-medium text-sm mb-2">{label}</p>
                            {payload.map((entry, index) => {
                              // Find the site data for this entry
                              const siteIdMatch = entry.dataKey?.toString().match(/site_(\d+)/);
                              const siteId = siteIdMatch ? parseInt(siteIdMatch[1]) : null;
                              const siteData = siteId ? analyticsData?.sites.find(s => s.id === siteId) : null;
                              
                              return (
                                <div key={index} className="mb-2 last:mb-0">
                                  <div className="flex items-center space-x-2 text-sm mb-1">
                                    <div 
                                      className="w-3 h-3 rounded-full" 
                                      style={{ backgroundColor: entry.color }}
                                    />
                                    <span className="font-medium">{entry.name}:</span>
                                    <span>{formatResponseTime(entry.value as number)}</span>
                                  </div>
                                  {siteData && (
                                    <div className="ml-5 text-xs text-muted-foreground space-y-0.5">
                                      {siteData.hostname && (
                                        <div>
                                          <span className="font-medium">Host:</span> {siteData.hostname}
                                        </div>
                                      )}
                                      {siteData.ip_address && (
                                        <div>
                                          <span className="font-medium">IP:</span> {siteData.ip_address}
                                        </div>
                                      )}
                                      {siteData.last_checked_at && (
                                        <div>
                                          <span className="font-medium">Last check:</span>{' '}
                                          {new Date(siteData.last_checked_at).toLocaleString()}
                                        </div>
                                      )}
                                      {siteData.last_status && (
                                        <div>
                                          <span className="font-medium">Status:</span>{' '}
                                          <span className={siteData.last_status === 'up' ? 'text-green-600' : 'text-red-600'}>
                                            {siteData.last_status}
                                          </span>
                                          {siteData.last_status_code && ` (${siteData.last_status_code})`}
                                        </div>
                                      )}
                                    </div>
                                  )}
                                </div>
                              );
                            })}
                          </div>
                        );
                      }
                      return null;
                    }}
                  />
                  {getDisplayLines().map((line) => (
                    <Line
                      key={line.key}
                      type="basis"
                      dataKey={line.key}
                      stroke={line.color}
                      strokeWidth={line.strokeWidth}
                      dot={false}
                      name={line.name}
                      connectNulls={true}
                    />
                  ))}
                </LineChart>
              )}
            </ResponsiveContainer>
          </div>
        )}
        
        {analyticsData && (
          <div className="flex items-center justify-between mt-4 pt-4 border-t">
            <div className="flex items-center space-x-4 text-sm text-muted-foreground">
              <span>
                Data points: {analyticsData.data.length}
              </span>
              <span>
                Time range: {analyticsData.time_range.hours}h
              </span>
            </div>
            {selectedSites.length > 1 && !selectedSites.includes('all') && (
              <div className="flex flex-wrap gap-1">
                {selectedSites.map((siteId, index) => {
                  const site = analyticsData.sites.find(s => s.id.toString() === siteId);
                  return (
                    <div key={siteId} className="relative group">
                      <Badge 
                        variant="outline" 
                        className="text-xs cursor-help"
                        style={{ borderColor: CHART_COLORS[index % CHART_COLORS.length] }}
                      >
                        {site?.name}
                      </Badge>
                      
                      {/* Hover tooltip */}
                      {site && (
                        <div className="absolute bottom-full left-1/2 transform -translate-x-1/2 mb-2 px-3 py-2 bg-popover text-popover-foreground text-sm rounded-md border shadow-lg opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none z-50 min-w-max">
                          <div className="space-y-1">
                            <div className="font-medium">{site.name}</div>
                            {site.hostname && (
                              <div>
                                <span className="text-muted-foreground">Host:</span> {site.hostname}
                              </div>
                            )}
                            {site.ip_address && (
                              <div>
                                <span className="text-muted-foreground">IP:</span> {site.ip_address}
                              </div>
                            )}
                            {site.last_checked_at && (
                              <div>
                                <span className="text-muted-foreground">Last check:</span>{' '}
                                {new Date(site.last_checked_at).toLocaleString()}
                              </div>
                            )}
                            {site.last_status && (
                              <div>
                                <span className="text-muted-foreground">Status:</span>{' '}
                                <span className={site.last_status === 'up' ? 'text-green-600' : 'text-red-600'}>
                                  {site.last_status}
                                </span>
                                {site.last_status_code && ` (${site.last_status_code})`}
                              </div>
                            )}
                            {site.scan_interval && (
                              <div>
                                <span className="text-muted-foreground">Interval:</span> {site.scan_interval}
                              </div>
                            )}
                          </div>
                          {/* Arrow */}
                          <div className="absolute top-full left-1/2 transform -translate-x-1/2 border-l-4 border-r-4 border-t-4 border-l-transparent border-r-transparent border-t-border"></div>
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
} 
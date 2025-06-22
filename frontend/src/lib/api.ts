const API_BASE = '/api';

export interface Site {
  id: number;
  url: string;
  name: string;
  created_at: string;
}

export interface SiteStatus {
  id: number;
  url: string;
  name: string;
  created_at: string;
  status?: string;
  response_time?: number;
  status_code?: number;
  error_message?: string;
  checked_at?: string;
  total_up: number;
  total_down: number;
  scan_interval?: string;
  // Security and agent information
  connection_type?: 'agent' | 'resource' | 'controller';
  agent_port?: string;
  hostname?: string;
  ip_address?: string;
  protocol?: string;
  is_encrypted?: boolean;
  tls_version?: string;
  cipher_suite?: string;
  key_strength?: number;
  http_version?: string;
  connection_status?: 'connected' | 'disconnected' | 'failed' | 'error';
  fallback_used?: boolean;
}

export interface SiteCheck {
  id: number;
  site_id: number;
  status: string;
  response_time?: number;
  status_code?: number;
  error_message?: string;
  checked_at: string;
}

export interface MonitorStats {
  total_sites: number;
  sites_up: number;
  sites_down: number;
  average_response_time?: number;
}

export interface AppConfig {
  scan_interval: {
    min_seconds: number;
    max_seconds: number;
    range_description: string;
    development_mode: boolean;
  };
}

export interface CreateSiteRequest {
  url: string;
  name: string;
  scan_interval: string;
}

export interface SiteAnalytics {
  data: Array<{
    timestamp: string;
    full_timestamp: string;
    [key: string]: number | string | null; // site_X properties and average
  }>;
  sites: Array<{
    id: number;
    name: string;
    url?: string;
    hostname?: string;
    ip_address?: string;
    last_status?: string;
    last_response_time?: number;
    last_status_code?: number;
    last_checked_at?: string;
    scan_interval?: string;
  }>;
  time_range: {
    start: string;
    end: string;
    hours: number;
  };
}

export async function getSites(): Promise<Site[]> {
  const response = await fetch(`${API_BASE}/sites`);
  if (!response.ok) throw new Error('Failed to fetch sites');
  return response.json();
}

export async function getSitesStatus(): Promise<SiteStatus[]> {
  const response = await fetch(`${API_BASE}/sites/status`);
  if (!response.ok) throw new Error('Failed to fetch sites status');
  return response.json();
}

export async function createSite(site: CreateSiteRequest): Promise<{ id: number; message: string }> {
  const response = await fetch(`${API_BASE}/sites`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(site),
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.detail || 'Failed to create site');
  }
  return response.json();
}

export async function deleteSite(siteId: number): Promise<{ message: string }> {
  const response = await fetch(`${API_BASE}/sites/${siteId}`, {
    method: 'DELETE',
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.detail || 'Failed to delete site');
  }
  return response.json();
}

export async function getSiteHistory(siteId: number, limit = 100): Promise<SiteCheck[]> {
  const response = await fetch(`${API_BASE}/sites/${siteId}/history?limit=${limit}`);
  if (!response.ok) throw new Error('Failed to fetch site history');
  return response.json();
}

export async function getMonitorStats(): Promise<MonitorStats> {
  const response = await fetch(`${API_BASE}/stats`);
  if (!response.ok) throw new Error('Failed to fetch monitor stats');
  return response.json();
}

export async function triggerManualCheck(): Promise<{ message: string; results: any[] }> {
  const response = await fetch(`${API_BASE}/check/manual`, {
    method: 'POST',
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.detail || 'Failed to trigger manual check');
  }
  return response.json();
}

export async function getAppConfig(): Promise<AppConfig> {
  const response = await fetch(`${API_BASE}/config`);
  if (!response.ok) throw new Error('Failed to fetch app configuration');
  return response.json();
}

export async function getSitesAnalytics(
  siteIds?: number[] | 'all',
  hours: number = 1,
  intervalMinutes: number = 5
): Promise<SiteAnalytics> {
  const params = new URLSearchParams({
    hours: hours.toString(),
    interval_minutes: intervalMinutes.toString()
  });
  
  if (siteIds && siteIds !== 'all') {
    params.append('site_ids', siteIds.join(','));
  } else {
    params.append('site_ids', 'all');
  }
  
  const response = await fetch(`${API_BASE}/sites/analytics?${params}`);
  if (!response.ok) throw new Error('Failed to fetch sites analytics');
  return response.json();
}

export async function refreshAgentSecurity(siteId: number): Promise<{ message: string; site_id: number; security_info: any }> {
  const response = await fetch(`${API_BASE}/agents/${siteId}/refresh-security`, {
    method: 'POST',
  });
  if (!response.ok) {
    const error = await response.json();
    throw new Error(error.detail || 'Failed to refresh agent security');
  }
  return response.json();
} 
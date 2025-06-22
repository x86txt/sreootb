'use client';

import { useState } from 'react';
import { Trash2, Globe, ExternalLink } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { formatDate, formatResponseTime, getStatusColor, getStatusDot } from '@/lib/utils';
import { deleteSite, type SiteStatus } from '@/lib/api';

interface SitesListProps {
  sites: SiteStatus[];
  selectedSite: SiteStatus | null;
  onSiteSelect: (site: SiteStatus) => void;
  onSiteDeleted: () => void;
}

export function SitesList({ sites, selectedSite, onSiteSelect, onSiteDeleted }: SitesListProps) {
  const [deletingId, setDeletingId] = useState<number | null>(null);

  const handleDelete = async (siteId: number, siteName: string) => {
    if (!confirm(`Are you sure you want to delete "${siteName}"? This will remove all monitoring data.`)) {
      return;
    }

    setDeletingId(siteId);
    try {
      await deleteSite(siteId);
      onSiteDeleted();
    } catch (error) {
      console.error('Failed to delete site:', error);
      alert('Failed to delete site. Please try again.');
    } finally {
      setDeletingId(null);
    }
  };

  if (sites.length === 0) {
    return (
      <div className="p-4 text-center text-gray-500">
        <Globe className="h-8 w-8 mx-auto mb-2 text-gray-400" />
        <p className="text-sm">No sites being monitored</p>
        <p className="text-xs text-gray-400 mt-1">Add your first site to get started</p>
      </div>
    );
  }

  return (
    <div className="divide-y">
      {sites.map((site) => (
        <div
          key={site.id}
          className={`p-4 cursor-pointer hover:bg-white/60 transition-colors ${
            selectedSite?.id === site.id ? 'bg-white border-r-2 border-blue-500' : ''
          }`}
          onClick={() => onSiteSelect(site)}
        >
          <div className="flex items-start justify-between">
            <div className="flex-1 min-w-0">
              <div className="flex items-center space-x-2 mb-1">
                <div className={`h-2 w-2 rounded-full flex-shrink-0 ${getStatusDot(site.status)}`} />
                <h3 className="font-medium text-sm truncate">{site.name}</h3>
              </div>
              
              <p className="text-xs text-gray-600 truncate mb-2">{site.url}</p>
              
              <div className="flex items-center justify-between">
                <div className="flex flex-col space-y-1">
                  {site.status && (
                    <span className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium border ${getStatusColor(site.status)}`}>
                      {site.status.toUpperCase()}
                    </span>
                  )}
                  
                  {site.response_time && (
                    <span className="text-xs text-gray-500">
                      {formatResponseTime(site.response_time)}
                    </span>
                  )}
                </div>
                
                <div className="flex items-center space-x-1">
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-6 w-6 p-0 hover:bg-gray-100"
                    onClick={(e) => {
                      e.stopPropagation();
                      window.open(site.url, '_blank');
                    }}
                  >
                    <ExternalLink className="h-3 w-3" />
                  </Button>
                  
                  <Button
                    size="sm"
                    variant="ghost"
                    className="h-6 w-6 p-0 hover:bg-red-100 hover:text-red-600"
                    onClick={(e) => {
                      e.stopPropagation();
                      handleDelete(site.id, site.name);
                    }}
                    disabled={deletingId === site.id}
                  >
                    <Trash2 className="h-3 w-3" />
                  </Button>
                </div>
              </div>
              
              {site.checked_at && (
                <p className="text-xs text-gray-400 mt-1">
                  Last checked: {formatDate(site.checked_at)}
                </p>
              )}
            </div>
          </div>
        </div>
      ))}
    </div>
  );
} 
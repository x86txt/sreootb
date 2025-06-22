'use client';

import { useState } from 'react';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Checkbox } from '@/components/ui/checkbox';
import { ExternalLink, Trash2 } from 'lucide-react';
import { type SiteStatus } from '@/lib/api';

interface DeleteSitesDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  sites: SiteStatus[];
  onDeleteSites: (siteIds: number[]) => Promise<void>;
}

export function DeleteSitesDialog({ 
  open, 
  onOpenChange, 
  sites, 
  onDeleteSites 
}: DeleteSitesDialogProps) {
  const [selectedSites, setSelectedSites] = useState<number[]>([]);
  const [isDeleting, setIsDeleting] = useState(false);

  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedSites(sites.map(site => site.id));
    } else {
      setSelectedSites([]);
    }
  };

  const handleSelectSite = (siteId: number, checked: boolean) => {
    if (checked) {
      setSelectedSites(prev => [...prev, siteId]);
    } else {
      setSelectedSites(prev => prev.filter(id => id !== siteId));
    }
  };

  const handleDelete = async () => {
    if (selectedSites.length === 0) return;

    const confirmed = window.confirm(
      `Are you sure you want to delete ${selectedSites.length} site${selectedSites.length !== 1 ? 's' : ''}? This action cannot be undone.`
    );

    if (!confirmed) return;

    setIsDeleting(true);
    try {
      await onDeleteSites(selectedSites);
      setSelectedSites([]);
      onOpenChange(false);
    } catch (error) {
      // Error handling is done in parent component
    } finally {
      setIsDeleting(false);
    }
  };

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      setSelectedSites([]);
    }
    onOpenChange(newOpen);
  };

  const getStatusBadge = (status: string) => {
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

  const allSelected = selectedSites.length === sites.length && sites.length > 0;
  const someSelected = selectedSites.length > 0 && selectedSites.length < sites.length;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="max-w-4xl max-h-[80vh]">
        <DialogHeader>
          <DialogTitle>Delete Sites</DialogTitle>
          <DialogDescription>
            Select the sites you want to delete. This action cannot be undone.
          </DialogDescription>
        </DialogHeader>
        
        <div className="flex-1 overflow-auto">
          {sites.length === 0 ? (
            <div className="p-8 text-center text-muted-foreground">
              No sites to delete.
            </div>
          ) : (
            <div className="rounded-md border">
              {/* Header with Select All */}
              <div className="flex items-center gap-4 p-4 bg-muted/50 border-b">
                <Checkbox
                  checked={allSelected}
                  ref={(ref: any) => {
                    if (ref) ref.indeterminate = someSelected;
                  }}
                  onCheckedChange={(checked: boolean) => handleSelectAll(checked)}
                />
                <div className="flex-1 font-medium text-sm">
                  {selectedSites.length === 0 
                    ? 'Select sites to delete'
                    : `${selectedSites.length} site${selectedSites.length !== 1 ? 's' : ''} selected`
                  }
                </div>
              </div>

              {/* Sites List */}
              <div className="divide-y max-h-[400px] overflow-auto">
                {sites.map((site) => (
                  <div key={site.id} className="flex items-center gap-4 p-4 hover:bg-muted/50">
                    <Checkbox
                      checked={selectedSites.includes(site.id)}
                      onCheckedChange={(checked) => handleSelectSite(site.id, checked as boolean)}
                    />
                    <div className="flex-1 min-w-0">
                      <div className="font-medium truncate">{site.name}</div>
                      <div className="text-sm text-muted-foreground flex items-center gap-2">
                        <span className="truncate">{site.url}</span>
                        <a
                          href={site.url}
                          target="_blank"
                          rel="noopener noreferrer"
                          className="text-primary hover:text-primary/80"
                        >
                          <ExternalLink className="h-3 w-3" />
                        </a>
                      </div>
                    </div>
                    <div className="flex items-center gap-4">
                      {getStatusBadge(site.status || 'unknown')}
                      <div className="text-sm text-muted-foreground">
                        {site.response_time ? `${Math.round(site.response_time * 1000)}ms` : 'N/A'}
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => handleOpenChange(false)}
            disabled={isDeleting}
          >
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={handleDelete}
            disabled={selectedSites.length === 0 || isDeleting}
            className="flex items-center gap-2"
          >
            <Trash2 className="h-4 w-4" />
            {isDeleting 
              ? 'Deleting...' 
              : `Delete ${selectedSites.length} Site${selectedSites.length !== 1 ? 's' : ''}`
            }
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
} 
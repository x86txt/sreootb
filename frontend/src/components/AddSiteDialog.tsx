'use client';

import { useState, useEffect, useRef } from 'react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Checkbox } from '@/components/ui/checkbox';
import { createSite, getAppConfig, type AppConfig } from '@/lib/api';

interface AddSiteDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSiteAdded: () => void;
}

export function AddSiteDialog({ open, onOpenChange, onSiteAdded }: AddSiteDialogProps) {
  const [name, setName] = useState('');
  const [url, setUrl] = useState('');
  const [monitorType, setMonitorType] = useState('https');
  const [scanInterval, setScanInterval] = useState('60s');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [config, setConfig] = useState<AppConfig | null>(null);
  const [showWarning, setShowWarning] = useState(false);
  const [warningData, setWarningData] = useState<{seconds: number} | null>(null);
  const nameInputRef = useRef<HTMLInputElement>(null);

  // Auto-focus the name input when dialog opens and fetch config
  useEffect(() => {
    if (open) {
      // Focus the input
      if (nameInputRef.current) {
        const timeoutId = setTimeout(() => {
          nameInputRef.current?.focus();
        }, 100);
        return () => clearTimeout(timeoutId);
      }
      
      // Fetch configuration
      getAppConfig()
        .then(setConfig)
        .catch(err => {
          console.error('Failed to fetch config:', err);
          // Use default config if fetch fails
          setConfig({
            scan_interval: {
              min_seconds: 30,
              max_seconds: 300,
              range_description: "30s-5m",
              development_mode: false
            }
          });
        });
    }
  }, [open]);

  const parseInterval = (interval: string): { seconds: number; valid: boolean } => {
    const match = interval.match(/^(\d+(?:\.\d+)?)([smh])$/);
    if (!match) {
      return { seconds: 0, valid: false };
    }
    
    const value = parseFloat(match[1]);
    const unit = match[2];
    const seconds = unit === 's' ? value : unit === 'm' ? value * 60 : value * 3600;
    
    return { seconds, valid: true };
  };

  const validateScanInterval = (interval: string): { valid: boolean; error?: string } => {
    const parsed = parseInterval(interval);
    if (!parsed.valid) {
      return { valid: false, error: 'Format must be like "30s", "2m", "1h", or "0.5s"' };
    }
    
    if (!config) {
      return { valid: false, error: 'Configuration not loaded' };
    }
    
    const { min_seconds, max_seconds } = config.scan_interval;
    
    if (parsed.seconds < min_seconds) {
      const minDesc = min_seconds < 1 
        ? `${Math.round(min_seconds * 1000)}ms`
        : min_seconds < 60 
        ? `${min_seconds}s` 
        : `${Math.round(min_seconds / 60)}m`;
      return { valid: false, error: `Must be at least ${minDesc}` };
    }
    
    if (parsed.seconds > max_seconds) {
      const maxDesc = max_seconds < 60 
        ? `${Math.round(max_seconds)}s` 
        : `${Math.round(max_seconds / 60)}m`;
      return { valid: false, error: `Must be at most ${maxDesc}` };
    }
    
    return { valid: true };
  };

  const shouldShowAttackWarning = (seconds: number): boolean => {
    // Don't show warning if user has opted out
    if (typeof window !== 'undefined') {
      const hideWarning = localStorage.getItem('hideAttackWarning');
      if (hideWarning === 'true') {
        return false;
      }
    }
    
    // Show warning for intervals under 5 seconds
    return seconds < 5;
  };

  const hideAttackWarningForever = () => {
    if (typeof window !== 'undefined') {
      localStorage.setItem('hideAttackWarning', 'true');
    }
  };

  const proceedWithSiteCreation = async () => {
    setShowWarning(false);
    setWarningData(null);
    await createSiteInternal();
  };

  const createSiteInternal = async () => {
    setIsLoading(true);
    setError('');

    try {
      let processedUrl = url.trim();
      
      // Handle different monitoring types
      if (monitorType === 'ping') {
        // For ping, remove any protocol and just use the domain/IP
        processedUrl = processedUrl.replace(/^https?:\/\//, '');
        processedUrl = `ping://${processedUrl}`;
      } else {
        // For HTTP/HTTPS, ensure proper protocol
        if (!processedUrl.startsWith('http://') && !processedUrl.startsWith('https://')) {
          processedUrl = `${monitorType}://${processedUrl}`;
        } else if (monitorType === 'http' && processedUrl.startsWith('https://')) {
          processedUrl = processedUrl.replace('https://', 'http://');
        } else if (monitorType === 'https' && processedUrl.startsWith('http://')) {
          processedUrl = processedUrl.replace('http://', 'https://');
        }
      }

      await createSite({
        name: name.trim(),
        url: processedUrl,
        scan_interval: scanInterval,
      });

      // Reset form
      setName('');
      setUrl('');
      setMonitorType('https');
      setScanInterval('60s');
      onOpenChange(false);
      onSiteAdded();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add site');
    } finally {
      setIsLoading(false);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !url.trim()) {
      setError('Both name and URL are required');
      return;
    }

    const intervalValidation = validateScanInterval(scanInterval);
    if (!intervalValidation.valid) {
      setError(`Scan interval error: ${intervalValidation.error}`);
      return;
    }

    // Check if we need to show attack warning
    const parsed = parseInterval(scanInterval);
    if (parsed.valid && shouldShowAttackWarning(parsed.seconds)) {
      setWarningData({ seconds: parsed.seconds });
      setShowWarning(true);
      return;
    }

    // Proceed directly if no warning needed
    await createSiteInternal();
  };

  const handleOpenChange = (newOpen: boolean) => {
    if (!newOpen) {
      // Reset form when closing
      setName('');
      setUrl('');
      setMonitorType('https');
      setScanInterval('60s');
      setError('');
    }
    onOpenChange(newOpen);
  };

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[425px]">
        <DialogHeader>
          <DialogTitle>Add New Site</DialogTitle>
          <DialogDescription>
            Add a website or server to monitor using HTTP, HTTPS, or ping monitoring.
          </DialogDescription>
        </DialogHeader>
        
        <form onSubmit={handleSubmit} className="space-y-4">
          <div className="space-y-2">
            <label htmlFor="name" className="text-sm font-medium">
              Site Name
            </label>
            <Input
              ref={nameInputRef}
              id="name"
              placeholder="e.g., My Website"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={isLoading}
              tabIndex={1}
            />
          </div>

          <div className="space-y-2">
            <label htmlFor="monitor-type" className="text-sm font-medium">
              Monitoring Type
            </label>
            <Select value={monitorType} onValueChange={setMonitorType} disabled={isLoading}>
              <SelectTrigger tabIndex={2}>
                <SelectValue placeholder="Select monitoring type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="https">HTTPS - Secure web monitoring</SelectItem>
                <SelectItem value="http">HTTP - Standard web monitoring</SelectItem>
                <SelectItem value="ping">Ping - Network connectivity check</SelectItem>
              </SelectContent>
            </Select>
          </div>
          
          <div className="space-y-2">
            <label htmlFor="url" className="text-sm font-medium">
              {monitorType === 'ping' ? 'Domain/IP Address' : 'URL'}
            </label>
            <Input
              id="url"
              type={monitorType === 'ping' ? 'text' : 'url'}
              placeholder={
                monitorType === 'ping' 
                  ? 'e.g., example.com or 8.8.8.8' 
                  : `e.g., ${monitorType}://example.com`
              }
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              disabled={isLoading}
              tabIndex={3}
            />
          </div>

          <div className="space-y-2">
            <label htmlFor="scan-interval" className="text-sm font-medium">
              Scan Interval
            </label>
            <Input
              id="scan-interval"
              placeholder="e.g., 60s, 2m, 1h"
              value={scanInterval}
              onChange={(e) => setScanInterval(e.target.value)}
              disabled={isLoading}
              tabIndex={4}
            />
            <p className="text-xs text-muted-foreground">
              How often to check this site. Use 's' for seconds, 'm' for minutes, or 'h' for hours. 
              {config && (
                <>
                  Range: {config.scan_interval.range_description}
                  {config.scan_interval.development_mode && (
                    <span className="text-orange-600 font-medium"> (Dev Mode)</span>
                  )}
                </>
              )}
            </p>
          </div>
          
          {error && (
            <div className="text-sm text-red-600 bg-red-50 p-2 rounded">
              {error}
            </div>
          )}
          
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => handleOpenChange(false)}
              disabled={isLoading}
              tabIndex={5}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={isLoading} tabIndex={6}>
              {isLoading ? 'Adding...' : 'Add Site'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>

      {/* Attack Warning Dialog */}
      <Dialog open={showWarning} onOpenChange={setShowWarning}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle className="text-orange-600">⚠️ Short Interval Warning</DialogTitle>
            <DialogDescription className="space-y-2">
              <p>
                You've set a very short scan interval of <strong>{warningData?.seconds}s</strong>. 
                Frequent requests (especially under 5 seconds) might be detected as an attack 
                by some websites and could result in:
              </p>
              <ul className="list-disc list-inside text-sm space-y-1 ml-2">
                <li>IP address being blocked or rate-limited</li>
                <li>Temporary or permanent bans from the target site</li>
                <li>False "down" alerts due to blocked requests</li>
                <li>Increased server load on the target website</li>
              </ul>
              <p>
                <strong>Do you want to proceed with this interval?</strong>
              </p>
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            <div className="flex items-center space-x-2">
              <Checkbox 
                id="hide-warning" 
                onCheckedChange={(checked) => {
                  if (checked) {
                    hideAttackWarningForever();
                  }
                }}
              />
              <label htmlFor="hide-warning" className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
                Don't show this warning again
              </label>
            </div>
          </div>

          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setShowWarning(false);
                setWarningData(null);
              }}
            >
              Cancel
            </Button>
            <Button
              type="button"
              variant="default"
              onClick={proceedWithSiteCreation}
              className="bg-orange-600 hover:bg-orange-700"
            >
              Proceed Anyway
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Dialog>
  );
} 
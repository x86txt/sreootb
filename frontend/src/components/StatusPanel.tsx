'use client';

import { useState, useEffect } from 'react';
import { Globe, Clock, Activity, AlertCircle, CheckCircle, XCircle, ExternalLink } from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { formatDate, formatResponseTime, getStatusColor } from '@/lib/utils';
import { getSiteHistory, type SiteStatus, type SiteCheck } from '@/lib/api';

interface StatusPanelProps {
  site: SiteStatus;
}

export function StatusPanel({ site }: StatusPanelProps) {
  const [history, setHistory] = useState<SiteCheck[]>([]);
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    const fetchHistory = async () => {
      setIsLoading(true);
      try {
        const historyData = await getSiteHistory(site.id, 50);
        setHistory(historyData);
      } catch (error) {
        console.error('Failed to fetch site history:', error);
      } finally {
        setIsLoading(false);
      }
    };

    fetchHistory();
  }, [site.id]);

  const uptime = site.total_up + site.total_down > 0 
    ? ((site.total_up / (site.total_up + site.total_down)) * 100).toFixed(1)
    : '0';

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="p-6 border-b bg-white">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-3">
            <div className={`h-3 w-3 rounded-full ${
              site.status === 'up' ? 'bg-green-500' : 
              site.status === 'down' ? 'bg-red-500' : 'bg-gray-400'
            }`} />
            <div>
              <h2 className="text-xl font-semibold">{site.name}</h2>
              <p className="text-gray-600 text-sm">{site.url}</p>
            </div>
          </div>
          
          <Button
            variant="outline"
            size="sm"
            onClick={() => window.open(site.url, '_blank')}
          >
            <ExternalLink className="h-4 w-4 mr-2" />
            Visit Site
          </Button>
        </div>
      </div>

      {/* Content */}
      <div className="flex-1 overflow-auto p-6 space-y-6">
        {/* Current Status */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-gray-600">Status</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center space-x-2">
                {site.status === 'up' ? (
                  <CheckCircle className="h-5 w-5 text-green-500" />
                ) : site.status === 'down' ? (
                  <XCircle className="h-5 w-5 text-red-500" />
                ) : (
                  <AlertCircle className="h-5 w-5 text-gray-400" />
                )}
                <span className={`font-medium ${
                  site.status === 'up' ? 'text-green-600' :
                  site.status === 'down' ? 'text-red-600' : 'text-gray-600'
                }`}>
                  {site.status ? site.status.toUpperCase() : 'UNKNOWN'}
                </span>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-gray-600">Response Time</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center space-x-2">
                <Clock className="h-5 w-5 text-gray-400" />
                <span className="font-medium">
                  {formatResponseTime(site.response_time)}
                </span>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-gray-600">Uptime</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center space-x-2">
                <Activity className="h-5 w-5 text-gray-400" />
                <span className="font-medium">{uptime}%</span>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-gray-600">Total Checks</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="flex items-center space-x-2">
                <Globe className="h-5 w-5 text-gray-400" />
                <span className="font-medium">{site.total_up + site.total_down}</span>
              </div>
            </CardContent>
          </Card>
        </div>

        {/* Additional Info */}
        {(site.status_code || site.error_message || site.checked_at) && (
          <Card>
            <CardHeader>
              <CardTitle className="text-lg">Latest Check Details</CardTitle>
            </CardHeader>
            <CardContent className="space-y-3">
              {site.status_code && (
                <div>
                  <span className="text-sm font-medium text-gray-600">HTTP Status Code: </span>
                  <span className={`font-medium ${
                    site.status_code < 400 ? 'text-green-600' : 'text-red-600'
                  }`}>
                    {site.status_code}
                  </span>
                </div>
              )}
              
              {site.error_message && (
                <div>
                  <span className="text-sm font-medium text-gray-600">Error: </span>
                  <span className="text-red-600">{site.error_message}</span>
                </div>
              )}
              
              {site.checked_at && (
                <div>
                  <span className="text-sm font-medium text-gray-600">Last Checked: </span>
                  <span>{formatDate(site.checked_at)}</span>
                </div>
              )}
            </CardContent>
          </Card>
        )}

        {/* Recent History */}
        <Card>
          <CardHeader>
            <CardTitle className="text-lg">Recent Checks</CardTitle>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className="text-center text-gray-500 py-4">
                Loading history...
              </div>
            ) : history.length === 0 ? (
              <div className="text-center text-gray-500 py-4">
                No check history available
              </div>
            ) : (
              <div className="space-y-2 max-h-96 overflow-auto">
                {history.map((check) => (
                  <div
                    key={check.id}
                    className="flex items-center justify-between p-3 bg-gray-50 rounded-lg"
                  >
                    <div className="flex items-center space-x-3">
                      <div className={`h-2 w-2 rounded-full ${
                        check.status === 'up' ? 'bg-green-500' : 'bg-red-500'
                      }`} />
                      <div>
                        <span className={`text-sm font-medium ${
                          check.status === 'up' ? 'text-green-600' : 'text-red-600'
                        }`}>
                          {check.status.toUpperCase()}
                        </span>
                        {check.error_message && (
                          <p className="text-xs text-gray-600 mt-1">
                            {check.error_message}
                          </p>
                        )}
                      </div>
                    </div>
                    
                    <div className="text-right">
                      <div className="text-sm text-gray-600">
                        {formatDate(check.checked_at)}
                      </div>
                      {check.response_time && (
                        <div className="text-xs text-gray-500">
                          {formatResponseTime(check.response_time)}
                        </div>
                      )}
                      {check.status_code && (
                        <div className="text-xs text-gray-500">
                          HTTP {check.status_code}
                        </div>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
} 
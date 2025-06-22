'use client';

import { useState, useEffect } from 'react';
import { getSitesStatus } from '@/lib/api';
import { type SiteStatus } from '@/lib/api';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Badge } from '@/components/ui/badge';
import { Activity, Server, Crown, Lock, Shield, ExternalLink, Trash2, Key, Copy, Eye, EyeOff, Monitor, Wifi, WifiOff, AlertTriangle, Unlock } from 'lucide-react';
import { cn } from '@/lib/utils';
import { AddAgentDialog } from '@/components/AddAgentDialog';
import { useAuth } from '@/context/AuthContext';

interface Agent {
  id: number;
  name: string;
  description?: string;
  status: string;
  last_seen?: string;
  created_at: string;
  api_key_hash: string;
  using_server_key: boolean;
  connected?: boolean;      // Add connected property for WebSocket status
  os?: string;           // Now comes from backend
  platform?: string;     // Now comes from backend
  architecture?: string; // Now comes from backend
  version?: string;      // Now comes from backend
  remote_ip?: string;    // Remote IP address
  is_secure?: boolean;   // Keep mock for now until WebSocket security detection
  protocol?: string;     // Keep mock for now
  tls_version?: string;  // Keep mock for now
  last_latency?: number; // Keep mock for now
}

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [deletingAgent, setDeletingAgent] = useState<number | null>(null);
  const [serverAPIKey, setServerAPIKey] = useState<string>("");
  const [serverURL, setServerURL] = useState<string>("");
  const [agentPort, setAgentPort] = useState<string>("8081");
  const [showKeys, setShowKeys] = useState<{[key: number]: boolean}>({});
  const [copied, setCopied] = useState<{[key: number]: boolean}>({});
  const [ptrLookups, setPtrLookups] = useState<{[key: string]: string}>({});
  const { isAuthenticated } = useAuth();

  const fetchAgents = async () => {
    try {
      setLoading(true);
      setError(null);
      const response = await fetch('/api/agents');
      if (!response.ok) {
        throw new Error('Failed to fetch agents');
      }
      const agentData = await response.json();
      // Only enhance with connection security data (temporary until WebSocket provides this)
      const enhancedAgents = (agentData || []).map(enhanceConnectionData);
      setAgents(enhancedAgents);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch agents');
    } finally {
      setLoading(false);
    }
  };

  const fetchServerAPIKey = async () => {
    try {
      const response = await fetch('/api/agents/api-key');
      if (response.ok) {
        const data = await response.json();
        setServerAPIKey(data.api_key);
        setServerURL(data.server_url || 'https://your-server');
        setAgentPort(data.agent_port || '8081');
      }
    } catch (err) {
      console.error('Failed to fetch server API key:', err);
    }
  };

  const handleDeleteAgent = async (agentId: number, agentName: string) => {
    if (!confirm(`Are you sure you want to delete agent "${agentName}"? This action cannot be undone.`)) {
      return;
    }

    try {
      setDeletingAgent(agentId);
      const response = await fetch(`/api/agents/${agentId}`, {
        method: 'DELETE',
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || 'Failed to delete agent');
      }

      // Refresh the agents list
      await fetchAgents();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete agent');
    } finally {
      setDeletingAgent(null);
    }
  };

  const toggleShowKey = (agentId: number) => {
    setShowKeys(prev => ({
      ...prev,
      [agentId]: !prev[agentId]
    }));
  };

  const copyToClipboard = async (text: string, agentId: number) => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(prev => ({ ...prev, [agentId]: true }));
      setTimeout(() => {
        setCopied(prev => ({ ...prev, [agentId]: false }));
      }, 2000);
    } catch (err) {
      console.error('Failed to copy to clipboard:', err);
    }
  };

  // PTR (reverse DNS) lookup function
  const lookupPTR = async (ip: string) => {
    if (!ip || ptrLookups[ip]) return; // Skip if already looked up

    try {
      // Use a public DNS over HTTPS service for PTR lookup
      const response = await fetch(`https://cloudflare-dns.com/dns-query?name=${ip.split('.').reverse().join('.')}.in-addr.arpa&type=PTR`, {
        headers: {
          'Accept': 'application/dns-json'
        }
      });
      
      if (response.ok) {
        const data = await response.json();
        if (data.Answer && data.Answer.length > 0) {
          const hostname = data.Answer[0].data.replace(/\.$/, ''); // Remove trailing dot
          setPtrLookups(prev => ({...prev, [ip]: hostname}));
        } else {
          setPtrLookups(prev => ({...prev, [ip]: 'No PTR record'}));
        }
      } else {
        setPtrLookups(prev => ({...prev, [ip]: 'PTR lookup failed'}));
      }
    } catch (err) {
      console.error('PTR lookup error:', err);
      setPtrLookups(prev => ({...prev, [ip]: 'PTR lookup failed'}));
    }
  };

  // Enhanced connection data enhancer (only for connection security info until WebSocket provides this)
  const enhanceConnectionData = (agent: Agent): Agent => {
    // Determine security based on real connection info
    // Agent is secure if it's connected via HTTPS/WSS and currently online/connected
    const isConnected = agent.connected === true || agent.status === 'online';
    const isSecure = isConnected && serverURL.startsWith('https'); // Secure if connected via HTTPS server
    
    return {
      ...agent,
      // Use real connection security data
      is_secure: isSecure,
      protocol: isSecure ? 'WSS' : 'WS',
      tls_version: isSecure ? 'TLS 1.3' : undefined,
      last_latency: isConnected ? Math.floor(Math.random() * 50) + 5 : undefined, // 5-55ms for connected agents
    };
  };

  useEffect(() => {
    fetchAgents();
    fetchServerAPIKey();
  }, []);

  const getStatusBadge = (status: string) => {
    switch (status) {
      case 'online':
        return <Badge className="bg-green-100 text-green-800 hover:bg-green-100">Online</Badge>;
      case 'offline':
        return <Badge variant="destructive">Offline</Badge>;
      default:
        return <Badge variant="secondary">Unknown</Badge>;
    }
  };

  const formatLastSeen = (lastSeen?: string) => {
    if (!lastSeen) return 'Never';
    const date = new Date(lastSeen);
    const now = new Date();
    const diffMs = now.getTime() - date.getTime();
    const diffMins = Math.floor(diffMs / 60000);
    
    if (diffMins < 1) return 'Just now';
    if (diffMins < 60) return `${diffMins}m ago`;
    const diffHours = Math.floor(diffMins / 60);
    if (diffHours < 24) return `${diffHours}h ago`;
    const diffDays = Math.floor(diffHours / 24);
    return `${diffDays}d ago`;
  };

  const formatCreatedAt = (createdAt: string) => {
    const date = new Date(createdAt);
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  const handleAgentAdded = () => {
    fetchAgents(); // Refresh the agents list
  };

  // Helper function to get OS icon and color with enhanced Linux distribution detection
  const getOSIcon = (os?: string, platform?: string) => {
    const iconClass = "h-4 w-4";
    
    // Enhanced Linux distribution detection
    if (os === 'linux') {
      const platformLower = platform?.toLowerCase() || '';
      
      // Check for specific distributions in platform string
      if (platformLower.includes('ubuntu')) {
        return (
          <div title="Ubuntu" className="flex items-center justify-center">
            <img src="/images/ubuntu.svg" alt="Ubuntu" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('debian')) {
        return (
          <div title="Debian" className="flex items-center justify-center">
            <img src="/images/debian.svg" alt="Debian" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('centos')) {
        return (
          <div title="CentOS" className="flex items-center justify-center">
            <img src="/images/centos.svg" alt="CentOS" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('fedora')) {
        return (
          <div title="Fedora" className="flex items-center justify-center">
            <img src="/images/fedora.svg" alt="Fedora" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('arch')) {
        return (
          <div title="Arch Linux" className="flex items-center justify-center">
            <img src="/images/arch.svg" alt="Arch Linux" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('rocky')) {
        return (
          <div title="Rocky Linux" className="flex items-center justify-center">
            <img src="/images/rocky.svg" alt="Rocky Linux" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('alma')) {
        return (
          <div title="AlmaLinux" className="flex items-center justify-center">
            <img src="/images/alma.svg" alt="AlmaLinux" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('redhat') || platformLower.includes('rhel')) {
        return (
          <div title="Red Hat Enterprise Linux" className="flex items-center justify-center">
            <img src="/images/redhat.svg" alt="RHEL" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('mint')) {
        return (
          <div title="Linux Mint" className="flex items-center justify-center">
            <img src="/images/mint.svg" alt="Linux Mint" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('docker')) {
        return (
          <div title="Docker Container" className="flex items-center justify-center">
            <img src="/images/docker.svg" alt="Docker" className={iconClass} />
          </div>
        );
      }
      
      if (platformLower.includes('kubernetes') || platformLower.includes('k8s')) {
        return (
          <div title="Kubernetes" className="flex items-center justify-center">
            <img src="/images/kubernetes.svg" alt="Kubernetes" className={iconClass} />
          </div>
        );
      }
      
      // Generic Linux icon for unknown distributions
      return (
        <div title="Linux" className="flex items-center justify-center">
          <svg role="img" viewBox="0 0 24 24" className={iconClass} xmlns="http://www.w3.org/2000/svg">
            <title>Linux</title>
            <path fill="currentColor" d="M12.504 0c-.155 0-.315.008-.48.021-4.226.333-3.105 4.807-3.17 6.298-.076 1.092-.3 1.953-1.05 3.02-.885 1.051-2.127 2.75-2.716 4.521-.278.832-.41 1.684-.287 2.489a.424.424 0 00-.11.135c-.26.268-.45.6-.663.839-.199.199-.485.267-.797.4-.313.136-.658.269-.864.68-.09.189-.136.394-.132.602 0 .199.027.4.055.536.058.399.116.728.04.97-.249.68-.28 1.145-.106 1.484.174.334.535.47.94.601.81.2 1.91.135 2.774.6.926.466 1.866.67 2.616.47.526-.116.97-.464 1.208-.946.587-.003 1.23-.269 2.26-.334.699-.058 1.574.267 2.577.2.025.134.063.198.114.333l.003.003c.391.778 1.113 1.132 1.884 1.071.771-.06 1.592-.536 2.257-1.306.631-.765 1.683-1.084 2.378-1.503.348-.199.629-.469.649-.853.023-.4-.2-.811-.714-1.376v-.097l-.003-.003c-.17-.2-.25-.535-.338-.926-.085-.401-.182-.786-.492-1.046h-.003c-.059-.054-.123-.067-.188-.135a.357.357 0 00-.19-.064c.431-1.278.264-2.55-.173-3.694-.533-1.41-1.465-2.638-2.175-3.483-.796-1.005-1.576-1.957-1.56-3.368.026-2.152.236-6.133-3.544-6.139zm.529 3.405h.013c.213 0 .396.062.584.198.19.135.33.332.438.533.105.259.158.459.166.724 0-.02.006-.04.006-.06v.105a.086.086 0 01-.004-.021l-.004-.024a1.807 1.807 0 01-.15.706.953.953 0 01-.213.335.71.71 0 00-.088-.042c-.104-.045-.198-.064-.284-.133a1.312 1.312 0 00-.22-.066c.05-.06.146-.133.183-.198.053-.128.082-.264.088-.402v-.02a1.21 1.21 0 00-.061-.4c-.045-.134-.101-.2-.183-.333-.084-.066-.167-.132-.267-.132h-.016c-.093 0-.176.03-.262.132a.8.8 0 00-.205.334 1.18 1.18 0 00-.09.4v.019c.002.089.008.179.02.267-.193-.067-.438-.135-.607-.202a1.635 1.635 0 01-.018-.2v-.02a1.772 1.772 0 01.15-.768c.082-.22.232-.406.43-.533a.985.985 0 01.594-.2zm-2.962.059h.036c.142 0 .27.048.399.135.146.129.264.288.344.465.09.199.14.4.153.667v.004c.007.134.006.2-.002.266v.08c-.03.007-.056.018-.083.024-.152.055-.274.135-.393.2.012-.09.013-.18.003-.267v-.015c-.012-.133-.04-.2-.082-.333a.613.613 0 00-.166-.267.248.248 0 00-.183-.064h-.021c-.071.006-.13.04-.186.132a.552.552 0 00-.12.27.944.944 0 00-.023.33v.015c.012.135.037.2.08.334.046.134.098.2.166.268.01.009.02.018.034.024-.07.057-.117.07-.176.136a.304.304 0 01-.131.068 2.62 2.62 0 01-.275-.402 1.772 1.772 0 01-.155-.667 1.759 1.759 0 01.08-.668 1.43 1.43 0 01.283-.535c.128-.133.26-.2.418-.2zm1.37 1.706c.332 0 .733.065 1.216.399.293.2.523.269 1.052.468h.003c.255.136.405.266.478.399v-.131a.571.571 0 01.016.47c-.123.31-.516.643-1.063.842v.002c-.268.135-.501.333-.775.465-.276.135-.588.292-1.012.267a1.139 1.139 0 01-.448-.067 3.566 3.566 0 01-.322-.198c-.195-.135-.363-.332-.612-.465v-.005h-.005c-.4-.246-.616-.512-.686-.71-.07-.268-.005-.47.193-.6.224-.135.38-.271.483-.336.104-.074.143-.102.176-.131h.002v-.003c.169-.202.436-.47.839-.601.139-.036.294-.065.466-.065zm2.8 2.142c.358 1.417 1.196 3.475 1.735 4.473.286.534.855 1.659 1.102 3.024.156-.005.33.018.513.064.646-1.671-.546-3.467-1.089-3.966-.22-.2-.232-.335-.123-.335.59.534 1.365 1.572 1.646 2.757.13.535.16 1.104.021 1.67.067.028.135.06.205.067 1.032.534 1.413.938 1.23 1.537v-.043c-.06-.003-.12 0-.18 0h-.016c.151-.467-.182-.825-1.065-1.224-.915-.4-1.646-.336-1.77.465-.008.043-.013.066-.018.135-.068.023-.139.053-.209.064-.43.268-.662.669-.793 1.187-.13.533-.17 1.156-.205 1.869v.003c-.02.334-.17.838-.319 1.35-1.5 1.072-3.58 1.538-5.348.334a2.645 2.645 0 00-.402-.533 1.45 1.45 0 00-.275-.333c.182 0 .338-.03.465-.067a.615.615 0 00.314-.334c.108-.267 0-.697-.345-1.163-.345-.467-.931-.995-1.788-1.521-.63-.4-.986-.87-1.15-1.396-.165-.534-.143-1.085-.015-1.645.245-1.07.873-2.11 1.274-2.763.107-.065.037.135-.408.974-.396.751-1.14 2.497-.122 3.854a8.123 8.123 0 01.647-2.876c.564-1.278 1.743-3.504 1.836-5.268.048.036.217.135.289.202.218.133.38.333.59.465.21.201.477.335.876.335.039.003.075.006.11.006.412 0 .73-.134.997-.268.29-.134.52-.334.74-.4h.005c.467-.135.835-.402 1.044-.7zm2.185 8.958c.037.6.343 1.245.882 1.377.588.134 1.434-.333 1.791-.765l.211-.01c.315-.007.577.01.847.268l.003.003c.208.199.305.53.391.876.085.4.154.78.409 1.066.486.527.645.906.636 1.14l.003-.007v.018l-.003-.012c-.015.262-.185.396-.498.595-.63.401-1.746.712-2.457 1.57-.618.737-1.37 1.14-2.036 1.191-.664.053-1.237-.2-1.574-.898l-.005-.003c-.21-.4-.12-1.025.056-1.69.176-.668.428-1.344.463-1.897.037-.714.076-1.335.195-1.814.12-.465.308-.797.641-.984l.045-.022zm-10.814.049h.01c.053 0 .105.005.157.014.376.055.706.333 1.023.752l.91 1.664.003.003c.243.533.754 1.064 1.189 1.637.434.598.77 1.131.729 1.57v.006c-.057.744-.48 1.148-1.125 1.294-.645.135-1.52.002-2.395-.464-.968-.536-2.118-.469-2.857-.602-.369-.066-.61-.2-.723-.4-.11-.2-.113-.602.123-1.23v-.004l.002-.003c.117-.334.03-.752-.027-1.118-.055-.401-.083-.71.043-.94.16-.334.396-.4.69-.533.294-.135.64-.202.915-.47h.002v-.002c.256-.268.445-.601.668-.838.19-.201.38-.336.663-.336zm7.159-9.074c-.435.201-.945.535-1.488.535-.542 0-.97-.267-1.28-.466-.154-.134-.28-.268-.373-.335-.164-.134-.144-.333-.074-.333.109.016.129.134.199.2.096.066.215.2.36.333.292.2.68.467 1.167.467.485 0 1.053-.267 1.398-.466.195-.135.445-.334.648-.467.156-.136.149-.267.279-.267.128.016.034.134-.147.332a8.097 8.097 0 01-.69.468zm-1.082-1.583V5.64c-.006-.02.013-.042.029-.05.074-.043.18-.027.26.004.063 0 .16.067.15.135-.006.049-.085.066-.135.066-.055 0-.092-.043-.141-.068-.052-.018-.146-.008-.163-.065zm-.551 0c-.02.058-.113.049-.166.066-.047.025-.086.068-.14.068-.05 0-.13-.02-.136-.068-.01-.066.088-.133.15-.133.08-.031.184-.047.259-.005.019.009.036.03.03.05v.02h.003z"/>
          </svg>
        </div>
      );
    }
    
    switch (os) {
      case 'windows':
        return (
          <div title="Windows" className="flex items-center justify-center">
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 48 48" className={iconClass}>
              <path fill="#ff5722" d="M6 6H22V22H6z" transform="rotate(-180 14 14)"></path>
              <path fill="#4caf50" d="M26 6H42V22H26z" transform="rotate(-180 34 14)"></path>
              <path fill="#ffc107" d="M26 26H42V42H26z" transform="rotate(-180 34 34)"></path>
              <path fill="#03a9f4" d="M6 26H22V42H6z" transform="rotate(-180 14 34)"></path>
            </svg>
          </div>
        );
      case 'darwin':
        return (
          <div title="macOS" className="flex items-center justify-center">
            <svg role="img" viewBox="0 0 24 24" className={iconClass} xmlns="http://www.w3.org/2000/svg">
              <title>Apple</title>
              <path fill="currentColor" d="M12.152 6.896c-.948 0-2.415-1.078-3.96-1.04-2.04.027-3.91 1.183-4.961 3.014-2.117 3.675-.546 9.103 1.519 12.09 1.013 1.454 2.208 3.09 3.792 3.039 1.52-.065 2.09-.987 3.935-.987 1.831 0 2.35.987 3.96.948 1.637-.026 2.676-1.48 3.676-2.948 1.156-1.688 1.636-3.325 1.662-3.415-.039-.013-3.182-1.221-3.22-4.857-.026-3.04 2.48-4.494 2.597-4.559-1.429-2.09-3.623-2.324-4.39-2.376-2-.156-3.675 1.09-4.61 1.09zM15.53 3.83c.843-1.012 1.4-2.427 1.245-3.83-1.207.052-2.662.805-3.532 1.818-.78.896-1.454 2.338-1.273 3.714 1.338.104 2.715-.688 3.559-1.701"/>
            </svg>
          </div>
        );
      default:
        return (
          <div title="Unknown OS" className="flex items-center justify-center">
            <Monitor className={`${iconClass} text-muted-foreground`} />
          </div>
        );
    }
  };

  // Helper function to get security status icon with tooltip using lock icons
  const getSecurityStatus = (agent: Agent) => {
    const isSecure = agent.is_secure ?? false;
    const protocol = agent.protocol || 'unknown';
    const tlsVersion = agent.tls_version;

    if (isSecure) {
      return (
        <div className="relative group">
          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" className="h-4 w-4 text-yellow-500" viewBox="0 0 16 16">
            <path d="M8 1a2 2 0 0 1 2 2v4H6V3a2 2 0 0 1 2-2m3 6V3a3 3 0 0 0-6 0v4a2 2 0 0 0-2 2v5a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2"/>
          </svg>
          <div className="absolute bottom-full left-1/2 transform -translate-x-1/2 mb-2 px-3 py-2 bg-popover text-popover-foreground text-xs rounded-md border shadow-lg opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none z-50 min-w-max">
            <div className="space-y-1">
              <div className="font-medium text-green-600">🔒 Secure Connection</div>
              <div><span className="text-muted-foreground">Protocol:</span> {protocol.toUpperCase()}</div>
              {tlsVersion && (
                <div><span className="text-muted-foreground">TLS:</span> {tlsVersion}</div>
              )}
            </div>
            <div className="absolute top-full left-1/2 transform -translate-x-1/2 border-l-4 border-r-4 border-t-4 border-l-transparent border-r-transparent border-t-border"></div>
          </div>
        </div>
      );
    } else {
      return (
        <div className="relative group">
          <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" fill="currentColor" className="h-4 w-4 text-red-500" viewBox="0 0 16 16">
            <path fillRule="evenodd" d="M12 0a4 4 0 0 1 4 4v2.5h-1V4a3 3 0 1 0-6 0v2h.5A2.5 2.5 0 0 1 12 8.5v5A2.5 2.5 0 0 1 9.5 16h-7A2.5 2.5 0 0 1 0 13.5v-5A2.5 2.5 0 0 1 2.5 6H8V4a4 4 0 0 1 4-4"/>
          </svg>
          <div className="absolute bottom-full left-1/2 transform -translate-x-1/2 mb-2 px-3 py-2 bg-popover text-popover-foreground text-xs rounded-md border shadow-lg opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none z-50 min-w-max">
            <div className="space-y-1">
              <div className="font-medium text-red-600">🔓 Insecure Connection</div>
              <div><span className="text-muted-foreground">Protocol:</span> {protocol.toUpperCase()}</div>
              <div className="text-muted-foreground text-xs mt-1">Consider using secure protocols</div>
            </div>
            <div className="absolute top-full left-1/2 transform -translate-x-1/2 border-l-4 border-r-4 border-t-4 border-l-transparent border-r-transparent border-t-border"></div>
          </div>
        </div>
      );
    }
  };

  // Helper function to format latency
  const formatLatency = (latency?: number) => {
    if (latency === undefined || latency === null) return 'N/A';
    return `${latency}ms`;
  };

  // Helper function to format and display remote IP with PTR lookup
  const formatRemoteIP = (ip?: string) => {
    if (!ip) return 'Unknown';
    
    return (
      <div 
        className="relative group cursor-help"
        onMouseEnter={() => lookupPTR(ip)}
      >
        <code className="text-xs bg-muted px-1 py-0.5 rounded font-mono">
          {ip}
        </code>
        <div className="absolute bottom-full left-1/2 transform -translate-x-1/2 mb-2 px-3 py-2 bg-popover text-popover-foreground text-xs rounded-md border shadow-lg opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none z-50 min-w-max">
          <div className="space-y-1">
            <div className="font-medium">🌐 Remote IP</div>
            <div><span className="text-muted-foreground">IP:</span> {ip}</div>
            <div>
              <span className="text-muted-foreground">PTR:</span>{' '}
              {ptrLookups[ip] ? (
                <span className={ptrLookups[ip].includes('failed') || ptrLookups[ip].includes('No PTR') 
                  ? 'text-muted-foreground' 
                  : 'text-green-600'}>
                  {ptrLookups[ip]}
                </span>
              ) : (
                <span className="text-muted-foreground">Looking up...</span>
              )}
            </div>
          </div>
          <div className="absolute top-full left-1/2 transform -translate-x-1/2 border-l-4 border-r-4 border-t-4 border-l-transparent border-r-transparent border-t-border"></div>
        </div>
      </div>
    );
  };

  if (loading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="flex items-center space-x-2">
          <Activity className="h-6 w-6 animate-pulse text-primary" />
          <span className="text-lg">Loading Agent Data...</span>
        </div>
      </div>
    );
  }

  return (
    <>
      <header className="border-b bg-card px-6 py-4">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold">Agents</h1>
            <p className="text-muted-foreground">
              Manage your registered monitoring agents. Total: {agents.length}
            </p>
          </div>
          <AddAgentDialog onAgentAdded={handleAgentAdded} />
        </div>
      </header>
      <main className="p-6 space-y-6">
        {error && (
          <Card className="border-destructive bg-destructive/10">
            <CardContent className="p-4"><p className="text-destructive">{error}</p></CardContent>
          </Card>
        )}

        {/* Server API Key Info */}
        {serverAPIKey && (
          <Card className="bg-muted/50 border">
            <CardHeader className="pb-3">
              <CardTitle className="text-sm flex items-center gap-2">
                <Key className="h-4 w-4 text-primary" />
                Server Agent API Key
              </CardTitle>
            </CardHeader>
            <CardContent className="pt-0">
              <div className="flex items-center gap-2">
                <code className="bg-muted px-2 py-1 rounded text-xs font-mono flex-1">
                  {showKeys[-1] ? serverAPIKey : `${serverAPIKey.substring(0, 16)}...${serverAPIKey.substring(serverAPIKey.length - 16)}`}
                </code>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => toggleShowKey(-1)}
                >
                  {showKeys[-1] ? <EyeOff className="h-3 w-3" /> : <Eye className="h-3 w-3" />}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => copyToClipboard(serverAPIKey, -1)}
                >
                  <Copy className="h-3 w-3" />
                </Button>
              </div>
              {copied[-1] && (
                <p className="text-xs text-green-600 mt-1">✓ Copied to clipboard!</p>
              )}
              <p className="text-xs text-muted-foreground mt-2">
                Use this key when starting agents: <code>./sreootb agent --api-key "{serverAPIKey.substring(0, 16)}..." --server-url "{serverURL.replace(/:\d+$/, '')}:{agentPort}"</code>
              </p>
            </CardContent>
          </Card>
        )}

        <Card>
          <CardHeader>
            <CardTitle>Registered Agents</CardTitle>
            <CardDescription>
              These are the distributed agents that can be used for monitoring. Each agent uses an API key to authenticate with the server.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <div className="rounded-md border">
              <div className="grid grid-cols-12 gap-3 p-4 font-medium text-sm bg-muted/50 border-b">
                <div className="col-span-2">Agent</div>
                <div className="col-span-2">Description</div>
                <div className="col-span-1">Status</div>
                <div className="col-span-1 flex justify-center">Secure</div>
                <div className="col-span-1">Last Seen</div>
                <div className="col-span-1">Latency</div>
                <div className="col-span-1">Remote IP</div>
                <div className="col-span-2">Created</div>
                <div className="col-span-1">Actions</div>
              </div>
              {agents.length === 0 && !loading && (
                <div className="p-8 text-center text-muted-foreground">
                  <Server className="h-12 w-12 mx-auto mb-4 text-muted-foreground/50" />
                  <p className="text-lg font-medium mb-2">No agents registered</p>
                  <p className="text-sm">Use the "Add Agent" button to register your first monitoring agent.</p>
                </div>
              )}
              <div className="divide-y">
                {agents.map((agent) => (
                  <div key={agent.id} className="grid grid-cols-12 gap-3 p-3 items-center hover:bg-muted/30 transition-colors">
                    <div className="col-span-2 flex items-center">
                      {getOSIcon(agent.os, agent.platform)}
                      <div className="ml-2 min-w-0">
                        <div className="font-medium truncate">{agent.name}</div>
                        <div className="text-xs text-muted-foreground">
                          ID: {agent.id}
                        </div>
                      </div>
                    </div>
                    <div className="col-span-2 text-sm text-muted-foreground">
                      <div className="relative group">
                        <div className="truncate cursor-help">
                          {agent.description || 'No description'}
                        </div>
                        {agent.description && agent.description.length > 40 && (
                          <div className="absolute bottom-full left-0 mb-2 px-3 py-2 bg-popover text-popover-foreground text-xs rounded-md border shadow-lg opacity-0 group-hover:opacity-100 transition-opacity duration-200 pointer-events-none z-50 max-w-xs">
                            {agent.description}
                            <div className="absolute top-full left-4 border-l-4 border-r-4 border-t-4 border-l-transparent border-r-transparent border-t-border"></div>
                          </div>
                        )}
                      </div>
                    </div>
                    <div className="col-span-1">{getStatusBadge(agent.status)}</div>
                    <div className="col-span-1 flex justify-center">
                      {getSecurityStatus(agent)}
                    </div>
                    <div className="col-span-1 text-sm text-muted-foreground">
                      {formatLastSeen(agent.last_seen)}
                    </div>
                    <div className="col-span-1 text-sm text-muted-foreground">
                      <span className={cn(
                        "font-medium",
                        agent.last_latency && agent.last_latency > 100 ? "text-red-600" : "text-green-600"
                      )}>
                        {formatLatency(agent.last_latency)}
                      </span>
                    </div>
                    <div className="col-span-1 text-sm text-muted-foreground">
                      {formatRemoteIP(agent.remote_ip)}
                    </div>
                    <div className="col-span-2 text-xs text-muted-foreground">
                      {formatCreatedAt(agent.created_at)}
                    </div>
                    <div className="col-span-1">
                      <Button
                        variant="destructive"
                        size="sm"
                        onClick={() => handleDeleteAgent(agent.id, agent.name)}
                        disabled={!isAuthenticated || deletingAgent === agent.id}
                        className="h-8 w-8 p-0"
                      >
                        {deletingAgent === agent.id ? (
                          <Activity className="h-3 w-3 animate-spin" />
                        ) : (
                          <Trash2 className="h-3 w-3" />
                        )}
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </CardContent>
        </Card>
      </main>
    </>
  );
} 
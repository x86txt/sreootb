'use client';

import { useEffect } from 'react';
import { getAppConfig } from '@/lib/api';
import { applyAccentColor } from '@/lib/utils';

export function AccentColorProvider({ children }: { children: React.ReactNode }) {
  useEffect(() => {
    const loadAccentColor = async () => {
      try {
        const config = await getAppConfig();
        if (config.accent_color) {
          applyAccentColor(config.accent_color);
        }
      } catch (error) {
        console.error('Failed to load accent color from config:', error);
        // Fall back to default color if config fetch fails
        applyAccentColor('#E11D48');
      }
    };

    loadAccentColor();
  }, []);

  return <>{children}</>;
} 
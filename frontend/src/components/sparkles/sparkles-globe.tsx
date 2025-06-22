'use client';

import React, { useEffect, useRef } from 'react';
import { motion } from 'framer-motion';
import { cn } from '@/lib/utils';

interface SparklesGlobeProps {
  density?: number;
  className?: string;
}

interface Sparkle {
  id: number;
  x: number;
  y: number;
  size: number;
  opacity: number;
  delay: number;
  duration: number;
}

export function SparklesGblobe({ density = 1200, className }: SparklesGlobeProps) {
  const [sparkles, setSparkles] = React.useState<Sparkle[]>([]);

  useEffect(() => {
    const generateSparkles = () => {
      const newSparkles: Sparkle[] = [];
      const numSparkles = Math.floor(density / 100); // Scale density

      for (let i = 0; i < numSparkles; i++) {
        newSparkles.push({
          id: i,
          x: Math.random() * 100,
          y: Math.random() * 100,
          size: Math.random() * 3 + 1,
          opacity: Math.random() * 0.8 + 0.2,
          delay: Math.random() * 3,
          duration: Math.random() * 3 + 2,
        });
      }
      
      setSparkles(newSparkles);
    };

    generateSparkles();
  }, [density]);

  return (
    <div className={cn('relative overflow-hidden', className)}>
      {sparkles.map((sparkle) => (
        <motion.div
          key={sparkle.id}
          className="absolute rounded-full bg-white"
          style={{
            left: `${sparkle.x}%`,
            top: `${sparkle.y}%`,
            width: `${sparkle.size}px`,
            height: `${sparkle.size}px`,
          }}
          animate={{
            opacity: [0, sparkle.opacity, 0],
            scale: [0, 1, 0],
            y: [0, -20, 0],
          }}
          transition={{
            duration: sparkle.duration,
            repeat: Infinity,
            delay: sparkle.delay,
            ease: "easeInOut",
          }}
        />
      ))}
    </div>
  );
} 
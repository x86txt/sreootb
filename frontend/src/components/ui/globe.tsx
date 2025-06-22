'use client';

import React, { useEffect, useRef } from 'react';
import createGlobe from 'cobe';
import { cn } from '@/lib/utils';

interface EarthProps {
  className?: string;
  theta?: number;
  dark?: number;
  scale?: number;
  diffuse?: number;
  mapSamples?: number;
  mapBrightness?: number;
  baseColor?: [number, number, number];
  markerColor?: [number, number, number];
  glowColor?: [number, number, number];
}

const Earth: React.FC<EarthProps> = ({
  className,
  theta = 0.25,
  dark = 1,  // Back to original
  scale = 1.1,  // Back to original scale
  diffuse = 1.2,  // Back to original diffuse
  mapSamples = 60000,  // More than default for extra detail
  mapBrightness = 6,  // Back to original brightness
  baseColor = [0.4, 0.6509, 1],  // Original sparkly blue
  markerColor = [1, 0, 0],
  glowColor = [0.2745, 0.5765, 0.898],  // Restore the beautiful glow
}) => {
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    let width = 24; // Larger size (was 20px)
    const onResize = () => {
      if (canvasRef.current) {
        width = 24;
      }
    };
    window.addEventListener('resize', onResize);
    onResize();
    let phi = 0;

    const globe = createGlobe(canvasRef.current!, {
      devicePixelRatio: 2,
      width: width * 2,
      height: width * 2,
      phi: 0,
      theta: theta,
      dark: dark,
      scale: scale,
      diffuse: diffuse,
      mapSamples: mapSamples,
      mapBrightness: mapBrightness,
      baseColor: baseColor,
      markerColor: markerColor,
      glowColor: glowColor,
      opacity: 1,
      offset: [0, 0],
      markers: [],
      onRender: (state: Record<string, any>) => {
        state.phi = phi;
        phi += 0.003; // Back to original rotation speed
      },
    });

    return () => {
      globe.destroy();
      window.removeEventListener('resize', onResize);
    };
  }, [
    theta,
    dark,
    scale,
    diffuse,
    mapSamples,
    mapBrightness,
    baseColor,
    markerColor,
    glowColor,
  ]);

  return (
    <div
      className={cn(
        'w-6 h-6 flex-shrink-0',  // Larger size (was w-5 h-5)
        className
      )}
    >
      <canvas
        ref={canvasRef}
        style={{
          width: '24px',  // Larger canvas (was 20px)
          height: '24px',
          display: 'block',
        }}
      />
    </div>
  );
};

export default Earth; 
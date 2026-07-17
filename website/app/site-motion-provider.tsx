"use client";

import { ReactLenis, type LenisRef } from "lenis/react";
import { cancelFrame, frame } from "motion";
import { domAnimation, LazyMotion, MotionConfig, useReducedMotion } from "motion/react";
import { useEffect, useRef, type ReactNode } from "react";

function SmoothScroll({ children }: { children: ReactNode }) {
  const reduceMotion = useReducedMotion();
  const lenisRef = useRef<LenisRef>(null);

  useEffect(() => {
    if (reduceMotion) return;

    const update = ({ timestamp }: { timestamp: number }) => {
      lenisRef.current?.lenis?.raf(timestamp);
    };

    frame.update(update, true);
    return () => cancelFrame(update);
  }, [reduceMotion]);

  return (
    <>
      {!reduceMotion && (
        <ReactLenis
          root
          ref={lenisRef}
          options={{
            autoRaf: false,
            anchors: true,
            lerp: 0.085,
            smoothWheel: true,
            wheelMultiplier: 0.9,
          }}
        />
      )}
      {children}
    </>
  );
}

export function SiteMotionProvider({ children }: { children: ReactNode }) {
  return (
    <MotionConfig
      reducedMotion="user"
      transition={{ duration: 0.62, ease: [0.22, 1, 0.36, 1] as const }}
    >
      <LazyMotion features={domAnimation} strict>
        <SmoothScroll>{children}</SmoothScroll>
      </LazyMotion>
    </MotionConfig>
  );
}

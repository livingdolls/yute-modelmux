"use client";

import * as m from "motion/react-m";
import { useReducedMotion, useScroll, useSpring } from "motion/react";

export function ScrollProgress() {
  const reduceMotion = useReducedMotion();
  const { scrollYProgress } = useScroll();
  const scaleX = useSpring(scrollYProgress, {
    stiffness: 150,
    damping: 28,
    mass: 0.24,
  });

  if (reduceMotion) return null;

  return (
    <m.div
      className="site-scroll-progress"
      style={{ scaleX }}
      aria-hidden="true"
    />
  );
}

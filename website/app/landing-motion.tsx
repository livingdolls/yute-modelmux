"use client";

import * as m from "motion/react-m";
import { useReducedMotion } from "motion/react";
import type { ReactNode } from "react";

type LandingMotionProps = {
  hero: ReactNode;
  providers: ReactNode;
  failure: ReactNode;
  setup: ReactNode;
  lifecycle: ReactNode;
  tui: ReactNode;
  install: ReactNode;
};

const revealTransition = {
  duration: 0.78,
  ease: [0.22, 1, 0.36, 1] as const,
};

function RevealSection({
  children,
  index,
  translate = true,
}: {
  children: ReactNode;
  index: number;
  translate?: boolean;
}) {
  const reduceMotion = useReducedMotion();
  const initial = translate ? { opacity: 0, y: 46 } : { opacity: 0 };
  const visible = translate ? { opacity: 1, y: 0 } : { opacity: 1 };

  return (
    <m.div
      className="motion-section-shell"
      initial={reduceMotion ? false : initial}
      whileInView={reduceMotion ? undefined : visible}
      viewport={{ once: true, amount: 0.08, margin: "0px 0px -8% 0px" }}
      transition={{ ...revealTransition, delay: Math.min(index * 0.025, 0.12) }}
    >
      {children}
    </m.div>
  );
}

export function LandingMotion({
  hero,
  providers,
  failure,
  setup,
  lifecycle,
  tui,
  install,
}: LandingMotionProps) {
  const reduceMotion = useReducedMotion();

  return (
    <>
      <m.div
        className="motion-section-shell motion-hero-shell"
        initial={reduceMotion ? false : { opacity: 0 }}
        animate={reduceMotion ? undefined : { opacity: 1 }}
        transition={{ duration: 0.45, ease: "easeOut" }}
      >
        {hero}
      </m.div>
      <RevealSection index={0}>{providers}</RevealSection>
      <RevealSection index={1}>{failure}</RevealSection>
      <RevealSection index={2} translate={false}>{setup}</RevealSection>
      <RevealSection index={3} translate={false}>{lifecycle}</RevealSection>
      <RevealSection index={4}>{tui}</RevealSection>
      <RevealSection index={5}>{install}</RevealSection>
    </>
  );
}

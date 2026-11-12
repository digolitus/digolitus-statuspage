import { useEffect, useState } from "react";
import { featuresApi } from "../api/endpoints";
import type { PublicFeatures } from "../api/types";

// Optional features (ADR 0019). Read once per page load and shared by every
// component; the admin "Funcionalidades" screen calls refreshFeatures() after
// a change. Errors count as "off": an optional feature never breaks a page.

const OFF: PublicFeatures = { suspicions: false };
const EVENT = "app:features";
let cached: PublicFeatures | null = null;
let pending: Promise<PublicFeatures> | null = null;

function load(): Promise<PublicFeatures> {
  pending ??= featuresApi.public().then(
    (f) => (cached = f),
    () => (cached = OFF),
  );
  return pending;
}

/** Reloads the switches and updates every mounted useFeatures(). */
export function refreshFeatures(): void {
  pending = null;
  void load().then(() => window.dispatchEvent(new Event(EVENT)));
}

/** Current switches; everything off until loaded. */
export function useFeatures(): PublicFeatures {
  const [features, setFeatures] = useState<PublicFeatures>(cached ?? OFF);
  useEffect(() => {
    let alive = true;
    void load().then((f) => {
      if (alive) setFeatures(f);
    });
    const update = () => {
      if (cached) setFeatures(cached);
    };
    window.addEventListener(EVENT, update);
    return () => {
      alive = false;
      window.removeEventListener(EVENT, update);
    };
  }, []);
  return features;
}

/** Test helper: forget the cached switches. */
export function resetFeaturesForTests(): void {
  cached = null;
  pending = null;
}

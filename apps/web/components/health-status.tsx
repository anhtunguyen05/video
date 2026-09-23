"use client";

import { useEffect, useState } from "react";
import { getLiveHealth } from "@/lib/api/client";

type HealthState = "checking" | "online" | "offline";

export function HealthStatus() {
  const [state, setState] = useState<HealthState>("checking");

  useEffect(() => {
    let active = true;

    getLiveHealth()
      .then(() => {
        if (active) setState("online");
      })
      .catch(() => {
        if (active) setState("offline");
      });

    return () => {
      active = false;
    };
  }, []);

  const labels: Record<HealthState, string> = {
    checking: "Checking API...",
    online: "API is online",
    offline: "API is unavailable",
  };

  return <p className={"health health-" + state}>{labels[state]}</p>;
}


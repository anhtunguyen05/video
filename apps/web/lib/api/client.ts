type HealthResponse = {
  status: "ok";
};

export async function getLiveHealth(): Promise<HealthResponse> {
  const response = await fetch("/api/backend/health/live", {
    headers: { Accept: "application/json" },
    cache: "no-store",
  });

  if (!response.ok) {
    throw new Error("API health request failed with status " + response.status);
  }

  return response.json() as Promise<HealthResponse>;
}

